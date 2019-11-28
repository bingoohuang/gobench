package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"mime/multipart"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bingoohuang/golang-trial/randimg"
	"github.com/dustin/go-humanize"

	"github.com/valyala/fasthttp"

	"github.com/docker/go-units"
)

// App ...
type App struct {
	requests    int
	threads     int
	connections int

	duration         string
	urls             string
	postDataFilePath string
	uploadFilePath   string

	keepAlive     bool
	uploadRandImg bool
	exitRequested bool
	printResult   bool

	writeTimeout    uint
	readTimeout     uint
	readThroughput  uint64
	writeThroughput uint64

	authHeader     string
	uploadFileName string
	fixedImgSize   string
	contentType    string

	connectionChan chan bool
}

// Conf for gobench
type Conf struct {
	urls        []string
	method      string
	postData    []byte
	requests    int
	duration    time.Duration
	keepAlive   bool
	authHeader  string
	contentType string

	myClient fasthttp.Client
}

// Init ...
func (a *App) Init() {
	flag.IntVar(&a.requests, "r", 0, "Number of requests per client")
	flag.IntVar(&a.connections, "c", 100, "Number of connections")
	flag.IntVar(&a.threads, "t", 100, "Number of concurrent threads")
	flag.StringVar(&a.urls, "u", "", "URL list (comma separated), or @URL's file path (line separated)")
	flag.BoolVar(&a.keepAlive, "keepAlive", true, "Do HTTP keep-alive")
	flag.BoolVar(&a.printResult, "v", false, "Print http request result")
	flag.BoolVar(&a.uploadRandImg, "randomPng", false, "Upload random png images by file upload")
	flag.StringVar(&a.fixedImgSize, "fixedImgSize", "", "Upload fixed img size (eg. 44kB, 17MB)")
	flag.StringVar(&a.postDataFilePath, "postDataFile", "", "HTTP POST data file path")
	flag.StringVar(&a.uploadFilePath, "f", "", "HTTP upload file path")
	flag.StringVar(&a.duration, "d", "0s", "Duration of time (eg 10s, 10m, 2h45m)")
	flag.UintVar(&a.writeTimeout, "writeTimeout", 5000, "Write timeout (in milliseconds)")
	flag.UintVar(&a.readTimeout, "readTimeout", 5000, "Read timeout (in milliseconds)")
	flag.StringVar(&a.authHeader, "authHeader", "", "Authorization header")
	flag.StringVar(&a.uploadFileName, "fileName", "file", "Upload file name")
	flag.StringVar(&a.contentType, "contentType", "", "Content-Type, eg, json, plain, or other full name")

	flag.Parse()

	rand.Seed(time.Now().UnixNano())
}

const methodPOST = "POST"

func main() {
	var app App

	app.Init()

	startTime := time.Now()

	HandleInterrupt(func() {
		app.exitRequested = true
	}, false)
	HandleMaxProcs()

	c := app.NewConfiguration()
	fmt.Printf("Dispatching %d threads (goroutines)\n", app.threads)

	resultChan := make(chan requestResult)
	totalReqsChan := make(chan int)
	statComplete := make(chan bool)

	var rr requestResult
	go stating(resultChan, &rr, totalReqsChan, statComplete)

	requestsChan := make(chan int, app.threads)

	for i := 0; i < app.threads; i++ {
		go app.client(requestsChan, resultChan, c)
	}

	fmt.Println("Waiting for results...")

	totalRequests := app.waitResults(requestsChan, totalReqsChan, statComplete)

	app.printResults(startTime, totalRequests, rr)
}

func (a *App) printResults(startTime time.Time, totalRequests int, rr requestResult) {
	elapsed := time.Since(startTime)
	elapsedSeconds := elapsed.Seconds()

	fmt.Println()
	fmt.Printf("Requests:                   %10d hits\n", totalRequests)
	fmt.Printf("Successful requests:        %10d hits\n", rr.success)
	fmt.Printf("Network failed:             %10d hits\n", rr.networkFailed)
	fmt.Printf("Bad requests failed (!2xx): %10d hits\n", rr.badFailed)
	fmt.Printf("Successful requests rate:   %10d hits/sec\n", uint64(float64(rr.success)/elapsedSeconds))
	fmt.Printf("Read throughput:            %10s/sec\n",
		humanize.IBytes(uint64(float64(a.readThroughput)/elapsedSeconds)))
	fmt.Printf("Write throughput:           %10s/sec\n",
		humanize.IBytes(uint64(float64(a.writeThroughput)/elapsedSeconds)))
	fmt.Printf("Test time:                  %10s\n", elapsed.String())
}

func (a *App) waitResults(requestsChan chan int, totalReqsChan chan int, statComplete chan bool) int {
	totalRequests := 0
	for i := 0; i < a.threads; i++ {
		totalRequests += <-requestsChan
	}

	totalReqsChan <- totalRequests

	<-statComplete

	return totalRequests
}

func stating(resultChan chan requestResult, rr *requestResult, totalReqsChan chan int, statComplete chan bool) {
	var received int

	for {
		select {
		case r := <-resultChan:
			received++
			rr.success += r.success
			rr.networkFailed += r.networkFailed
			rr.badFailed += r.badFailed
		case totalReqs := <-totalReqsChan:
			for i := 0; i < totalReqs-received; i++ {
				r := <-resultChan
				rr.success += r.success
				rr.networkFailed += r.networkFailed
				rr.badFailed += r.badFailed
			}

			statComplete <- true

			return
		}
	}
}

// NewConfiguration create Conf
func (a *App) NewConfiguration() (c *Conf) {
	a.connectionChan = make(chan bool, a.connections)
	for i := 0; i < a.connections; i++ {
		a.connectionChan <- true
	}

	c = &Conf{
		urls:       make([]string, 0),
		method:     "GET",
		postData:   nil,
		keepAlive:  a.keepAlive,
		requests:   a.requests,
		authHeader: a.authHeader,
	}

	a.period(c)
	a.processUrls(c)
	a.dealPostDataFilePath(c)
	a.dealUploadFilePath(c)

	c.myClient.ReadTimeout = time.Duration(a.readTimeout) * time.Millisecond
	c.myClient.WriteTimeout = time.Duration(a.writeTimeout) * time.Millisecond
	// c.myClient.MaxConnsPerHost = connections
	c.myClient.Dial = a.MyDialer()

	if a.contentType != "" {
		c.contentType = a.contentType
	}

	return
}

func (a *App) processUrls(c *Conf) {
	if a.urls == "" {
		log.Fatalf("urls must be provided")
	}

	if strings.Index(a.urls, "@") == 0 {
		urlsFilePath := a.urls[1:]

		var err error

		c.urls, err = FileToLines(urlsFilePath)
		if err != nil {
			log.Fatalf("Error in ioutil.ReadFile for file: %s Error: %v", urlsFilePath, err)
		}
	} else {
		parts := strings.Split(a.urls, ",")
		c.urls = append(c.urls, parts...)
	}
}

func (a *App) dealUploadFilePath(c *Conf) {
	if a.uploadFilePath == "" {
		return
	}

	var err error
	c.method = methodPOST
	c.postData, c.contentType, err = ReadUploadMultipartFile(a.uploadFileName, a.uploadFilePath)
	if err != nil {
		log.Fatalf("Error in ReadUploadMultipartFile for file path: %s Error: %v", a.uploadFilePath, err)
	}
}

func (a *App) dealPostDataFilePath(c *Conf) {
	if a.postDataFilePath == "" {
		return
	}

	var err error

	c.method = methodPOST
	c.postData, err = ioutil.ReadFile(a.postDataFilePath)
	if err != nil {
		log.Fatalf("Error in ioutil.ReadFile for file path: %s Error: %v", a.postDataFilePath, err)
	}

	if a.contentType == "" {
		firstByte := c.postData[0]
		if firstByte == '{' || firstByte == '[' {
			c.contentType = "application/json; charset=utf-8"
		}
	}
}

func (a *App) period(c *Conf) {
	period, err := time.ParseDuration(a.duration)
	if err != nil {
		log.Fatal(err)
	}

	if a.requests == 0 && period == 0 {
		log.Fatalf("requests or duration must be provided")
	}

	if a.requests != 0 && period != 0 {
		log.Fatalf("only one should be provided: [requests|duration]")
	}

	if period == 0 {
		return
	}

	c.duration = period

	timeout := make(chan bool, 1)

	go func() {
		<-time.After(period)
		timeout <- true
	}()

	go func() {
		<-timeout
		pid := os.Getpid()
		proc, _ := os.FindProcess(pid)
		err := proc.Signal(os.Interrupt)
		if err != nil {
			log.Println(err)
			return
		}
	}()
}

func (a *App) randomImage(imageSize string) (imageBytes []byte, contentType, imageFile string) {
	var err error

	var size int64

	if imageSize == "" {
		size = (rand.Int63n(4) + 1) << 20 //  << 20 means MiB
	} else {
		size, err = units.FromHumanSize(a.fixedImgSize)
		if err != nil {
			fmt.Println("error fixedImgSize " + err.Error())
			panic(err)
		}
	}

	randText := strconv.FormatUint(randimg.RandUint64(), 10)
	imageFile = randText + ".png"
	randimg.GenerateRandomImageFile(640, 320, randText, imageFile, size)

	defer os.Remove(imageFile)

	log.Println("create random image", imageFile, "in size", units.HumanSize(float64(size)))

	imageBytes, contentType, _ = ReadUploadMultipartFile(a.uploadFileName, imageFile)

	return
}

func (a *App) client(stopChan chan int, resultChan chan requestResult, configuration *Conf) {
	urlIndex := rand.Intn(len(configuration.urls))
	requests := configuration.requests

	i := 0
	for !a.exitRequested && (requests == 0 || i < requests) {
		url := configuration.urls[urlIndex]
		i++
		a.doRequest(resultChan, configuration, url)

		if urlIndex++; urlIndex >= len(configuration.urls) {
			urlIndex = 0
		}
	}

	stopChan <- i
}

func (a *App) doRequest(resultChan chan requestResult, configuration *Conf, url string) {
	method := configuration.method
	postData := configuration.postData
	contentType := configuration.contentType
	fileName := ""

	if a.uploadRandImg || a.fixedImgSize != "" {
		method = "POST"
		postData, contentType, fileName = a.randomImage(a.fixedImgSize)
	}

	<-a.connectionChan

	go a.do(resultChan, configuration, url, method, contentType, fileName, postData)
}

type requestResult struct {
	networkFailed int
	success       int
	badFailed     int
}

func (a *App) do(result chan requestResult, cnf *Conf, url, method, contentType, fileName string, postData []byte) {
	req := fasthttp.AcquireRequest()

	defer func() {
		fasthttp.ReleaseRequest(req)
		a.connectionChan <- true
	}()

	req.SetRequestURI(url)
	req.Header.SetMethod(method)
	SetHeaderIfNotEmpty(req, "Connection", IfElse(cnf.keepAlive, "keep-alive", "close"))
	SetHeaderIfNotEmpty(req, "Authorization", cnf.authHeader)
	SetHeaderIfNotEmpty(req, "Content-Type", contentType)
	req.SetBody(postData)

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	err := cnf.myClient.Do(req, resp)
	statusCode := resp.StatusCode()

	var rr requestResult

	var resultDesc string

	switch {
	case err != nil:
		rr.networkFailed = 1
		resultDesc = "network failed bcoz " + err.Error()
	case statusCode < 400:
		rr.success = 1
		resultDesc = "success"
	default:
		rr.badFailed = 1
		resultDesc = "failed"
	}

	if a.printResult {
		fmt.Fprintln(os.Stdout, "fileName", fileName, resultDesc, statusCode, string(resp.Body()))
	}

	result <- rr
}

// SetHeaderIfNotEmpty set request header if value is not empty.
func SetHeaderIfNotEmpty(request *fasthttp.Request, header, value string) {
	if value != "" {
		request.Header.Set(header, value)
	}
}

// MyConn for net connection
type MyConn struct {
	net.Conn
	app *App
}

// Read bytes from net connection
func (myConn *MyConn) Read(b []byte) (n int, err error) {
	if n, err = myConn.Conn.Read(b); err == nil {
		atomic.AddUint64(&myConn.app.readThroughput, uint64(n))
	}

	return
}

// Write bytes to net
func (myConn *MyConn) Write(b []byte) (n int, err error) {
	if n, err = myConn.Conn.Write(b); err == nil {
		atomic.AddUint64(&myConn.app.writeThroughput, uint64(n))
	}

	return
}

// MyDialer create Dial function
func (a *App) MyDialer() func(address string) (conn net.Conn, err error) {
	return func(address string) (net.Conn, error) {
		conn, err := net.Dial("tcp", address)
		if err != nil {
			return nil, err
		}

		return &MyConn{Conn: conn, app: a}, nil
	}
}

// HandleInterrupt handles Ctrl+C to exit after calling f.
func HandleInterrupt(f func(), exitRightNow bool) {
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, os.Interrupt)

	go func() {
		<-ch
		f()

		if exitRightNow {
			os.Exit(0)
		}
	}()
}

// HandleMaxProcs process GOMAXPROCS.
func HandleMaxProcs() {
	goMaxProcs := os.Getenv("GOMAXPROCS")
	if goMaxProcs == "" {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}
}

// IfElse return then if condition is true,  else els if false
func IfElse(condition bool, then, els string) string {
	if condition {
		return then
	}

	return els
}

// FileToLines read file into lines
func FileToLines(filePath string) (lines []string, err error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	return lines, scanner.Err()
}

// MustOpen open file successfully or panic
func MustOpen(f string) *os.File {
	r, err := os.Open(f)
	if err != nil {
		panic(err)
	}

	return r
}

// ReadUploadMultipartFile read file filePath for upload in multipart,
// return multipart content, form data content type and error
func ReadUploadMultipartFile(filename, filePath string) (imageBytes []byte, contentType string, err error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

	part, err := writer.CreateFormFile(filename, filepath.Base(filePath))
	if err != nil {
		return nil, "", err
	}

	file := MustOpen(filePath)
	defer file.Close()

	_, _ = io.Copy(part, file)

	_ = writer.Close()

	return buffer.Bytes(), writer.FormDataContentType(), nil
}
