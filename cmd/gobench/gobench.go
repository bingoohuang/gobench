package main

import (
	"bufio"
	"bytes"
	"errors"
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

var (
	requests         int
	duration         string
	threads          int
	connections      int
	urls             string
	keepAlive        bool
	postDataFilePath string
	uploadFilePath   string
	uploadRandImg    bool
	writeTimeout     uint
	readTimeout      uint
	authHeader       string
	uploadFileName   string
	exitRequested    bool
	fixedImgSize     string

	printResult bool
)

// Configuration for gobench
type Configuration struct {
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

func init() {
	flag.IntVar(&requests, "r", 0, "Number of requests per client")
	flag.IntVar(&connections, "c", 100, "Number of connections")
	flag.IntVar(&threads, "t", 100, "Number of concurrent threads")
	flag.StringVar(&urls, "u", "", "URL list (comma separated), or @URL's file path (line separated)")
	flag.BoolVar(&keepAlive, "keepAlive", true, "Do HTTP keep-alive")
	flag.BoolVar(&printResult, "v", false, "Print http request result")
	flag.BoolVar(&uploadRandImg, "randomPng", false, "Upload random png images by file upload")
	flag.StringVar(&fixedImgSize, "fixedImgSize", "", "Upload fixed img size (eg. 44kB, 17MB)")
	flag.StringVar(&postDataFilePath, "postDataFile", "", "HTTP POST data file path")
	flag.StringVar(&uploadFilePath, "f", "", "HTTP upload file path")
	flag.StringVar(&duration, "d", "0s", "Duration of time (eg 10s, 10m, 2h45m)")
	flag.UintVar(&writeTimeout, "writeTimeout", 5000, "Write timeout (in milliseconds)")
	flag.UintVar(&readTimeout, "readTimeout", 5000, "Read timeout (in milliseconds)")
	flag.StringVar(&authHeader, "authHeader", "", "Authorization header")
	flag.StringVar(&uploadFileName, "fileName", "file", "Upload file name")

	flag.Parse()

	rand.Seed(time.Now().UnixNano())
}

func main() {
	startTime := time.Now()

	HandleInterrupt(func() {
		exitRequested = true
	}, false)
	HandleMaxProcs()

	configuration, err := NewConfiguration()
	if err != nil {
		fmt.Println(err.Error())
		flag.Usage()

		return
	}

	fmt.Printf("Dispatching %d threads (goroutines)\n", threads)

	resultChan := make(chan requestResult)
	totalReqsChan := make(chan int)
	statComplete := make(chan bool)

	var rr requestResult
	go stating(resultChan, &rr, totalReqsChan, statComplete)

	requestsChan := make(chan int, threads)

	for i := 0; i < threads; i++ {
		go client(requestsChan, resultChan, configuration)
	}

	fmt.Println("Waiting for results...")

	totalRequests := waitResults(requestsChan, totalReqsChan, statComplete)

	printResults(startTime, totalRequests, rr)
}

func printResults(startTime time.Time, totalRequests int, rr requestResult) {
	elapsed := time.Since(startTime)
	elapsedSeconds := elapsed.Seconds()

	fmt.Println()
	fmt.Printf("Requests:                       %10d hits\n", totalRequests)
	fmt.Printf("Successful requests:            %10d hits\n", rr.success)
	fmt.Printf("Network failed:                 %10d hits\n", rr.networkFailed)
	fmt.Printf("Bad requests failed (!2xx):     %10d hits\n", rr.badFailed)
	fmt.Printf("Successful requests rate:       %10d hits/sec\n", uint64(float64(rr.success)/elapsedSeconds))
	fmt.Printf("Read throughput:                %10s/sec\n", humanize.IBytes(uint64(float64(readThroughput)/elapsedSeconds)))
	fmt.Printf("Write throughput:               %10s/sec\n", humanize.IBytes(uint64(float64(writeThroughput)/elapsedSeconds)))
	fmt.Printf("Test time:                      %10s\n", elapsed.String())
}

func waitResults(requestsChan chan int, totalReqsChan chan int, statComplete chan bool) int {
	totalRequests := 0
	for i := 0; i < threads; i++ {
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

var connectionChan chan bool

// NewConfiguration create Configuration
func NewConfiguration() (configuration *Configuration, err error) {
	if urls == "" {
		return nil, errors.New("urls must be provided")
	}

	period, err := time.ParseDuration(duration)
	if err != nil {
		return nil, err
	}

	if requests == 0 && period == 0 {
		return nil, errors.New("requests or duration must be provided")
	}
	if requests != 0 && period != 0 {
		return nil, errors.New("only one should be provided: [requests|duration]")
	}

	connectionChan = make(chan bool, connections)
	for i := 0; i < connections; i++ {
		connectionChan <- true
	}

	configuration = &Configuration{
		urls:       make([]string, 0),
		method:     "GET",
		postData:   nil,
		keepAlive:  keepAlive,
		requests:   requests,
		authHeader: authHeader,
	}

	if period != 0 {
		configuration.duration = period

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

	if strings.Index(urls, "@") == 0 {
		urlsFilePath := urls[1:]
		configuration.urls, err = FileToLines(urlsFilePath)

		if err != nil {
			log.Fatalf("Error in ioutil.ReadFile for file: %s Error: %v", urlsFilePath, err)
		}
	} else {
		parts := strings.Split(urls, ",")
		configuration.urls = append(configuration.urls, parts...)
	}

	if postDataFilePath != "" {
		configuration.method = "POST"
		configuration.postData, err = ioutil.ReadFile(postDataFilePath)
		if err != nil {
			log.Fatalf("Error in ioutil.ReadFile for file path: %s Error: %v", postDataFilePath, err)
		}
	}

	if uploadFilePath != "" {
		configuration.method = "POST"
		configuration.postData, configuration.contentType, err = ReadUploadMultipartFile(uploadFileName, uploadFilePath)
		if err != nil {
			log.Fatalf("Error in ReadUploadMultipartFile for file path: %s Error: %v", uploadFilePath, err)
		}
	}

	configuration.myClient.ReadTimeout = time.Duration(readTimeout) * time.Millisecond
	configuration.myClient.WriteTimeout = time.Duration(writeTimeout) * time.Millisecond
	// configuration.myClient.MaxConnsPerHost = connections
	configuration.myClient.Dial = MyDialer()
	return
}

func randomImage(imageSize string) (imageBytes []byte, contentType, imageFile string, err error) {
	var size int64
	if imageSize == "" {
		size = (rand.Int63n(4) + 1) << 20 //  << 20 means MiB
	} else {
		size, err = units.FromHumanSize(fixedImgSize)
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

	imageBytes, contentType, err = ReadUploadMultipartFile(uploadFileName, imageFile)
	return
}

func client(stopChan chan int, resultChan chan requestResult, configuration *Configuration) {
	urlIndex := rand.Intn(len(configuration.urls))
	requests := configuration.requests

	i := 0
	for ; !exitRequested && (requests == 0 || i < requests); {
		url := configuration.urls[urlIndex]
		i++
		doRequest(resultChan, configuration, url)

		if urlIndex++; urlIndex >= len(configuration.urls) {
			urlIndex = 0
		}
	}

	stopChan <- i
}

func doRequest(resultChan chan requestResult, configuration *Configuration, url string) {
	method := configuration.method
	postData := configuration.postData
	contentType := configuration.contentType
	fileName := ""

	if uploadRandImg || fixedImgSize != "" {
		method = "POST"
		postData, contentType, fileName, _ = randomImage(fixedImgSize)
	}

	<-connectionChan
	go goRequest(resultChan, configuration, url, method, contentType, fileName, postData)
}

type requestResult struct {
	networkFailed int
	success       int
	badFailed     int
}

func goRequest(resultChan chan requestResult, cnf *Configuration, url, method, contentType, fileName string, postData []byte) {
	req := fasthttp.AcquireRequest()
	defer func() {
		fasthttp.ReleaseRequest(req)
		connectionChan <- true
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
	if err != nil {
		rr.networkFailed = 1
		resultDesc = "network failed bcoz " + err.Error()
	} else if statusCode < 400 {
		rr.success = 1
		resultDesc = "success"
	} else {
		rr.badFailed = 1
		resultDesc = "failed"
	}

	if printResult {
		fmt.Fprintln(os.Stdout, "fileName", fileName, resultDesc, statusCode, string(resp.Body()))
	}

	resultChan <- rr
}

// SetHeaderIfNotEmpty set request header if value is not empty.
func SetHeaderIfNotEmpty(request *fasthttp.Request, header, value string) {
	if value != "" {
		request.Header.Set(header, value)
	}
}

var readThroughput uint64
var writeThroughput uint64

// MyConn for net connection
type MyConn struct {
	net.Conn
}

// Read bytes from net connection
func (myConn *MyConn) Read(b []byte) (n int, err error) {
	if n, err = myConn.Conn.Read(b); err == nil {
		atomic.AddUint64(&readThroughput, uint64(n))
	}

	return
}

// Write bytes to net
func (myConn *MyConn) Write(b []byte) (n int, err error) {
	if n, err = myConn.Conn.Write(b); err == nil {
		atomic.AddUint64(&writeThroughput, uint64(n))
	}
	return
}

// MyDialer create Dial function
func MyDialer() func(address string) (conn net.Conn, err error) {
	return func(address string) (net.Conn, error) {
		conn, err := net.Dial("tcp", address)
		if err != nil {
			return nil, err
		}

		return &MyConn{Conn: conn}, nil
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

	io.Copy(part, file)
	writer.Close()
	return buffer.Bytes(), writer.FormDataContentType(), nil
}
