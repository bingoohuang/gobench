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
	"sync"
	"sync/atomic"
	"time"

	"github.com/bingoohuang/golang-trial/randimg"
	humanize "github.com/dustin/go-humanize"

	"github.com/valyala/fasthttp"
)

var (
	requests         uint64
	duraton          string
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

	printResult bool
)

// Configuration for gobench
type Configuration struct {
	urls        []string
	method      string
	postData    []byte
	requests    uint64
	duraton     time.Duration
	keepAlive   bool
	authHeader  string
	contentType string

	myClient fasthttp.Client
}

// Result of gobench
type Result struct {
	requests      uint64
	success       uint64
	networkFailed uint64
	badFailed     uint64
}

func init() {
	flag.Uint64Var(&requests, "r", 0, "Number of requests per client")
	flag.IntVar(&connections, "c", 100, "Number of connections")
	flag.IntVar(&threads, "t", 100, "Number of concurrent threads")
	flag.StringVar(&urls, "u", "", "URL list (comma separated), or @URL's file path (line separated)")
	flag.BoolVar(&keepAlive, "keepAlive", true, "Do HTTP keep-alive")
	flag.BoolVar(&printResult, "v", false, "Print http request result")
	flag.BoolVar(&uploadRandImg, "randomPng", false, "Upload random png images by file upload")
	flag.StringVar(&postDataFilePath, "postDataFile", "", "HTTP POST data file path")
	flag.StringVar(&uploadFilePath, "f", "", "HTTP upload file path")
	flag.StringVar(&duraton, "d", "0s", "Duration of time (eg 10s, 10m, 2h45m)")
	flag.UintVar(&writeTimeout, "writeTimeout", 5000, "Write timeout (in milliseconds)")
	flag.UintVar(&readTimeout, "readTimeout", 5000, "Read timeout (in milliseconds)")
	flag.StringVar(&authHeader, "authHeader", "", "Authorization header")
	flag.StringVar(&uploadFileName, "fileName", "file", "Upload file name")

	flag.Parse()

	rand.Seed(time.Now().UnixNano())
}

func main() {
	startTime := time.Now()
	results := make(map[int]*Result)

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

	var done sync.WaitGroup
	done.Add(threads)
	for i := 0; i < threads; i++ {
		result := &Result{}
		results[i] = result
		go client(configuration, result, &done)
	}

	fmt.Println("Waiting for results...")
	done.Wait()
	printResults(results, startTime)
}

func printResults(results map[int]*Result, startTime time.Time) {
	var requests uint64
	var success uint64
	var networkFailed uint64
	var badFailed uint64

	for _, result := range results {
		requests += result.requests
		success += result.success
		networkFailed += result.networkFailed
		badFailed += result.badFailed
	}

	elapsed := time.Since(startTime)
	if elapsed == 0 {
		elapsed = 1
	}

	elapsedSeconds := uint64(elapsed.Seconds())

	fmt.Println()
	fmt.Printf("Requests:                       %10d hits\n", requests)
	fmt.Printf("Successful requests:            %10d hits\n", success)
	fmt.Printf("Network failed:                 %10d hits\n", networkFailed)
	fmt.Printf("Bad requests failed (!2xx):     %10d hits\n", badFailed)
	fmt.Printf("Successful requests rate:       %10d hits/sec\n", success/elapsedSeconds)
	fmt.Printf("Read throughput:                %10s/sec\n", humanize.IBytes(readThroughput/elapsedSeconds))
	fmt.Printf("Write throughput:               %10s/sec\n", humanize.IBytes(writeThroughput/elapsedSeconds))
	fmt.Printf("Test time:                      %10s\n", elapsed.String())
}

var conectionChan chan bool

// NewConfiguration create Configuration
func NewConfiguration() (configuration *Configuration, err error) {
	if urls == "" {
		return nil, errors.New("urls must be provided")
	}

	period, err := time.ParseDuration(duraton)
	if err != nil {
		return nil, err
	}

	if requests == 0 && period == 0 {
		return nil, errors.New("requests or duraton must be provided")
	}
	if requests != 0 && period != 0 {
		return nil, errors.New("only one should be provided: [requests|duraton]")
	}

	conectionChan = make(chan bool, connections)
	for i := 0; i < connections; i++ {
		conectionChan <- true
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
		configuration.duraton = period

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

func randomImage() (imageBytes []byte, contentType, imageFile string, err error) {
	randText := strconv.FormatUint(randimg.RandUint64(), 10)
	size := rand.Int63n(4) + 1
	imageFile = randText + ".png"
	randimg.GenerateRandomImageFile(640, 320, randText, imageFile, size<<20)
	defer os.Remove(imageFile)

	log.Println("create random image", imageFile, "in size", size, "MiB")

	imageBytes, contentType, err = ReadUploadMultipartFile(uploadFileName, imageFile)
	return
}

func client(configuration *Configuration, result *Result, done *sync.WaitGroup) {
	defer done.Done()

	urlIndex := rand.Intn(len(configuration.urls))
	requests := configuration.requests
	for !exitRequested && (requests == 0 || result.requests < requests) {
		url := configuration.urls[urlIndex]
		doRequest(configuration, result, url)

		if urlIndex++; urlIndex >= len(configuration.urls) {
			urlIndex = 0
		}
	}
}

func doRequest(configuration *Configuration, result *Result, url string) {
	method := configuration.method
	postData := configuration.postData
	contentType := configuration.contentType
	fileName := ""

	if uploadRandImg {
		method = "POST"
		postData, contentType, fileName, _ = randomImage()
	}

	<-conectionChan
	go goRequest(result, configuration, url, method, contentType, fileName, postData)
}

func goRequest(result *Result, configuration *Configuration, url, method, contentType, fileName string, postData []byte) {
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	defer func() {
		conectionChan <- true
	}()

	req.SetRequestURI(url)
	req.Header.SetMethod(method)
	SetHeaderIfNotEmpty(req, "Connection", IfElse(configuration.keepAlive, "keep-alive", "close"))
	SetHeaderIfNotEmpty(req, "Authorization", configuration.authHeader)
	SetHeaderIfNotEmpty(req, "Content-Type", contentType)
	req.SetBody(postData)

	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(resp)

	err := configuration.myClient.Do(req, resp)
	statusCode := resp.StatusCode()
	atomic.AddUint64(&result.requests, 1)

	var resultDesc string
	if err != nil {
		atomic.AddUint64(&result.networkFailed, 1)
		resultDesc = "network failed bcoz " + err.Error()
	} else if statusCode < 400 {
		atomic.AddUint64(&result.success, 1)
		resultDesc = "succuess"
	} else {
		atomic.AddUint64(&result.badFailed, 1)
		resultDesc = "failed"
	}

	if printResult {
		fmt.Fprintln(os.Stdout, "fileName", fileName, resultDesc, statusCode, string(resp.Body()))
	}
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
	n, err = myConn.Conn.Read(b)
	if err == nil {
		atomic.AddUint64(&readThroughput, uint64(n))
	}

	return
}

// Write bytes to net
func (myConn *MyConn) Write(b []byte) (n int, err error) {
	n, err = myConn.Conn.Write(b)
	if err == nil {
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
		_ = <-ch
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
