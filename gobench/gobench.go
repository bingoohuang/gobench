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
	uploddRandImg    bool
	writeTimeout     int
	readTimeout      int
	authHeader       string
	uploadFileName   string
	urlsRandRobin    bool
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

var readThroughput int64
var writeThroughput int64

// MyConn for net connection
type MyConn struct {
	net.Conn
}

// Read bytes from net connection
func (myConn *MyConn) Read(b []byte) (n int, err error) {
	len, err := myConn.Conn.Read(b)

	if err == nil {
		atomic.AddInt64(&readThroughput, int64(len))
	}

	return len, err
}

// Write bytes to net
func (myConn *MyConn) Write(b []byte) (n int, err error) {
	len, err := myConn.Conn.Write(b)

	if err == nil {
		atomic.AddInt64(&writeThroughput, int64(len))
	}

	return len, err
}

func init() {
	flag.Int64Var(&requests, "r", -1, "Number of requests per client")
	flag.IntVar(&clients, "c", 100, "Number of concurrent clients")
	flag.StringVar(&urls, "u", "", "URL list (comma seperated)")
	flag.StringVar(&urlsFilePath, "uf", "", "URL's file path (line seperated)")
	flag.BoolVar(&urlsRandRobin, "ur", true, "select url in Rand-Robin ")
	flag.BoolVar(&keepAlive, "k", true, "Do HTTP keep-alive")
	flag.BoolVar(&uploddRandImg, "fr", false, "Upload random png images by file upload")
	flag.StringVar(&postDataFilePath, "d", "", "HTTP POST data file path")
	flag.StringVar(&uploadFilePath, "fp", "", "HTTP upload file path")
	flag.Int64Var(&period, "t", -1, "Period of time (in seconds)")
	flag.IntVar(&writeTimeout, "tw", 5000, "Write timeout (in milliseconds)")
	flag.IntVar(&readTimeout, "tr", 5000, "Read timeout (in milliseconds)")
	flag.StringVar(&authHeader, "auth", "", "Authorization header")
	flag.StringVar(&uploadFileName, "fn", "file", "Upload file name")

	flag.Parse()
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

func readLines(path string) (lines []string, err error) {
	var file *os.File
	var part []byte
	var prefix bool

	if file, err = os.Open(path); err != nil {
		return
	}
	defer file.Close()

	reader := bufio.NewReader(file)
	buffer := bytes.NewBuffer(make([]byte, 0))
	for {
		if part, prefix, err = reader.ReadLine(); err != nil {
			break
		}
		buffer.Write(part)
		if !prefix {
			lines = append(lines, buffer.String())
			buffer.Reset()
		}
	}
	if err == io.EOF {
		err = nil
	}
	return
}

// NewConfiguration create Configuration
func NewConfiguration() *Configuration {
	if urlsFilePath == "" && urls == "" {
		flag.Usage()
		os.Exit(1)
	}

	if requests == -1 && period == -1 {
		fmt.Println("Requests or period must be provided")
		flag.Usage()
		os.Exit(1)
	}

	if requests != -1 && period != -1 {
		fmt.Println("Only one should be provided: [requests|period]")
		flag.Usage()
		os.Exit(1)
	}

	configuration := &Configuration{
		urls:       make([]string, 0),
		method:     "GET",
		postData:   nil,
		keepAlive:  keepAlive,
		requests:   requests,
		authHeader: authHeader}

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
		fileLines, err := readLines(urlsFilePath)

		if err != nil {
			log.Fatalf("Error in ioutil.ReadFile for file: %s Error: %v", urlsFilePath, err)
		}

		configuration.urls = fileLines
	}

	if urls != "" {
		parts := strings.Split(urls, ",")
		configuration.urls = append(configuration.urls, parts...)
	}

	if postDataFilePath != "" {
		configuration.method = "POST"
		data, err := ioutil.ReadFile(postDataFilePath)
		if err != nil {
			log.Fatalf("Error in ioutil.ReadFile for file path: %s Error: %v", postDataFilePath, err)
		}

		configuration.postData = data
	}

	if uploadFilePath != "" {
		configuration.method = "POST"
		var err error
		configuration.postData, configuration.contentType, err = ReadUploadMultipartFile(uploadFileName, uploadFilePath)
		if err != nil {
			log.Fatalf("Error in ReadUploadMultipartFile for file path: %s Error: %v", uploadFilePath, err)
		}
	}

	configuration.myClient.ReadTimeout = time.Duration(readTimeout) * time.Millisecond
	configuration.myClient.WriteTimeout = time.Duration(writeTimeout) * time.Millisecond
	configuration.myClient.MaxConnsPerHost = clients

	configuration.myClient.Dial = MyDialer()

	return configuration
}

// MyDialer create Dial function
func MyDialer() func(address string) (conn net.Conn, err error) {
	return func(address string) (net.Conn, error) {
		conn, err := net.Dial("tcp", address)
		if err != nil {
			return nil, err
		}

		myConn := &MyConn{Conn: conn}
		return myConn, nil
	}
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
func ReadUploadMultipartFile(filename, filePath string) ([]byte, string, error) {
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

func init() {
	rand.Seed(time.Now().UnixNano())
}

func randomImage() ([]byte, string, error) {
	seq := randimg.RandUint64()
	randText := strconv.FormatUint(seq, 10)
	size := rand.Intn(4) + 1
	imageFile := randText + ".png"
	randimg.GenerateRandomImageFile(640, 320, randText, imageFile, size<<20)
	defer os.Remove(imageFile)

	return ReadUploadMultipartFile(uploadFileName, imageFile)
}

func client(configuration *Configuration, result *Result, done *sync.WaitGroup) {
	urlIndex := rand.Intn(len(configuration.urls))
	requests := configuration.requests
	for requests < 0 || result.requests < requests {
		if urlsRandRobin {
			tmpURL := configuration.urls[urlIndex]
			doRequest(configuration, result, tmpURL)

			urlIndex++
			if urlIndex >= len(configuration.urls) {
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
	req := fasthttp.AcquireRequest()

	method := configuration.method
	postData := configuration.postData
	contentType := configuration.contentType

	if uploddRandImg {
		method = "POST"
		postData, contentType, _ = randomImage()
	}

	req.SetRequestURI(tmpURL)
	req.Header.SetMethodBytes([]byte(method))
	req.Header.Set("Connection", IfElse(configuration.keepAlive, "keep-alive", "close"))
	if len(configuration.authHeader) > 0 {
		req.Header.Set("Authorization", configuration.authHeader)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.SetBody(postData)

	resp := fasthttp.AcquireResponse()
	err := configuration.myClient.Do(req, resp)
	statusCode := resp.StatusCode()
	result.requests++
	fasthttp.ReleaseRequest(req)
	fasthttp.ReleaseResponse(resp)

	if err != nil {
		result.networkFailed++
		return
	}

	if statusCode == fasthttp.StatusOK || statusCode == fasthttp.StatusCreated {
		result.success++
	} else {
		result.badFailed++
	}
}

func main() {
	startTime := time.Now()
	results := make(map[int]*Result)

	HandleCtrlC(func() {
		printResults(results, startTime)
	})
	HandleMaxProcs()

	configuration := NewConfiguration()

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

// HandleCtrlC handles Ctrl+C to exit after calling f.
func HandleCtrlC(f func()) {
	signalChannel := make(chan os.Signal, 2)
	signal.Notify(signalChannel, os.Interrupt)
	go func() {
		_ = <-signalChannel
		f()
		os.Exit(0)
	}()
}

// HandleMaxProcs process GOMAXPROCS.
func HandleMaxProcs() {
	goMaxProcs := os.Getenv("GOMAXPROCS")
	if goMaxProcs == "" {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}
}

func IfElse(condition bool, then, els string) string {
	if condition {
		return then
	}

	return els
}
