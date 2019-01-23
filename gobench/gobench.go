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
	"net/http"
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

	"github.com/valyala/fasthttp"
)

var (
	requests         int64
	period           int64
	clients          int
	urls             string
	urlsFilePath     string
	keepAlive        bool
	postDataFilePath string
	uploadFilePath   string
	uploadRandImg    bool
	writeTimeout     int
	readTimeout      int
	authHeader       string
	uploadFileName   string
	urlsRandRobin    bool
	exitRequested    bool

	printSuccess bool
)

// Configuration for gobench
type Configuration struct {
	urls        []string
	method      string
	postData    []byte
	requests    int64
	period      int64
	keepAlive   bool
	authHeader  string
	contentType string

	myClient fasthttp.Client
}

// Result of gobench
type Result struct {
	requests      int64
	success       int64
	networkFailed int64
	badFailed     int64
}

func init() {
	flag.Int64Var(&requests, "r", -1, "Number of requests per client")
	flag.IntVar(&clients, "c", 100, "Number of concurrent clients")
	flag.StringVar(&urls, "u", "", "URL list (comma separated)")
	flag.StringVar(&urlsFilePath, "uf", "", "URL's file path (line separated)")
	flag.BoolVar(&urlsRandRobin, "ur", true, "select url in Rand-Robin ")
	flag.BoolVar(&keepAlive, "k", true, "Do HTTP keep-alive")
	flag.BoolVar(&printSuccess, "ps", true, "Print when http request successfully")
	flag.BoolVar(&uploadRandImg, "fr", false, "Upload random png images by file upload")
	flag.StringVar(&postDataFilePath, "d", "", "HTTP POST data file path")
	flag.StringVar(&uploadFilePath, "fp", "", "HTTP upload file path")
	flag.Int64Var(&period, "t", -1, "Period of time (in seconds)")
	flag.IntVar(&writeTimeout, "tw", 5000, "Write timeout (in milliseconds)")
	flag.IntVar(&readTimeout, "tr", 5000, "Read timeout (in milliseconds)")
	flag.StringVar(&authHeader, "a", "", "Authorization header")
	flag.StringVar(&uploadFileName, "fn", "file", "Upload file name")

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

	fmt.Printf("Dispatching %d clients\n", clients)

	var done sync.WaitGroup
	done.Add(clients)
	for i := 0; i < clients; i++ {
		result := &Result{}
		results[i] = result
		go client(configuration, result, &done)
	}

	fmt.Println("Waiting for results...")
	done.Wait()
	printResults(results, startTime)
}

func printResults(results map[int]*Result, startTime time.Time) {
	var requests int64
	var success int64
	var networkFailed int64
	var badFailed int64

	for _, result := range results {
		requests += result.requests
		success += result.success
		networkFailed += result.networkFailed
		badFailed += result.badFailed
	}

	elapsed := int64(time.Since(startTime).Seconds())
	if elapsed == 0 {
		elapsed = 1
	}

	fmt.Println()
	fmt.Printf("Requests:                       %10d hits\n", requests)
	fmt.Printf("Successful requests:            %10d hits\n", success)
	fmt.Printf("Network failed:                 %10d hits\n", networkFailed)
	fmt.Printf("Bad requests failed (!2xx):     %10d hits\n", badFailed)
	fmt.Printf("Successful requests rate:       %10d hits/sec\n", success/elapsed)
	fmt.Printf("Read throughput:                %10d bytes/sec\n", readThroughput/elapsed)
	fmt.Printf("Write throughput:               %10d bytes/sec\n", writeThroughput/elapsed)
	fmt.Printf("Test time:                      %10d sec\n", elapsed)
}

// NewConfiguration create Configuration
func NewConfiguration() (configuration *Configuration, err error) {
	if urlsFilePath == "" && urls == "" {
		return nil, errors.New("urls or urlsFilePath must be provided")
	}
	if requests == -1 && period == -1 {
		return nil, errors.New("requests or period must be provided")
	}
	if requests != -1 && period != -1 {
		return nil, errors.New("only one should be provided: [requests|period]")
	}

	configuration = &Configuration{
		urls:       make([]string, 0),
		method:     "GET",
		postData:   nil,
		keepAlive:  keepAlive,
		requests:   requests,
		authHeader: authHeader,
	}

	if period != -1 {
		configuration.period = period

		timeout := make(chan bool, 1)
		go func() {
			<-time.After(time.Duration(period) * time.Second)
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

	if urlsFilePath != "" {
		configuration.urls, err = FileToLines(urlsFilePath)

		if err != nil {
			log.Fatalf("Error in ioutil.ReadFile for file: %s Error: %v", urlsFilePath, err)
		}
	}

	if urls != "" {
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
	configuration.myClient.MaxConnsPerHost = clients
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
	urlIndex := rand.Intn(len(configuration.urls))
	requests := configuration.requests
	for !exitRequested && (requests < 0 || result.requests < requests) {
		if urlsRandRobin {
			tmpURL := configuration.urls[urlIndex]
			doRequest(configuration, result, tmpURL)

			if urlIndex++; urlIndex >= len(configuration.urls) {
				urlIndex = 0
			}
		} else {
			for _, tmpURL := range configuration.urls {
				doRequest(configuration, result, tmpURL)
			}
		}
	}

	done.Done()
}

func doRequest(configuration *Configuration, result *Result, tmpURL string) {
	method := configuration.method
	postData := configuration.postData
	contentType := configuration.contentType
	fileName := ""

	if uploadRandImg {
		method = "POST"
		postData, contentType, fileName, _ = randomImage()
	}

	req := fasthttp.AcquireRequest()
	req.SetRequestURI(tmpURL)
	req.Header.SetMethod(method)
	SetHeaderIfNotEmpty(req, "Connection", IfElse(configuration.keepAlive, "keep-alive", "close"))
	SetHeaderIfNotEmpty(req, "Authorization", configuration.authHeader)
	SetHeaderIfNotEmpty(req, "Content-Type", contentType)
	req.SetBody(postData)

	resp := fasthttp.AcquireResponse()
	err := configuration.myClient.Do(req, resp)
	statusCode := resp.StatusCode()
	result.requests++

	if err != nil {
		result.networkFailed++
		fmt.Fprintln(os.Stderr, "networkFailed", err.Error())
	} else if statusCode < http.StatusOK || statusCode > http.StatusIMUsed {
		result.badFailed++
		fmt.Fprintln(os.Stderr, "fileName", fileName, "failed", statusCode, string(resp.Body()))
	} else {
		result.success++
		fmt.Fprintln(os.Stderr, "fileName", fileName, "success", statusCode, string(resp.Body()))
	}

	fasthttp.ReleaseRequest(req)
	fasthttp.ReleaseResponse(resp)
}

// SetHeaderIfNotEmpty set request header if value is not empty.
func SetHeaderIfNotEmpty(request *fasthttp.Request, header, value string) {
	if value != "" {
		request.Header.Set(header, value)
	}
}

var readThroughput int64
var writeThroughput int64

// MyConn for net connection
type MyConn struct {
	net.Conn
}

// Read bytes from net connection
func (myConn *MyConn) Read(b []byte) (n int, err error) {
	n, err = myConn.Conn.Read(b)
	if err == nil {
		atomic.AddInt64(&readThroughput, int64(n))
	}

	return
}

// Write bytes to net
func (myConn *MyConn) Write(b []byte) (n int, err error) {
	n, err = myConn.Conn.Write(b)
	if err == nil {
		atomic.AddInt64(&writeThroughput, int64(n))
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
