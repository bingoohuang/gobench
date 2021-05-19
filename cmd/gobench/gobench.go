package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	flag "github.com/bingoohuang/gg/pkg/fla9"
	"github.com/bingoohuang/jj"
	"github.com/cheggaaa/pb/v3"
	"io"
	"io/ioutil"
	"log"
	"math/big"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/karrick/godirwalk"

	"github.com/Knetic/govaluate"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"golang.org/x/net/proxy"

	"github.com/bingoohuang/golang-trial/randimg"
	"github.com/dustin/go-humanize"

	"github.com/valyala/fasthttp"

	"github.com/docker/go-units"
	"github.com/satori/go.uuid"
	_ "net/http/pprof"
)

// App ...
type App struct {
	requests, requestsTotal, goroutines, connections                int
	method, duration, urls, postData, uFilePath                     string
	keepAlive                                                       bool
	uploadRandImg, printResult                                      string
	timeout, thinkMin, thinkMax                                     time.Duration
	rThroughput, wThroughput                                        uint64
	authHeader, uFieldName, fixedImgSize, contentType, think, proxy string
	weedMasterURL, pprof, profile, profileBaseURL, eval             string

	// 返回JSON时的判断是否调用成功的表达式
	cond *govaluate.EvaluableExpression

	connectionChan      chan struct{}
	responsePrinter     func(s string)
	responsePrinterFile *os.File

	weedVolumeAssignedUrl chan string // 海草文件上传路径地址
	exitChan              chan bool

	body        string
	bodyPrintCh chan string
	waitGroup   *sync.WaitGroup
	seqFn       func(s ...string) []string
	ctx         context.Context
	ctxCancel   context.CancelFunc
}

// Conf for gobench.
type Conf struct {
	urls                            []string
	postData                        []byte
	requests, firstRequests         int
	duration                        time.Duration
	keepAlive                       bool
	method, authHeader, contentType string

	myClient        fasthttp.Client
	postFileChannel chan string
	profiles        []*Profile
}

const usage = `Usage: gobench [options...] url1[,url2...]
Options:
  -l               URL list (# separated), or @URL's file path (line separated)
  -X               HTTP method(GET, POST, PUT, DELETE, HEAD, OPTIONS and etc)
  -c               Number of connections (default 100)
  -n               Number of total requests
  -t               Number of concurrent goroutines (default 100)
  -r               Number of requests per goroutine
  -d               Duration of time (eg 10s, 10m, 2h45m) (10s if no total requests or per-goroutine-requests set)
  -p               Print something. 0: Print http response; 1: with extra newline; x.log: log file
  -profile         Profile file name, pass an non-existing profile to generate a sample one
  -profile.baseurl Base URL for requests uri in Profile, like http://127.0.0.1:1080
  -proxy           Proxy url, like socks5://127.0.0.1:1080, http://127.0.0.1:1080
  -P               POST data, use @a.json for a file
  -c.type          Content-Type, eg, json, plain, or other full name
  -auth            Authorization header
  -k               HTTP keep-alive (default true)
  -ok              Condition like 'status == 200' for json output
  -image           Upload random images, png/jpg
  -i.size          Upload fixed img size (eg. 44kB, 17MB)
  -u.file          Upload file path
  -u.field         Upload field name (default "file")
  -timeout         Read/Write timeout (like 5ms,10ms,10s) (default 5s)
  -cpus            Number of used cpu cores. (default for current machine is %d cores)
  -think           Think time, eg. 1s, 100ms, 100-200ms and etc. (unit ns, us/µs, ms, s, m, h)
  -v               Print version
  -weed            Weed master URL, like http://127.0.0.1:9333
  -pprof           Profile pprof address, like :6060
  -body[:cond]     A filename to save response body, with an optional jj expression to filter body when saving, like person.name, see github.com/bingoohuang/jj
  -eval dd:seq     Eval dd in url/body(eg. dd:seq will evaluated to dd0-n, d00:seq will evaluated to d00-d99) 
  -eval dd:seq0    Eval dd in url/body(eg. dd:seq0 will evaluated to 0-n, d00:seq will evaluated to 00-99) 
  -seq             Start sequence number for request header X-Gobench-Seq 
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
	flag.StringVar(&a.urls, "l", "", "")
	flag.BoolVar(&a.keepAlive, "k", true, "")
	flag.StringVar(&a.printResult, "p", "", "")
	flag.StringVar(&a.profile, "profile", "", "")
	flag.StringVar(&a.profileBaseURL, "profile.baseurl", "", "")
	flag.StringVar(&a.postData, "P", "", "")
	flag.StringVar(&a.uFilePath, "u.file", "", "")
	flag.StringVar(&a.uFieldName, "u.field", "file", "")
	flag.StringVar(&a.uploadRandImg, "image", "", "")
	flag.StringVar(&a.fixedImgSize, "i.size", "", "")
	flag.StringVar(&a.method, "X", "", "")
	flag.DurationVar(&a.timeout, "timeout", 5*time.Second, "")
	flag.StringVar(&a.authHeader, "auth", "", "")
	flag.StringVar(&a.contentType, "c.type", "", "")
	flag.StringVar(&a.proxy, "proxy", "", "")
	flag.StringVar(&a.think, "think", "", "")
	flag.StringVar(&a.weedMasterURL, "weed", "", "")
	flag.StringVar(&a.pprof, "pprof", "", "")
	flag.StringVar(&a.body, "body", "", "")
	flag.StringVar(&a.eval, "eval", "", "")
	cond := flag.String("ok", "", "")
	version := flag.Bool("v", false, "")
	cpus := flag.Int("cpus", runtime.GOMAXPROCS(-1), "")
	argSeq := flag.Int("seq", 0, "")

	flag.Parse()
	if *version {
		fmt.Println("v1.0.3 at 2021-03-24 23:04:19")
		os.Exit(0)
	}

	seq = int32(*argSeq)

	if a.weedMasterURL != "" {
		var err error
		a.weedMasterURL, err = CreateUri(a.weedMasterURL, "/dir/assign", map[string]string{"count": "1"})
		if err != nil {
			usageAndExit(err.Error())
		}
	}

	if flag.NArg() > 0 {
		if a.urls != "" {
			usageAndExit("bad args for " + strings.Join(flag.Args(), ", "))
		}
		a.urls = strings.Join(flag.Args(), ",")
	}

	runtime.GOMAXPROCS(*cpus)
	a.parseCond(*cond)

	if err := a.parseThinkTime(); err != nil {
		panic(err)
	}

	a.exitChan = make(chan bool)
	a.waitGroup = &sync.WaitGroup{}
	a.setupResponsePrinter()
	a.setupBodyPrint()
	a.parseEval(a.eval)

	signalCh := make(chan os.Signal)
	signal.Notify(signalCh, os.Interrupt, os.Kill, syscall.SIGTERM)
	go func() {
		<-signalCh
		close(a.exitChan)
		a.ctxCancel()
	}()
}

func (a *App) parseEval(eval string) {
	if eval == "" {
		return
	}

	pos := strings.LastIndex(eval, ":")
	if pos <= 0 || pos == len(eval)-1 {
		fmt.Fprintf(os.Stderr, "bad format for eval %s", eval)
		os.Exit(1)
	}

	pattern := eval[:pos]
	evaluator := eval[pos+1:]
	switch evaluator {
	case "seq0", "seq":
		prepend := pattern
		if evaluator == "seq0" {
			prepend = ""
		}
		loc := regexp.MustCompile(`\d+`).FindStringIndex(pattern)
		f := func(ss []string, i int64) []string {
			seqStr := fmt.Sprintf("%d", i)
			ret := make([]string, len(ss))
			un := uuid.NewV4().String()

			for i, s := range ss {
				s = strings.ReplaceAll(s, pattern, prepend+seqStr)
				ret[i] = strings.ReplaceAll(s, "{uuid}", un)
			}
			return ret
		}
		start, max := int64(0), int64(0)
		if len(loc) > 0 {
			start, _ = strconv.ParseInt(pattern[loc[0]:loc[1]], 10, 64)
			width := loc[1] - loc[0]
			max = 1
			for i := 0; i < width; i++ {
				max *= 10
			}
			left, right := pattern[:loc[0]], pattern[loc[1]:]
			if evaluator == "seq0" {
				left = ""
			}
			f = func(ss []string, i int64) []string {
				seqStr := fmt.Sprintf("%0*d", width, i)
				ret := make([]string, len(ss))
				un := uuid.NewV4().String()
				for i, s := range ss {
					s = strings.ReplaceAll(s, pattern, left+seqStr+right)
					ret[i] = strings.ReplaceAll(s, "{uuid}", un)
				}
				return ret
			}
		}
		seqCh := make(chan int64, 100)
		go func() {
			for i := start; ; i++ {
				if i == max {
					i = start
				}
				seqCh <- i
			}
		}()
		a.seqFn = func(s ...string) []string { return f(s, <-seqCh) }
	default:
		fmt.Fprintf(os.Stderr, "unknown evaluator for eval %s", eval)
		os.Exit(1)
	}
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

func randInt(n int64) int64 {
	result, _ := rand.Int(rand.Reader, big.NewInt(n))
	return result.Int64()
}

func (a *App) thinking() {
	if a.thinkMax == 0 {
		return
	}

	var thinkTime time.Duration
	if a.thinkMax == a.thinkMin {
		thinkTime = a.thinkMin
	} else {
		thinkTime = time.Duration(randInt(int64(a.thinkMax-a.thinkMin))) + a.thinkMin
	}

	a.responsePrinter("think " + thinkTime.String() + "...")
	ticker := time.NewTicker(thinkTime)
	defer ticker.Stop()

	select {
	case <-a.exitChan:
	case <-ticker.C:
	}
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
	ctx, ctxCancel := context.WithCancel(context.Background())
	app := &App{
		ctx:       ctx,
		ctxCancel: ctxCancel,
	}

	app.Init()

	if app.responsePrinterFile != nil {
		defer func() {
			app.responsePrinterFile.Close()
			fmt.Println(app.printResult, " generated!")
		}()
	}

	if app.pprof != "" {
		go func() {
			log.Printf("Starting pprof at %s", app.pprof)
			log.Println(http.ListenAndServe(app.pprof, nil))
		}()
	}

	startTime := time.Now()
	c := app.NewConfiguration()
	fmt.Printf("Dispatching %d goroutines at %s\n", app.goroutines, startTime.Format("2006-01-02 15:04:05.000"))

	app.setupWeed(&c.myClient)

	resultChan := make(chan requestResult)
	totalReqsChan := make(chan int)
	statComplete := make(chan bool)

	var rr requestResult
	go stating(resultChan, &rr, totalReqsChan, statComplete)

	requestsChan := make(chan int, app.goroutines)
	barIncr, barFinish := createBars(app, c)

	for i := 0; i < app.goroutines; i++ {
		reqs := c.requests
		if i == 0 {
			reqs = c.firstRequests
		}

		go app.client(barIncr, requestsChan, resultChan, c, reqs)
	}

	totalRequests := app.waitResults(requestsChan, totalReqsChan, statComplete)

	select {
	case _, opened := <-app.exitChan:
		if opened {
			close(app.exitChan)
		}
	default:
		close(app.exitChan)
	}

	if app.bodyPrintCh != nil {
		close(app.bodyPrintCh)
	}
	if app.responsePrinter != nil {
		app.responsePrinter(CloseString)
	}

	barFinish()

	app.waitGroup.Wait()
	app.printResults(startTime, totalRequests, rr)
}

func createBars(app *App, c *Conf) (barIncr, barFinish func()) {
	if app.printResult != "" {
		return func() {}, func() {}
	}

	if app.requestsTotal > 0 {
		bar := pb.StartNew(app.requestsTotal)
		return func() { bar.Increment() }, func() { bar.Finish() }
	}

	bar := pb.StartNew(int(c.duration.Seconds()))
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				bar.Increment()
			case <-app.ctx.Done():
				return
			}
		}
	}()

	return func() {}, func() { bar.Finish() }
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
	fmt.Fprintf(w, "Successful requests rate:\t%0.3f hits/sec\n", float64(rr.success)/elapsedSeconds)
	fmt.Fprintf(w, "Read throughput:\t%s/sec\n",
		humanize.IBytes(uint64(float64(a.rThroughput)/elapsedSeconds)))
	fmt.Fprintf(w, "Write throughput:\t%s/sec\n",
		humanize.IBytes(uint64(float64(a.wThroughput)/elapsedSeconds)))
	fmt.Fprintf(w, "Test time:\t%s(%s-%s)\n", elapsed.Round(time.Millisecond).String(),
		startTime.Format("2006-01-02 15:04:05.000"), endTime.Format("15:04:05.000"))
	fmt.Fprintf(w, "X-Gobench-Seq:\t%d\n", SecCur())
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
	a.connectionChan = make(chan struct{}, a.connections)
	for i := 0; i < a.connections; i++ {
		a.connectionChan <- struct{}{}
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

	a.processProfile(c)
	a.period(c)
	a.processUrls(c)
	a.dealPostDataFilePath(c)
	a.dealUploadFilePath(c)

	c.myClient.ReadTimeout = a.timeout
	c.myClient.WriteTimeout = a.timeout
	c.myClient.MaxConnsPerHost = a.connections
	c.myClient.Dial = a.MyDialer()

	if a.contentType != "" {
		c.contentType = a.contentType
	}

	return c
}

func (a *App) processUrls(c *Conf) {
	if a.urls == "" && a.weedMasterURL == "" && len(c.profiles) == 0 {
		usageAndExit("url/weed/profile required!")
	}

	if strings.Index(a.urls, "@") == 0 {
		var err error

		urlsFilePath := a.urls[1:]
		c.urls, err = FileToLines(urlsFilePath)
		if err != nil {
			log.Fatalf("Error in ioutil.ReadFile for file: %s Error: %v", urlsFilePath, err)
		}
	} else {
		c.urls = strings.Split(a.urls, "#")
	}

	for i, u := range c.urls {
		c.urls[i] = fixUrl("", u)
	}
}

func expand(path string) string {
	if len(path) == 0 || path[0] != '~' {
		return path
	}

	usr, err := user.Current()
	if err != nil {
		return path
	}
	return filepath.Join(usr.HomeDir, path[1:])
}

func IfExit(ch chan bool) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func (a *App) dealUploadFilePath(c *Conf) {
	if a.uFilePath == "" {
		return
	}
	a.uFilePath = expand(a.uFilePath)
	fs, err := os.Stat(a.uFilePath)
	if err != nil && os.IsNotExist(err) {
		log.Fatalf("%s dos not exist", a.uFilePath)
	}
	if err != nil {
		log.Fatalf("stat file %s error  %v", a.uFilePath, err)
	}

	// 单个上传文件
	a.tryMethod(c, http.MethodPost)

	c.postFileChannel = make(chan string, 1)
	isSingleFile := !fs.IsDir()

	go func() {
		errStopped := fmt.Errorf("program stopped")
		defer func() { close(c.postFileChannel) }()

		for i := 0; i < a.requestsTotal; {
			if IfExit(a.exitChan) {
				return
			}

			if isSingleFile {
				c.postFileChannel <- a.uFilePath
				i++
				continue
			}

			err := godirwalk.Walk(a.uFilePath, &godirwalk.Options{
				Unsorted: true,
				Callback: func(osPathname string, de *godirwalk.Dirent) error {
					if v, e := de.IsDirOrSymlinkToDir(); v || e != nil {
						return e
					}

					if strings.HasPrefix(de.Name(), ".") {
						return nil
					}

					if IfExit(a.exitChan) {
						return errStopped
					}

					if randInt(10) != 0 {
						return nil
					}

					c.postFileChannel <- osPathname
					if i++; i >= a.requestsTotal {
						return errStopped
					}

					return nil
				},
			})

			if err != nil && err != errStopped {
				log.Printf("Error in walk dir: %s Error: %v", a.uFilePath, err)
			}
		}
	}()
}

func (a *App) dealPostDataFilePath(c *Conf) {
	if a.postData == "" {
		return
	}

	var err error

	a.tryMethod(c, http.MethodPost)

	if a.postData != "" {
		if strings.HasPrefix(a.postData, "@") {
			postDataFile := a.postData[1:]
			c.postData, err = ioutil.ReadFile(postDataFile)
			if err != nil {
				log.Fatalf("Error in ioutil.ReadFile for file path: %s Error: %v", postDataFile, err)
			}
		} else {
			c.postData = []byte(a.postData)
		}
	}

	if a.contentType == "" && len(c.postData) > 0 {
		firstByte := c.postData[0]
		if firstByte == '{' || firstByte == '[' {
			c.contentType = "application/json; charset=utf-8"
		} else if bytes.HasSuffix(bytes.TrimSpace(c.postData), []byte(">")) {
			c.contentType = "application/xml; charset=utf-8"
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
	} else {
		a.requestsTotal = c.requests * a.goroutines
	}

	if c.requests == 0 && period == 0 {
		period = 10 * time.Second
	}

	if c.requests != 0 {
		period = 0
	}

	if len(c.profiles) > 0 {
		a.requestsTotal *= len(c.profiles)
	}

	if period == 0 {
		return
	}

	c.duration = period

	go func() {
		<-time.After(period)
		a.ctxCancel()
	}()
}

// nolint:gomnd
func (a *App) randomImage(imageExt, imageSize string) (imageBytes []byte, contentType, imageFile string) {
	var (
		err  error
		size int64
	)

	if imageSize == "" {
		size = (randInt(4) + 1) << 20 //  << 20 means MiB
	} else {
		if size, err = units.FromHumanSize(a.fixedImgSize); err != nil {
			log.Fatal("error fixedImgSize ", err.Error())
		}
	}

	randText := strconv.FormatUint(randimg.RandUint64(), 10)
	ext := ".png"
	if imageExt != "" {
		ext = "." + imageExt
	}
	imageFile = randText + ext
	rc := randimg.RandImageConfig{
		Width:      650,
		Height:     350,
		RandomText: randText,
		FileName:   imageFile,
		FixedSize:  size,
	}
	rc.GenerateFile()
	defer os.Remove(imageFile)

	imageBytes, contentType, _ = ReadUploadMultipartFile(a.uFieldName, imageFile)
	return
}

func (a *App) client(barIncr func(), requestsChan chan int, resultChan chan requestResult, conf *Conf, req int) {
	urlIndex := -1
	if len(conf.urls) > 0 {
		urlIndex = int(randInt(int64(len(conf.urls))))
	}

	i := 0
	count := 0
	for ; a.ctx.Err() == nil && (req == 0 || i < req); i++ {
		addr := ""
		if urlIndex >= 0 {
			addr = conf.urls[urlIndex]
		}
		count += a.doRequest(barIncr, resultChan, conf, addr)

		if urlIndex >= 0 {
			if urlIndex++; urlIndex >= len(conf.urls) {
				urlIndex = 0
			}
		}
	}

	requestsChan <- count
}

func (a *App) doRequest(barIncr func(), resultChan chan requestResult, c *Conf, addr string) int {
	postData := c.postData
	contentType := c.contentType
	fileName := ""

	if a.uploadRandImg != "" || a.fixedImgSize != "" {
		a.tryMethod(c, http.MethodPost)

		postData, contentType, fileName = a.randomImage(a.uploadRandImg, a.fixedImgSize)
	}

	a.tryMethod(c, http.MethodGet)

	if c.postFileChannel == nil { // 非目录文件上传请求
		return a.do(barIncr, resultChan, c, a.weed(addr), c.method, contentType, fileName, postData)
	}

	count := 0
	for pf := range c.postFileChannel {
		data, ct, err := ReadUploadMultipartFile(a.uFieldName, pf)
		if err != nil {
			log.Printf("Error in ReadUploadMultipartFile for file path: %s Error: %v", a.uFilePath, err)
			continue
		}

		count += a.do(barIncr, resultChan, c, a.weed(addr), c.method, ct, pf, data)
	}

	return count
}

type requestResult struct {
	networkFailed, success, badFailed int
}

func (a *App) do(barIncr func(), rc chan requestResult, cnf *Conf, addr, method, contentType, fileName string, postData []byte) int {
	var err error
	var rsp *fasthttp.Response

	<-a.connectionChan

	defer func() {
		a.connectionChan <- struct{}{}
		a.thinking()
	}()

	statusCode := 0

	if strings.HasPrefix(addr, "err:") {
		err := errors.New("addr-" + addr)
		a.checkResult(rc, err, statusCode, rsp, addr, fileName)
		barIncr()
		return 1
	}

	if len(cnf.profiles) == 0 {
		a.exec(rc, cnf, addr, method, contentType, fileName, postData, rsp, err, statusCode)
		barIncr()
		return 1
	}

	for i, pr := range cnf.profiles {
		if i > 0 {
			a.thinking()
		}
		a.execProfile(rc, cnf, addr, rsp, pr, err, statusCode, fileName)
		barIncr()
		if a.ctx.Err() != nil {
			return i + 1
		}
	}

	return len(cnf.profiles)
}

func (a *App) execProfile(rc chan requestResult, cnf *Conf, addr string, rsp *fasthttp.Response, pr *Profile, err error, statusCode int, fileName string) {
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	rsp = fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(rsp)

	if a.profileBaseURL != "" && !strings.HasPrefix(pr.URL, "http") {
		joined, err := CreateUri(a.profileBaseURL, pr.URL, nil)
		if err != nil {
			log.Fatalf("eror %v", err)
		}
		req.SetRequestURI(fixUrl("", joined))
	} else {
		req.SetRequestURI(fixUrl("", pr.URL))
	}

	req.Header.SetMethod(pr.Method)
	for k, v := range pr.Headers {
		SetHeader(req, k, v)
	}
	SetGobenchHeaders(req)

	req.SetBody([]byte(pr.Body))

	err = cnf.myClient.Do(req, rsp)
	statusCode = rsp.StatusCode()
	a.checkResult(rc, err, statusCode, rsp, addr, fileName)
}

var seq int32

func SecCur() int32 { return atomic.LoadInt32(&seq) }
func SeqInc() int32 { return atomic.AddInt32(&seq, 1) }

func SetGobenchHeaders(req *fasthttp.Request) {
	SetHeader(req, "X-Gobench-Seq", fmt.Sprintf("%d", SeqInc()))
	SetHeader(req, "X-Gobench-Time", time.Now().Format(`2006-01-02 15:04:05.000`))
}

func (a *App) exec(rc chan requestResult, cnf *Conf, addr string, method string, contentType string, fileName string,
	postData []byte, rsp *fasthttp.Response, err error, statusCode int) {
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	rsp = fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(rsp)
	postDataStr := string(postData)
	if a.seqFn != nil {
		ret := a.seqFn(addr, postDataStr)
		addr = ret[0]
		postDataStr = ret[1]
		a.responsePrinter("url: " + addr)
	}
	req.SetRequestURI(fixUrl("", addr))
	req.Header.SetMethod(method)
	SetHeader(req, "Connection", IfElse(cnf.keepAlive, "keep-alive", "close"))
	SetHeader(req, "Authorization", cnf.authHeader)
	SetHeader(req, "Content-Type", contentType)
	SetGobenchHeaders(req)

	req.SetBody([]byte(postDataStr))

	err = cnf.myClient.Do(req, rsp)
	statusCode = rsp.StatusCode()

	a.checkResult(rc, err, statusCode, rsp, addr, fileName)
}

func (a *App) checkResult(rc chan requestResult, err error, statusCode int, rsp *fasthttp.Response, addr string, fileName string) {
	var (
		rr         requestResult
		resultDesc string
	)

	switch {
	case err != nil:
		rr.networkFailed = 1
		resultDesc = "[X] " + err.Error()
	case statusCode >= 200 && statusCode < 300:
		if a.isOK(rsp) {
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

	a.printResponse(addr, fileName, resultDesc, statusCode, rsp)

	rc <- rr
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
	if a.bodyPrintCh == nil && (a.responsePrinter == nil || resp == nil) {
		return
	}

	body := string(resp.Body())
	if a.bodyPrintCh != nil {
		a.bodyPrintCh <- body
	}

	if a.responsePrinter == nil || resp == nil {
		return
	}

	r := time.Now().Format(`2006-01-02 15:04:05.000 `)
	if a.weedVolumeAssignedUrl != nil {
		r += "url:" + addr + " "
	}

	if fileName != "" {
		r += "file:" + fileName + " "
	}

	r += resultDesc + " [" + strconv.Itoa(statusCode) + "] " + resp.String()
	a.responsePrinter(r)
}

// SetHeader set request header if value is not empty.
func SetHeader(r *fasthttp.Request, header, value string) {
	if value != "" {
		r.Header.Set(header, value)
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
		atomic.AddUint64(&myConn.app.rThroughput, uint64(n))
	}

	return
}

// Write bytes to net.
func (myConn *MyConn) Write(b []byte) (n int, err error) {
	if n, err = myConn.Conn.Write(b); err == nil {
		atomic.AddUint64(&myConn.app.wThroughput, uint64(n))
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

var re1 = regexp.MustCompile(`\r?\n`)
var re2 = regexp.MustCompile(`\s{2,}`)

func line(s string) string {
	s = re1.ReplaceAllString(s, " ")
	s = strings.TrimSpace(re2.ReplaceAllString(s, " "))

	return s
}

func (a *App) setupResponsePrinter() {
	var f func(a string, direct bool)

	switch r := a.printResult; r {
	case "":
		a.responsePrinter = func(a string) {}
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
		fmt.Printf("Log file %s created or appended!\n", r)

		a.responsePrinterFile = lf
		f = func(s string, direct bool) {
			if direct {
				_, _ = a.responsePrinterFile.WriteString(s)
			} else {
				_, _ = a.responsePrinterFile.WriteString(s + "\n")
			}
		}
	}

	responseCh := make(chan string, 10000)
	a.waitGroup.Add(1)
	go func() {
		defer a.waitGroup.Done()

		last := ""
		for a := range responseCh {
			if last == "" || last != a {
				f(a, false)
			} else {
				f(".", true)
			}

			last = a
		}
	}()

	a.responsePrinter = func(a string) {
		if a == CloseString {
			close(responseCh)
		} else {
			responseCh <- a
		}
	}
}

const CloseString = "<close>"

func (a *App) setupBodyPrint() {
	if a.body == "" {
		return
	}

	a.bodyPrintCh = make(chan string, 10000)

	bodyFile := a.body
	boydCond := ""
	if p := strings.Index(a.body, ":"); p > 0 {
		bodyFile = a.body[:p]
		boydCond = a.body[p+1:]
	}

	f, err := os.OpenFile(bodyFile, os.O_WRONLY|os.O_CREATE|os.O_APPEND, os.ModePerm)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Reponse body file %s created or appended!\n", bodyFile)

	a.waitGroup.Add(1)
	go func() {
		defer a.waitGroup.Done()

		for body := range a.bodyPrintCh {
			if boydCond != "" {
				body = jj.Get(body, boydCond).String()
			} else {
				body = string(jj.Ugly([]byte(body)))
			}
			_, _ = f.Write([]byte(body + "\n"))
		}
		f.Close()
	}()
}

func (a *App) weed(addr string) string {
	if a.weedVolumeAssignedUrl == nil {
		return addr
	}

	return <-a.weedVolumeAssignedUrl
}

func (a *App) setupWeed(c *fasthttp.Client) {
	if a.weedMasterURL == "" {
		return
	}

	a.weedVolumeAssignedUrl = make(chan string, 100)
	go a.assignFids(c)
}

// AssignResult contains assign result.
// Raw response: {"fid":"1,0a1653fd0f","url":"localhost:8899","publicUrl":"localhost:8899","count":1,"error":""}
type AssignResult struct {
	FileID    string `json:"fid,omitempty"`
	URL       string `json:"url,omitempty"`
	PublicURL string `json:"publicUrl,omitempty"`
	Count     int    `json:"count,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (a *App) assignFids(c *fasthttp.Client) {
	for i := 0; i < a.requestsTotal; i++ {
		r := &AssignResult{}
		if err := FastGet(c, a.weedMasterURL, r); err != nil || r.Error != "" {
			errMsg := r.Error
			if err != nil {
				errMsg = err.Error()
			}
			log.Printf("assign url error: %s", errMsg)
			a.weedVolumeAssignedUrl <- "err:" + errMsg
			continue
		}

		p := "http://" + r.PublicURL + "/" + r.FileID
		if r.Count <= 1 {
			a.weedVolumeAssignedUrl <- p
			continue
		}

		i += r.Count - 1
		for j := 0; j < r.Count; j++ {
			a.weedVolumeAssignedUrl <- p + fmt.Sprintf("_%d", j+1)
		}
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
func ReadUploadMultipartFile(fieldName, filePath string) (imageBytes []byte, contentType string, err error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

	part, err := writer.CreateFormFile(fieldName, filepath.Base(filePath))
	if err != nil {
		return nil, "", err
	}

	file := MustOpen(filePath)
	defer func() {
		if err := file.Close(); err != nil {
			log.Panicf("close file %s error %v", filePath, err)
		}
	}()

	_, _ = io.Copy(part, file)
	_ = writer.Close()

	return buffer.Bytes(), writer.FormDataContentType(), nil
}

// Throttle ...
type Throttle struct {
	tokenC, stopC chan bool
}

// MakeThrottle ...
func MakeThrottle(duration time.Duration) *Throttle {
	t := &Throttle{tokenC: make(chan bool, 1), stopC: make(chan bool, 1)}

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
func (t *Throttle) Stop() { t.stopC <- true }

// Allow ...
func (t *Throttle) Allow() bool {
	select {
	case <-t.tokenC:
		return true
	default:
		return false
	}
}

// NewProxyConn based on proxy url.
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

// ProxyConn is used to define the proxy.
type ProxyConn interface {
	Dial(network string, address string) (net.Conn, error)
}

// DefaultClient is used to implement a proxy in default.
type DefaultClient struct {
	rAddr *net.TCPAddr
}

// Dial implementation of ProxyConn.
// Set KeepAlive=-1 to reduce the call of syscall.
func (dc *DefaultClient) Dial(network string, address string) (conn net.Conn, err error) {
	if dc.rAddr == nil {
		if dc.rAddr, err = net.ResolveTCPAddr("tcp", address); err != nil {
			return nil, err
		}
	}
	return net.DialTCP(network, nil, dc.rAddr)
}

// Socks5Client is used to implement a proxy in socks5
type Socks5Client struct {
	proxyUrl *url.URL
}

// Dial implementation of ProxyConn.
func (s5 *Socks5Client) Dial(network string, address string) (net.Conn, error) {
	d, err := proxy.FromURL(s5.proxyUrl, nil)
	if err != nil {
		return nil, err
	}

	return d.Dial(network, address)
}

// HttpClient is used to implement a proxy in http.
type HttpClient struct {
	proxyUrl *url.URL
}

// Dial implementation of ProxyConn
func (hc *HttpClient) Dial(_ string, address string) (net.Conn, error) {
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

func CreateUri(baseUri, relativeUri string, query map[string]string) (string, error) {
	u, err := url.Parse(relativeUri)
	if err != nil {
		return "", err
	}

	q := u.Query()
	for k, v := range query {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	base, err := url.Parse(baseUri)
	if err != nil {
		return "", err
	}

	return base.ResolveReference(u).String(), nil
}

func FastGet(c *fasthttp.Client, requestUri string, out interface{}) error {
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	req.SetRequestURI(requestUri)

	rsp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(rsp)
	if err := c.Do(req, rsp); err != nil {
		return err
	}

	if out != nil {
		body := rsp.Body()
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("json Unmarshal %s error %w", body, err)
		}
	}

	return nil
}

func (a *App) processProfile(c *Conf) {
	if a.profile == "" {
		return
	}

	_, err := os.Stat(a.profile)
	if os.IsNotExist(err) {
		if err := os.WriteFile(a.profile, []byte(sampleProfile), os.ModePerm); err != nil {
			panic(err.Error())
		}

		fmt.Printf("sample profile %s generated!\n", a.profile)
		os.Exit(0)
	}

	if err != nil {
		panic(err.Error())
	}

	f, err := os.Open(a.profile)
	if err != nil {
		panic(err.Error())
	}
	defer f.Close()

	profiles, err := ReadHTTPFromFile(a.profileBaseURL, f)
	if err != nil {
		panic(err.Error())
	}

	c.profiles = profiles
}

type Profile struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    string
}

func ReadHTTPFromFile(baseUrl string, r io.Reader) ([]*Profile, error) {
	buf := bufio.NewReader(r)
	profiles, err := ParseRequests(baseUrl, buf)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return profiles, nil
		}

		return nil, err
	}

	return profiles, nil
}

func ParseRequests(baseUrl string, buf *bufio.Reader) ([]*Profile, error) {
	var profiles []*Profile
	var p *Profile

NEXT:
	for {
		l, err := buf.ReadString('\n')
		if err != nil {
			return profiles, err
		}

		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "#") {
			continue
		}
		if method, ok := HasAnyPrefix(l, http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch,
			http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace); ok {
			p = &Profile{
				Method:  method,
				URL:     fixUrl(baseUrl, strings.TrimSpace(l[len(method):])),
				Headers: make(map[string]string),
			}
			profiles = append(profiles, p)
		}

		if p == nil {
			continue
		}

		for {
			l, err = buf.ReadString('\n')
			if err != nil {
				return profiles, err
			}

			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "#") {
				goto NEXT
			}
			if l == "" {
				break
			}

			pos := strings.Index(l, ":")
			if pos > 0 {
				k := strings.TrimSpace(l[:pos])
				v := strings.TrimSpace(l[pos+1:])
				p.Headers[k] = v
			}
		}

		for {
			l, err = buf.ReadString('\n')
			if err != nil {
				return profiles, err
			}

			l = strings.TrimSpace(l)
			if strings.HasPrefix(l, "#") {
				goto NEXT
			}
			if l == "" {
				break
			}

			p.Body += l
		}

		p = nil
	}
}

func HasAnyPrefix(l string, subs ...string) (string, bool) {
	for _, sub := range subs {
		if strings.HasPrefix(l, sub+" ") {
			return sub, true
		}
	}

	return "", false
}

const sampleProfile = `
###
GET http://127.0.0.1:1080

###
POST http://127.0.0.1:1080
Content-Type: application/json;charset=utf-8
Bingoo-Name: bingoohuang

{"name": "bingoohuang", "age": 1000}
`

var (
	reScheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+-.]*://`)
)

func fixUrl(baseUrl, s string) string {
	if baseUrl != "" {
		return s
	}

	defaultScheme := "http"
	defaultHost := "localhost"

	if s == ":" {
		s = ":80"
	}

	// ex) :8080/hello or /hello or :
	if strings.HasPrefix(s, ":") || strings.HasPrefix(s, "/") {
		s = defaultHost + s
	}

	// ex) example.com/hello
	if !reScheme.MatchString(s) {
		s = defaultScheme + "://" + s
	}

	u, err := url.Parse(s)
	if err != nil {
		panic(err.Error())
	}

	u.Host = strings.TrimSuffix(u.Host, ":")
	if u.Path == "" {
		u.Path = "/"
	}

	return u.String()
}
