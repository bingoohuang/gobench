package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"github.com/karrick/godirwalk"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"text/tabwriter"
	"time"

	"github.com/Knetic/govaluate"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"golang.org/x/net/proxy"

	"github.com/bingoohuang/cweed"
	"github.com/bingoohuang/golang-trial/randimg"
	"github.com/dustin/go-humanize"

	"github.com/valyala/fasthttp"

	"github.com/docker/go-units"
)

// App ...
type App struct {
	requests      int
	requestsTotal int

	goroutines  int
	connections int

	method           string
	duration         string
	urls             string
	postData         string
	postDataFilePath string
	uploadFilePath   string

	keepAlive     bool
	uploadRandImg bool
	exitRequested bool
	printResult   string

	writeTimeout    uint
	readTimeout     uint
	readThroughput  uint64
	writeThroughput uint64

	authHeader     string
	uploadFileName string
	fixedImgSize   string
	contentType    string
	think          string
	proxy          string

	// 返回JSON时的判断是否调用成功的表达式
	cond *govaluate.EvaluableExpression

	connectionChan      chan bool
	responsePrinter     func(s string)
	responsePrinterFile *os.File
	thinkMin            time.Duration
	thinkMax            time.Duration

	weedVolumeAssignedUrl chan string // 海草文件上传路径地址
	exitChan              chan bool
}

// Conf for gobench.
type Conf struct {
	urls        []string
	method      string
	postData    []byte
	requests    int
	duration    time.Duration
	keepAlive   bool
	authHeader  string
	contentType string

	myClient        fasthttp.Client
	firstRequests   int
	postFileChannel chan string
}

const usage = `Usage: gobench [options...]

Options:
  -u               URL list (comma separated), or @URL's file path (line separated)
  -m               HTTP method(GET, POST, PUT, DELETE, HEAD, OPTIONS and etc)
  -c               Number of connections (default 100)
  -n               Number of total requests
  -t               Number of concurrent goroutines (default 100)
  -r               Number of requests per goroutine
  -d               Duration of time (eg 10s, 10m, 2h45m) (default "0s")
  -p               Print something. 0:Print http response; 1:with extra newline; x.log: log file
  -x               Proxy url, like socks5://127.0.0.1:1080, http://127.0.0.1:1080
  -post            POST data
  -post.file       POST data file path
  -content.type    Content-Type, eg, json, plain, or other full name
  -auth            Authorization header
  -keepalive       HTTP keep-alive (default true)
  -ok              Condition like 'status == 200' for json output
  -png             Upload random png images by file upload
  -png.size        Upload fixed img size (eg. 44kB, 17MB)
  -upload.file     Upload file path
  -upload.filename Upload file name (default "file")
  -read.timeout    Read timeout (in milliseconds) (default 5000)
  -write.timeout   Write timeout (in milliseconds) (default 5000)
  -cpus            Number of used cpu cores. (default for current machine is %d cores)
  -think           Think time, eg. 1s, 100ms, 100-200ms and etc. (unit ns, us/µs, ms, s, m, h)
  -v               Print version
  -weed            weed master URL, like http://127.0.0.1:9333
`

func usageAndExit(msg string) {
	if msg != "" {
		fmt.Fprintf(os.Stderr, msg)
		fmt.Fprintf(os.Stderr, "\n\n")
	}

	flag.Usage()
	os.Exit(1)
}

// Init ...
func (a *App) Init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, usage, runtime.NumCPU())
	}

	flag.IntVar(&a.connections, "c", 100, "")
	flag.IntVar(&a.goroutines, "t", 100, "")
	flag.IntVar(&a.requests, "r", 0, "")
	flag.IntVar(&a.requestsTotal, "n", 0, "")
	flag.StringVar(&a.duration, "d", "0s", "")
	flag.StringVar(&a.urls, "u", "", "")
	flag.BoolVar(&a.keepAlive, "keepalive", true, "")
	flag.StringVar(&a.printResult, "p", "", "")
	flag.StringVar(&a.postDataFilePath, "post.file", "", "")
	flag.StringVar(&a.postData, "post", "", "")
	flag.StringVar(&a.uploadFilePath, "upload.file", "", "")
	flag.StringVar(&a.uploadFileName, "upload.filename", "file", "")
	flag.BoolVar(&a.uploadRandImg, "png", false, "")
	flag.StringVar(&a.fixedImgSize, "png.size", "", "")
	flag.StringVar(&a.method, "m", "", "")
	flag.UintVar(&a.writeTimeout, "write.timeout", 5000, "")
	flag.UintVar(&a.readTimeout, "read.timeout", 5000, "")
	flag.StringVar(&a.authHeader, "auth", "", "")
	flag.StringVar(&a.contentType, "content.type", "", "")
	flag.StringVar(&a.proxy, "x", "", "")
	flag.StringVar(&a.think, "think", "", "")
	weedMasterURL := flag.String("weed", "", "")
	cond := flag.String("ok", "", "")
	version := flag.Bool("v", false, "")
	cpus := flag.Int("cpus", runtime.GOMAXPROCS(-1), "")

	flag.Parse()

	if flag.NArg() > 0 {
		usageAndExit("")
	}

	runtime.GOMAXPROCS(*cpus)

	a.parseCond(*cond)

	if *version {
		fmt.Println("v1.0.2 at 2020-09-07 09:46:32")
		os.Exit(0)
	}

	rand.Seed(time.Now().UnixNano())

	if err := a.parseThinkTime(); err != nil {
		panic(err)
	}

	a.exitChan = make(chan bool)
	a.setupWeed(*weedMasterURL)
	a.setupResponsePrinter()
}

func (a *App) parseCond(cond string) {
	s := strings.TrimSpace(cond)
	if s == "" {
		return
	}

	expr, err := govaluate.NewEvaluableExpression(s)
	if err != nil {
		fmt.Printf("Bad expression: %s Error: %v", s, err)
		os.Exit(1)
	}

	a.cond = expr
}

func (a *App) thinking() {
	if a.thinkMax == 0 {
		return
	}

	var thinkTime time.Duration
	if a.thinkMax == a.thinkMin {
		thinkTime = a.thinkMin
	} else {
		thinkTime = time.Duration(rand.Int31n(int32(a.thinkMax-a.thinkMin))) + a.thinkMin
	}

	a.responsePrinter("think " + thinkTime.String() + "...")

	time.Sleep(thinkTime)
}

func (a *App) parseThinkTime() (err error) {
	if a.think == "" {
		return nil
	}

	rangePos := strings.Index(a.think, "-")
	if rangePos < 0 {
		if a.thinkMin, err = time.ParseDuration(a.think); err != nil {
			return err
		}

		a.thinkMax = a.thinkMin
		return
	}

	min := a.think[0:rangePos]
	max := a.think[rangePos+1:]
	if a.thinkMax, err = time.ParseDuration(max); err != nil {
		return err
	}

	if min == "" {
		a.thinkMin = 0
		return
	}

	if regexp.MustCompile(`^\d+$`).MatchString(min) {
		min += findUnit(max)
	}

	if a.thinkMin, err = time.ParseDuration(min); err != nil {
		return err
	}

	if a.thinkMin > a.thinkMax {
		return errors.Errorf("min think time should be less than max")
	}

	return nil
}

func findUnit(s string) string {
	pos := strings.LastIndexFunc(s, func(r rune) bool {
		return r >= '0' && r <= '9'
	})

	if pos < 0 {
		return s
	}

	return s[pos+1:]
}

func main() {
	var app App

	app.Init()

	if app.responsePrinterFile != nil {
		defer func() {
			app.responsePrinterFile.Close()
			fmt.Println(app.printResult, " generated!")
		}()
	}

	startTime := time.Now()

	HandleInterrupt(func() { app.exitRequested = true }, false)
	HandleMaxProcs()

	c := app.NewConfiguration()
	fmt.Printf("Dispatching %d goroutines at %s\n", app.goroutines,
		startTime.Format("2006-01-02 15:04:05.000"))

	resultChan := make(chan requestResult)
	totalReqsChan := make(chan int)
	statComplete := make(chan bool)

	var rr requestResult
	go stating(resultChan, &rr, totalReqsChan, statComplete)

	requestsChan := make(chan int, app.goroutines)

	for i := 0; i < app.goroutines; i++ {
		reqs := c.requests
		if i == 0 {
			reqs = c.firstRequests
		}

		go app.client(requestsChan, resultChan, c, reqs)
	}

	fmt.Println("Waiting for results...")
	totalRequests := app.waitResults(requestsChan, totalReqsChan, statComplete)
	close(app.exitChan)
	app.printResults(startTime, totalRequests, rr)
}

func (a *App) printResults(startTime time.Time, totalRequests int, rr requestResult) {
	endTime := time.Now()
	elapsed := endTime.Sub(startTime)
	elapsedSeconds := elapsed.Seconds()

	fmt.Println()

	w := new(tabwriter.Writer)
	w.Init(os.Stdout, 0, 8, 0, '\t', 0)

	fmt.Fprintf(w, "Total Requests:\t%d hits\n", totalRequests)
	fmt.Fprintf(w, "Successful requests:\t%d hits\n", rr.success)
	fmt.Fprintf(w, "Network failed:\t%d hits\n", rr.networkFailed)
	if a.cond == nil {
		fmt.Fprintf(w, "Bad requests(!2xx):\t%d hits\n", rr.badFailed)
	} else {
		fmt.Fprintf(w, "Bad requests(!2xx/%s):\t%d hits\n", a.cond, rr.badFailed)
	}
	fmt.Fprintf(w, "Successful requests rate:\t%d hits/sec\n", uint64(float64(rr.success)/elapsedSeconds))
	fmt.Fprintf(w, "Read throughput:\t%s/sec\n",
		humanize.IBytes(uint64(float64(a.readThroughput)/elapsedSeconds)))
	fmt.Fprintf(w, "Write throughput:\t%s/sec\n",
		humanize.IBytes(uint64(float64(a.writeThroughput)/elapsedSeconds)))
	fmt.Fprintf(w, "Test time:\t%s(%s-%s)\n", elapsed.Round(time.Millisecond).String(),
		startTime.Format("2006-01-02 15:04:05.000"), endTime.Format("15:04:05.000"))
	w.Flush()
}

func (a *App) waitResults(requestsChan chan int, totalReqsChan chan int, statComplete chan bool) int {
	totalRequests := 0
	for i := 0; i < a.goroutines; i++ {
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

// NewConfiguration create Conf.
func (a *App) NewConfiguration() (c *Conf) {
	a.connectionChan = make(chan bool, a.connections)
	for i := 0; i < a.connections; i++ {
		a.connectionChan <- true
	}

	c = &Conf{
		urls:       make([]string, 0),
		postData:   nil,
		keepAlive:  a.keepAlive,
		requests:   a.requests,
		authHeader: a.authHeader,
	}

	if a.method != "" {
		c.method = strings.ToUpper(a.method)
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

	return c
}

func (a *App) processUrls(c *Conf) {
	if a.urls == "" && a.weedVolumeAssignedUrl == nil {
		usageAndExit("")
	}

	if strings.Index(a.urls, "@") == 0 {
		var err error

		urlsFilePath := a.urls[1:]
		c.urls, err = FileToLines(urlsFilePath)
		if err != nil {
			log.Fatalf("Error in ioutil.ReadFile for file: %s Error: %v", urlsFilePath, err)
		}
	} else {
		parts := strings.Split(a.urls, ",")
		addr := ""
		for i := 0; i < len(parts); i++ {
			if i == 0 {
				addr = parts[0]
				continue
			}

			if strings.HasPrefix(parts[i], "http:") || strings.HasPrefix(parts[i], "https:") {
				if addr != "" {
					c.urls = append(c.urls, addr)
				}
				addr = addr[:0]
			} else {
				addr += "," + parts[i]
			}
		}

		if addr != "" {
			c.urls = append(c.urls, addr)
		}
	}
}

func (a *App) dealUploadFilePath(c *Conf) {
	if a.uploadFilePath == "" {
		return
	}

	fs, err := os.Stat(a.uploadFilePath)
	if err != nil && os.IsNotExist(err) {
		log.Fatalf("%s dos not exist", a.uploadFilePath)
	}
	if err != nil {
		log.Fatalf("stat file %s error  %v", a.uploadFilePath, err)
	}

	// 单个上传文件
	a.tryMethod(c, http.MethodPost)

	c.postFileChannel = make(chan string, 1)

	if !fs.IsDir() {
		c.postFileChannel <- a.uploadFilePath
		return
	}

	go func() {
		errStopped := fmt.Errorf("program stopped")
		defer func() {
			close(c.postFileChannel)
		}()

		for {
			select {
			case <-a.exitChan:
				return
			default:
			}

			err := godirwalk.Walk(a.uploadFilePath, &godirwalk.Options{
				Callback: func(osPathname string, de *godirwalk.Dirent) error {
					if v, e := de.IsDirOrSymlinkToDir(); v || e != nil {
						return e
					}

					if strings.HasPrefix(de.Name(), ".") {
						return nil
					}

					select {
					case <-a.exitChan:
						return errStopped
					default:
					}

					c.postFileChannel <- osPathname
					return nil
				},
				Unsorted: true,
			})

			if err != nil {
				if err != errStopped {
					log.Printf("Error in walk dir: %s Error: %v", a.uploadFilePath, err)
				}
			}

		}
	}()
}

func (a *App) dealPostDataFilePath(c *Conf) {
	if a.postDataFilePath == "" && a.postData == "" {
		return
	}

	var err error

	a.tryMethod(c, http.MethodPost)

	if a.postData != "" {
		c.postData = []byte(a.postData)
	} else if a.postDataFilePath != "" {
		c.postData, err = ioutil.ReadFile(a.postDataFilePath)
	}

	if err != nil {
		log.Fatalf("Error in ioutil.ReadFile for file path: %s Error: %v", a.postDataFilePath, err)
	}

	if a.contentType == "" && len(c.postData) > 0 {
		firstByte := c.postData[0]
		if firstByte == '{' || firstByte == '[' {
			c.contentType = "application/json; charset=utf-8"
		}
	}
}

func (a *App) tryMethod(c *Conf, method string) {
	if c.method == "" {
		c.method = method
	}
}

func (a *App) period(c *Conf) {
	period, err := time.ParseDuration(a.duration)
	if err != nil {
		log.Fatal(err)
	}

	c.firstRequests = a.requests

	if a.requestsTotal > 0 {
		if a.requestsTotal < a.goroutines {
			a.goroutines = a.requestsTotal
		}

		c.requests = a.requestsTotal / a.goroutines
		c.firstRequests = a.requestsTotal - c.requests*(a.goroutines-1)
	}

	if c.requests == 0 && period == 0 {
		period = 10 * time.Second
	}

	if c.requests != 0 {
		period = 0
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

// nolint:gomnd
func (a *App) randomImage(imageSize string) (imageBytes []byte, contentType, imageFile string) {
	var (
		err  error
		size int64
	)

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

func (a *App) client(requestsChan chan int, resultChan chan requestResult, conf *Conf, req int) {
	urlIndex := -1
	if len(conf.urls) > 0 {
		urlIndex = rand.Intn(len(conf.urls))
	}

	i := 0
	for ; !a.exitRequested && (req == 0 || i < req); i++ {
		if i > 0 {
			a.thinking()
		}

		addr := ""
		if urlIndex >= 0 {
			addr = conf.urls[urlIndex]
		}
		a.doRequest(resultChan, conf, addr)

		if urlIndex >= 0 {
			if urlIndex++; urlIndex >= len(conf.urls) {
				urlIndex = 0
			}
		}
	}

	requestsChan <- i
}

func (a *App) doRequest(resultChan chan requestResult, c *Conf, addr string) {
	postData := c.postData
	contentType := c.contentType
	fileName := ""

	if a.uploadRandImg || a.fixedImgSize != "" {
		a.tryMethod(c, http.MethodPost)

		postData, contentType, fileName = a.randomImage(a.fixedImgSize)
	}

	a.tryMethod(c, http.MethodGet)

	<-a.connectionChan

	go func() {
		if c.postFileChannel == nil { // 非目录文件上传请求
			a.do(resultChan, c, a.weed(addr), c.method, contentType, fileName, postData)
			return
		}

		for pf := range c.postFileChannel {
			data, ct, err := ReadUploadMultipartFile(a.uploadFileName, pf)
			if err != nil {
				log.Printf("Error in ReadUploadMultipartFile for file path: %s Error: %v", a.uploadFilePath, err)
				continue
			}

			a.do(resultChan, c, a.weed(addr), c.method, ct, pf, data)
		}
	}()
}

type requestResult struct {
	networkFailed int
	success       int
	badFailed     int
}

func (a *App) do(result chan requestResult, cnf *Conf, addr, method, contentType, fileName string, postData []byte) {
	var (
		err  error
		resp *fasthttp.Response
	)

	statusCode := 0

	if strings.HasPrefix(addr, "err:") {
		err = errors.New(addr)
	} else {
		req := fasthttp.AcquireRequest()

		defer func() {
			fasthttp.ReleaseRequest(req)
			a.connectionChan <- true
		}()

		req.SetRequestURI(addr)
		req.Header.SetMethod(method)
		SetHeaderIfNotEmpty(req, "Connection", IfElse(cnf.keepAlive, "keep-alive", "close"))
		SetHeaderIfNotEmpty(req, "Authorization", cnf.authHeader)
		SetHeaderIfNotEmpty(req, "Content-Type", contentType)
		req.SetBody(postData)

		resp = fasthttp.AcquireResponse()
		defer fasthttp.ReleaseResponse(resp)

		err = cnf.myClient.Do(req, resp)
		statusCode = resp.StatusCode()
	}
	var (
		rr         requestResult
		resultDesc string
	)

	switch {
	case err != nil:
		rr.networkFailed = 1
		resultDesc = "[x] " + err.Error()
	case statusCode < http.StatusBadRequest:
		if a.isOK(resp) {
			rr.success = 1
			resultDesc = "[√]"
		} else {
			rr.badFailed = 1
			resultDesc = "[x]"
		}

	default:
		rr.badFailed = 1
		resultDesc = "[x]"
	}

	a.printResponse(addr, fileName, resultDesc, statusCode, resp)

	result <- rr
}

func (a *App) isOK(resp *fasthttp.Response) bool {
	if a.cond == nil {
		return true
	}

	body := resp.Body()
	if !gjson.ValidBytes(body) {
		return false
	}

	vars := a.cond.Vars()
	parameters := make(map[string]interface{}, len(vars))
	for _, v := range vars {
		jsonValue := gjson.GetBytes(body, v)
		parameters[v] = jsonValue.Value()
	}

	result, err := a.cond.Evaluate(parameters)
	if err != nil {
		return false
	}

	yes, ok := result.(bool)
	return yes && ok
}

func (a *App) printResponse(addr, fileName string, resultDesc string, statusCode int, resp *fasthttp.Response) {
	if a.responsePrinter == nil {
		return
	}

	r := ""

	if a.weedVolumeAssignedUrl != nil {
		r += "url:" + addr + " "
	}

	if fileName != "" {
		r += "file:" + fileName + " "
	}

	r += resultDesc + " [" + strconv.Itoa(statusCode) + "] "

	body := string(resp.Body())
	if body != "" {
		r += body
	}

	a.responsePrinter(r)
}

// SetHeaderIfNotEmpty set request header if value is not empty.
func SetHeaderIfNotEmpty(request *fasthttp.Request, header, value string) {
	if value != "" {
		request.Header.Set(header, value)
	}
}

// MyConn for net connection.
type MyConn struct {
	net.Conn
	app *App
}

// Read bytes from net connection.
func (myConn *MyConn) Read(b []byte) (n int, err error) {
	if n, err = myConn.Conn.Read(b); err == nil {
		atomic.AddUint64(&myConn.app.readThroughput, uint64(n))
	}

	return
}

// Write bytes to net.
func (myConn *MyConn) Write(b []byte) (n int, err error) {
	if n, err = myConn.Conn.Write(b); err == nil {
		atomic.AddUint64(&myConn.app.writeThroughput, uint64(n))
	}

	return
}

// MyDialer create Dial function.
func (a *App) MyDialer() func(address string) (conn net.Conn, err error) {
	return func(address string) (net.Conn, error) {
		dialer, err := NewProxyConn(a.proxy)
		if err != nil {
			return nil, err
		}

		conn, err := dialer.Dial("tcp", address)
		if err != nil {
			return nil, err
		}

		return &MyConn{Conn: conn, app: a}, nil
	}
}

var (
	re1 = regexp.MustCompile(`\r?\n`)
	re2 = regexp.MustCompile(`\s{2,}`)
)

func line(s string) string {
	s = re1.ReplaceAllString(s, " ")
	s = strings.TrimSpace(re2.ReplaceAllString(s, " "))

	return s
}

// nolint:gochecknoglobals
var (
	lastResponseCh chan string
)

func (a *App) setupResponsePrinter() {
	var f func(a string, direct bool)

	switch r := a.printResult; r {
	case "":
		return
	case "0":
		f = func(a string, direct bool) {
			if direct {
				_, _ = fmt.Fprint(os.Stdout, a)
			} else {
				_, _ = fmt.Fprint(os.Stdout, line(a))
			}
		}
	case "1":
		f = func(a string, direct bool) {
			if direct {
				_, _ = fmt.Fprint(os.Stdout, a)
			} else {
				_, _ = fmt.Fprintln(os.Stdout, line(a))
			}
		}
	default:
		lf, err := os.OpenFile(r, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			panic(err)
		}

		a.responsePrinterFile = lf
		f = func(s string, direct bool) {
			if direct {
				_, _ = a.responsePrinterFile.WriteString(s)
			} else {
				_, _ = a.responsePrinterFile.WriteString(s + "\n")
			}
		}
	}

	lastResponseCh = make(chan string, 10000)
	go func() {
		lastResponse := ""
		lastResponseThrottle := MakeThrottle(1 * time.Second)
		for a := range lastResponseCh {
			if lastResponse == "" || lastResponse != a {
				f(a, false)
			} else if lastResponseThrottle.Allow() {
				f(".", true)
			}

			lastResponse = a
		}
	}()

	a.responsePrinter = func(a string) {
		lastResponseCh <- a
	}
}

func (a *App) weed(addr string) string {
	if a.weedVolumeAssignedUrl == nil {
		return addr
	}

	return <-a.weedVolumeAssignedUrl
}

func (a *App) setupWeed(weedMasterURL string) {
	if weedMasterURL == "" {
		return
	}

	timeout := time.Duration(a.readTimeout) * time.Millisecond
	weedClient, err := cweed.New(weedMasterURL, nil, 8096, &http.Client{Timeout: timeout})
	if err != nil {
		log.Fatalf("create weedClient for %s error: %v", weedMasterURL, err)
	}

	a.weedVolumeAssignedUrl = make(chan string, 100)
	go func() {
		for {
			result, err := weedClient.Assign(nil)
			if err != nil {
				log.Printf("assign url error: %s", err)
				a.weedVolumeAssignedUrl <- "err:" + err.Error()
			} else {
				fileUrl := "http://" + result.PublicURL + "/" + result.FileID
				a.weedVolumeAssignedUrl <- fileUrl
			}
		}
	}()
}

// HandleInterrupt handles Ctrl+C to exit after calling f.
// nolint:gomnd
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

// IfElse return then if condition is true,  else els if false.
func IfElse(condition bool, then, els string) string {
	if condition {
		return then
	}

	return els
}

// FileToLines read file into lines.
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

// MustOpen open file successfully or panic.
func MustOpen(f string) *os.File {
	r, err := os.Open(f)
	if err != nil {
		panic(err)
	}

	return r
}

// ReadUploadMultipartFile read file filePath for upload in multipart,
// return multipart content, form data content type and error.
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

// Throttle ...
type Throttle struct {
	tokenC chan bool
	stopC  chan bool
}

// MakeThrottle ...
func MakeThrottle(duration time.Duration) *Throttle {
	t := &Throttle{
		tokenC: make(chan bool, 1),
		stopC:  make(chan bool, 1),
	}

	go func() {
		ticker := time.NewTicker(duration)
		defer ticker.Stop()

		for {
			select {
			case <-t.stopC:
				return
			case <-ticker.C:
				select {
				case t.tokenC <- true:
				default:
				}
			}
		}
	}()

	return t
}

// Stop ...
func (t *Throttle) Stop() {
	t.stopC <- true
}

// Allow ...
func (t *Throttle) Allow() bool {
	select {
	case <-t.tokenC:
		return true
	default:
		return false
	}
}

// Return based on proxy url
func NewProxyConn(proxyUrl string) (ProxyConn, error) {
	u, err := url.Parse(proxyUrl)
	if err != nil {
		return nil, err
	}

	switch u.Scheme {
	case "socks5":
		return &Socks5Client{proxyUrl: u}, nil
	case "http":
		return &HttpClient{proxyUrl: u}, nil
	default:
		return &DefaultClient{}, nil
	}
}

// ProxyConn is used to define the proxy
type ProxyConn interface {
	Dial(network string, address string) (net.Conn, error)
}

// DefaultClient is used to implement a proxy in default
type DefaultClient struct {
	rAddr *net.TCPAddr
}

// Socks5 implementation of ProxyConn
// Set KeepAlive=-1 to reduce the call of syscall
func (dc *DefaultClient) Dial(network string, address string) (conn net.Conn, err error) {
	if dc.rAddr == nil {
		dc.rAddr, err = net.ResolveTCPAddr("tcp", address)
		if err != nil {
			return nil, err
		}
	}
	return net.DialTCP(network, nil, dc.rAddr)
}

// Socks5Client is used to implement a proxy in socks5
type Socks5Client struct {
	proxyUrl *url.URL
}

// Socks5 implementation of ProxyConn
func (s5 *Socks5Client) Dial(network string, address string) (net.Conn, error) {
	d, err := proxy.FromURL(s5.proxyUrl, nil)
	if err != nil {
		return nil, err
	}

	return d.Dial(network, address)
}

// Socks5Client is used to implement a proxy in http
type HttpClient struct {
	proxyUrl *url.URL
}

// Http implementation of ProxyConn
func (hc *HttpClient) Dial(network string, address string) (net.Conn, error) {
	req, err := http.NewRequest("CONNECT", "http://"+address, nil)
	if err != nil {
		return nil, err
	}
	password, _ := hc.proxyUrl.User.Password()
	req.SetBasicAuth(hc.proxyUrl.User.Username(), password)
	proxyConn, err := net.Dial("tcp", hc.proxyUrl.Host)
	if err != nil {
		return nil, err
	}
	if err := req.Write(proxyConn); err != nil {
		return nil, err
	}
	res, err := http.ReadResponse(bufio.NewReader(proxyConn), req)
	if err != nil {
		return nil, err
	}
	_ = res.Body.Close()
	if res.StatusCode != 200 {
		return nil, errors.New("Proxy error " + res.Status)
	}
	return proxyConn, nil
}
