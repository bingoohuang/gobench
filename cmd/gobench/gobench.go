package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/bingoohuang/gg/pkg/badgerdb"
	"github.com/bingoohuang/gg/pkg/bytex"
	"github.com/bingoohuang/gg/pkg/filex"
	flag "github.com/bingoohuang/gg/pkg/fla9"
	"github.com/bingoohuang/gg/pkg/netx"
	"github.com/bingoohuang/gg/pkg/osx"
	"github.com/bingoohuang/gg/pkg/randx"
	"github.com/bingoohuang/gg/pkg/rest"
	"github.com/bingoohuang/gg/pkg/ss"
	"github.com/bingoohuang/gg/pkg/thinktime"
	"github.com/bingoohuang/gg/pkg/vars"
	"github.com/bingoohuang/jj"
	"github.com/cheggaaa/pb/v3"
	"github.com/mitchellh/go-homedir"
	"io"
	"io/ioutil"
	"log"
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
	"sync"
	"sync/atomic"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/karrick/godirwalk"

	"github.com/bingoohuang/golang-trial/randimg"
	"github.com/bingoohuang/govaluate"
	"github.com/dustin/go-humanize"
	"github.com/pkg/errors"

	"github.com/valyala/fasthttp"

	"github.com/docker/go-units"
	_ "net/http/pprof"
)

const versionInfo = "v1.1.0 at 2021-06-07 18:49:38"

// App ...
type App struct {
	requests, requestsTotal, goroutines, connections int
	method, duration, urls, postData, uFilePath      string
	keepAlive                                        bool
	uploadRandImg, printResult                       string

	timeout                  time.Duration
	rThroughput, wThroughput uint64

	basic, uFieldName, fixedImgSize, contentType, think, proxy string
	weedMasterURL, pprof, profile, profileBaseURL, eval        string

	badger string

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

	randomCh  chan []byte
	thinkTime *thinktime.ThinkTime
}

// Conf for gobench.
type Conf struct {
	urls                           []string
	postData                       []byte
	requests, firstRequests        int
	duration                       time.Duration
	keepAlive                      bool
	method, basicAuth, contentType string

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
  -basic           Basic Auth, username:password
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
  -badger          Badger DB path
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
	flag.StringVar(&a.basic, "basic", "", "")
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
	flag.StringVar(&a.badger, "badger", "", "")

	flag.Parse()
	if *version {
		fmt.Println(versionInfo)
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

type Valuer struct {
	Map map[string]interface{}
}

func NewValuer() *Valuer {
	return &Valuer{
		Map: make(map[string]interface{}),
	}
}

func (v *Valuer) Register(fn string, f jj.SubstitutionFn) {
	jj.DefaultSubstituteFns.Register(fn, f)
}

func (v *Valuer) Value(name, params string) interface{} {
	if x, ok := v.Map[name]; ok {
		return x
	}

	if !strings.HasSuffix(name, "_keep") {
		return jj.DefaultGen.Value(name, params)
	}

	name = strings.TrimSuffix(name, "_keep")
	if x, ok := v.Map[name]; ok {
		return x
	}

	x := jj.DefaultGen.Value(name, params)
	v.Map[name] = x

	return x
}

func evaluator(ss ...string) []string {
	ret := make([]string, len(ss))

	valuer := NewValuer()
	gen := jj.NewGenContext(valuer)
	for i, s := range ss {
		if jj.Valid(s) {
			ret[i] = gen.Gen(s)
		} else {
			ret[i] = vars.ToString(vars.ParseExpr(s).Eval(valuer))
		}
	}

	return ret
}

func (a *App) parseEval(eval string) {
	if eval == "" {
		a.seqFn = evaluator
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
			valuer := NewValuer()
			gen := jj.NewGenContext(valuer)
			for i, s := range ss {
				s = strings.ReplaceAll(s, pattern, prepend+seqStr)
				if jj.Valid(s) {
					ret[i] = gen.Gen(s)
				} else {
					ret[i] = vars.ToString(vars.ParseExpr(s).Eval(valuer))
				}
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
				valuer := NewValuer()
				gen := jj.NewGenContext(valuer)
				for i, s := range ss {
					s = strings.ReplaceAll(s, pattern, left+seqStr+right)
					if jj.Valid(s) {
						ret[i] = gen.Gen(s)
					} else {
						ret[i] = vars.ToString(vars.ParseExpr(s).Eval(valuer))
					}
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

func (a *App) thinking() {
	if a.thinkTime == nil {
		return
	}

	think := a.thinkTime.Think(false)
	a.responsePrinter("think " + think.String() + "...")
	ticker := time.NewTicker(think)
	defer ticker.Stop()

	select {
	case <-a.exitChan:
	case <-ticker.C:
	}
}

func (a *App) parseThinkTime() (err error) {
	a.thinkTime, err = thinktime.ParseThinkTime(a.think)
	return err
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

	c := app.NewConfiguration()

	app.setupWeed(&c.myClient)
	app.setupBadgerDb()

	resultChan := make(chan requestResult)
	totalReqsChan := make(chan int)
	statComplete := make(chan bool)

	var rr requestResult
	go stating(resultChan, &rr, totalReqsChan, statComplete)

	requestsChan := make(chan int, app.goroutines)
	barIncr, barFinish := createBars(app, c)

	startTime := time.Now()
	fmt.Printf("Dispatching %d goroutines at %s\n", app.goroutines, startTime.Format("2006-01-02 15:04:05.000"))

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
				if diff := bar.Total() - bar.Current(); diff > 0 {
					bar.Add64(diff)
				}
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

	fmt.Fprintf(w, "Total requests:\t%d hits\n", totalRequests)
	fmt.Fprintf(w, "OK    requests:\t%d hits\n", rr.success)
	fmt.Fprintf(w, "Network failed(NF):\t%d hits\n", rr.networkFailed)
	if a.cond == nil {
		fmt.Fprintf(w, "Bad requests(!2xx)(BF):\t%d hits\n", rr.badFailed)
	} else {
		fmt.Fprintf(w, "Bad requests(!2xx/%s)(BF):\t%d hits\n", a.cond, rr.badFailed)
	}
	fmt.Fprintf(w, "OK requests rate:\t%0.3f hits/sec\n", float64(rr.success)/elapsedSeconds)
	fmt.Fprintf(w, "Read throughput:\t%s/sec\n",
		humanize.IBytes(uint64(float64(a.rThroughput)/elapsedSeconds)))
	fmt.Fprintf(w, "Write throughput:\t%s/sec\n",
		humanize.IBytes(uint64(float64(a.wThroughput)/elapsedSeconds)))
	fmt.Fprintf(w, "Test time:\t%s(%s-%s)\n", elapsed.Round(time.Millisecond).String(),
		startTime.Format("2006-01-02 15:04:05.000"), endTime.Format("15:04:05.000"))

	if !methodsStat.ShowStats(w) {
		fmt.Fprintf(w, "Max X-Gobench-Seq:\t%d\n", SecCur())
	}
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
		urls:      make([]string, 0),
		postData:  nil,
		keepAlive: a.keepAlive,
		requests:  a.requests,
		basicAuth: a.basic,
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

		c.urls, err = filex.Lines(a.urls[1:])
		if err != nil {
			log.Fatalf("Error in ioutil.ReadFile for file: %s Error: %v", a.urls[1:], err)
		}
	} else {
		c.urls = strings.Split(a.urls, "#")
	}

	for i, u := range c.urls {
		c.urls[i] = fixUrl("", u)
	}
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
	a.uFilePath = osx.ExpandHome(a.uFilePath)
	fs, err := os.Stat(a.uFilePath)
	if err != nil && os.IsNotExist(err) {
		log.Fatalf("%s dos not exist", a.uFilePath)
	}
	if err != nil {
		log.Fatalf("stat file %s error  %v", a.uFilePath, err)
	}

	// 单个上传文件
	c.method = ss.Or(c.method, http.MethodPost)
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

					if randx.Int64N(10) != 0 {
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

	c.method = ss.Or(c.method, http.MethodPost)

	if a.postData != "" {
		if strings.HasPrefix(a.postData, "@") {
			postDataFile, _ := homedir.Expand(a.postData[1:])
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
		size = (randx.Int64N(4) + 1) << 20 //  << 20 means MiB
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
		urlIndex = randx.IntN(len(conf.urls))
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
		c.method = ss.Or(c.method, http.MethodPost)
		postData, contentType, fileName = a.randomImage(a.uploadRandImg, a.fixedImgSize)
	}

	c.method = ss.Or(c.method, http.MethodGet)

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

type MethodInfo struct {
	Count      int
	GobenchSeq int32
	requestResult
}
type MethodsStat struct {
	Stat map[string]*MethodInfo
	sync.Mutex
}

var methodsStat = &MethodsStat{Stat: make(map[string]*MethodInfo)}

func (s *MethodsStat) Incr(method string, gobenchSeq int32) {
	s.Lock()
	defer s.Unlock()

	v, ok := s.Stat[method]
	if !ok {
		v = &MethodInfo{}
		s.Stat[method] = v
	}

	v.Count++
	v.GobenchSeq = gobenchSeq
}

func (s *MethodsStat) IncrResult(method string, rr requestResult) {
	s.Lock()
	defer s.Unlock()

	v, ok := s.Stat[method]
	if !ok {
		v = &MethodInfo{}
		s.Stat[method] = v
	}

	v.success += rr.success
	v.networkFailed += rr.networkFailed
	v.badFailed += rr.badFailed
}

func (s *MethodsStat) ShowStats(w *tabwriter.Writer) bool {
	s.Lock()
	defer s.Unlock()

	if len(s.Stat) == 0 {
		return false
	}

	methods := make([]string, 0, len(s.Stat))
	counts := make([]string, 0, len(s.Stat))
	seqs := make([]string, 0, len(s.Stat))
	for k, v := range s.Stat {
		methods = append(methods, k)
		if v.badFailed == 0 && v.networkFailed == 0 {
			counts = append(counts, fmt.Sprintf("%d", v.Count))
		} else {
			counts = append(counts, fmt.Sprintf("%d(OK:%d NF:%d BF:%d)", v.Count, v.success, v.networkFailed, v.badFailed))
		}
		seqs = append(seqs, fmt.Sprintf("%d", v.GobenchSeq))
	}
	fmt.Fprintf(w, "# of %s:\t%s\n", strings.Join(methods, "/"), strings.Join(counts, "/"))
	fmt.Fprintf(w, "Last X-Gobench-Seq %s:\t%s\n", strings.Join(methods, "/"), strings.Join(seqs, "/"))
	return true
}

func (a *App) execProfile(rc chan requestResult, cnf *Conf, addr string, rsp *fasthttp.Response, pr *Profile, err error, statusCode int, fileName string) {
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	rsp = fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(rsp)

	if a.profileBaseURL != "" {
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
	gobenchSeq := SetGobenchHeaders(req)
	methodsStat.Incr(pr.Method, gobenchSeq)
	req.SetBody([]byte(pr.Body))

	err = cnf.myClient.Do(req, rsp)
	statusCode = rsp.StatusCode()
	rr := a.checkResult(rc, err, statusCode, rsp, addr, fileName)
	methodsStat.IncrResult(pr.Method, rr)
}

var seq int32

func SecCur() int32 { return atomic.LoadInt32(&seq) }
func SeqInc() int32 { return atomic.AddInt32(&seq, 1) }

func SetGobenchHeaders(req *fasthttp.Request) int32 {
	seq := SeqInc()
	SetHeader(req, "X-Gobench-Seq", fmt.Sprintf("%d", seq))
	SetHeader(req, "X-Gobench-Time", time.Now().Format(`2006-01-02 15:04:05.000`))
	return seq
}

func (a *App) exec(rc chan requestResult, cnf *Conf, addr string, method string, contentType string, fileName string,
	postData []byte, rsp *fasthttp.Response, err error, statusCode int) {
	req := fasthttp.AcquireRequest()
	defer fasthttp.ReleaseRequest(req)
	rsp = fasthttp.AcquireResponse()
	defer fasthttp.ReleaseResponse(rsp)

	if a.randomCh != nil {
		v := <-a.randomCh
		addr = vars.ParseExpr(addr).Eval(vars.ValuerHandler(func(name, params string) interface{} {
			return jj.GetBytes(v, name).String()
		})).(string)
	} else {
		if fileName == "" { // 文件上传时，不处理请求体
			ret := a.seqFn(addr, string(postData))
			addr = ret[0]
			postData = []byte(ret[1])
		} else {
			addr = a.seqFn(addr)[0]
		}
	}

	now := Now()
	a.responsePrinter(now + "URL: " + addr)
	if fileName == "" {
		a.responsePrinter(now + "POST: " + string(postData))
	}

	req.SetRequestURI(fixUrl("", addr))
	req.Header.SetMethod(method)
	SetHeader(req, "Connection", ss.If(cnf.keepAlive, "keep-alive", "close"))
	if cnf.basicAuth != "" {
		SetHeader(req, "Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(cnf.basicAuth)))
	}
	SetHeader(req, "Content-Type", contentType)
	SetGobenchHeaders(req)

	req.SetBody(postData)

	err = cnf.myClient.Do(req, rsp)
	statusCode = rsp.StatusCode()

	a.checkResult(rc, err, statusCode, rsp, addr, fileName)
}

func (a *App) checkResult(rc chan requestResult, err error, statusCode int, rsp *fasthttp.Response, addr string, fileName string) requestResult {
	var rr requestResult
	var resultDesc string

	switch {
	case err != nil:
		rr.networkFailed = 1
		resultDesc = "[X] " + err.Error()
	case statusCode >= 200 && statusCode < 300:
		if rspBody, ok := a.isOK(rsp); ok {
			rr.success = 1
			resultDesc = "[√]"
		} else {
			rr.badFailed = 1
			resultDesc = "[x]"
			log.Printf("failed addr: %s", addr)
			log.Printf("failed resp: %s", rspBody)
		}
	default:
		rr.badFailed = 1
		resultDesc = "[x]"
	}

	a.printResponse(addr, fileName, resultDesc, statusCode, rsp)

	rc <- rr
	return rr
}

func (a *App) isOK(resp *fasthttp.Response) ([]byte, bool) {
	if a.cond == nil {
		return nil, true
	}

	body := resp.Body()
	if !jj.ValidBytes(body) {
		return body, false
	}

	condVars := a.cond.Vars()
	parameters := make(map[string]interface{}, len(condVars))
	for _, v := range condVars {
		v1 := strings.ReplaceAll(v, "_", ".")
		jsonValue := jj.GetBytes(body, v1)
		parameters[v] = jsonValue.Value()
	}

	result, err := a.cond.Evaluate(parameters)
	if err != nil {
		return body, false
	}

	yes, ok := result.(bool)
	return body, yes && ok
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

	r := Now()
	if a.weedVolumeAssignedUrl != nil {
		r += "url: " + addr + " "
	}

	if fileName != "" {
		r += "file: " + fileName + " "
	}

	r += resultDesc + " [" + strconv.Itoa(statusCode) + "] " + resp.String()
	a.responsePrinter(r)
}

func Now() string { return time.Now().Format(`2006-01-02 15:04:05.000 `) }

// SetHeader set request header if value is not empty.
func SetHeader(r *fasthttp.Request, header, value string) {
	if value != "" {
		r.Header.Set(header, value)
	}
}

// MyDialer create Dial function.
func (a *App) MyDialer() func(address string) (conn net.Conn, err error) {
	return func(address string) (net.Conn, error) {
		dialer, err := netx.NewProxyDialer(a.proxy)
		if err != nil {
			return nil, err
		}

		conn, err := dialer.Dial("tcp", address)
		if err != nil {
			return nil, err
		}

		return netx.NewStatConnReadWrite(conn, &a.rThroughput, &a.wThroughput), nil
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

	a.weedMasterURL = fixUrl("", a.weedMasterURL)
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

// ReadUploadMultipartFile read file filePath for upload in multipart,
// return multipart content, form data content type and error.
func ReadUploadMultipartFile(fieldName, filePath string) (imageBytes []byte, contentType string, err error) {
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)

	part, err := writer.CreateFormFile(fieldName, filepath.Base(filePath))
	if err != nil {
		return nil, "", err
	}

	file := filex.Open(filePath)
	defer file.Close()

	_, _ = io.Copy(part, file)
	_ = writer.Close()

	return buffer.Bytes(), writer.FormDataContentType(), nil
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

	base, err := url.Parse(fixUrl("", baseUri))
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

type msg struct {
	t      time.Time
	format string
	v      []interface{}
}

type DelayLogger struct {
	stop chan struct{}
	msgs []msg
}

func (l *DelayLogger) Stop(format string, v ...interface{}) {
	l.Printf(format, v...)
	l.stop <- struct{}{}
}

func (l DelayLogger) Printf(format string, v ...interface{}) {
	l.msgs = append(l.msgs, msg{t: time.Now(), format: format, v: v})
}

func NewDelayLogger(expire time.Duration) *DelayLogger {
	d := &DelayLogger{stop: make(chan struct{})}
	go func() {
		select {
		case <-time.After(expire):
			for _, m := range d.msgs {
				v := append([]interface{}{m.t.Format(time.RFC3339)}, m.v...)
				log.Printf("real time %s, "+m.format, v...)
			}
			return
		case <-d.stop:
			return
		}
	}()

	return d
}

func (a *App) setupBadgerDb() {
	path := a.badger
	if path == "" {
		return
	}

	maxStr := regexp.MustCompile(`:\d+$`).FindString(path)
	max := int64(600000000)
	if maxStr != "" {
		max = ss.ParseInt64(maxStr[1:])
		path = path[:len(path)-len(maxStr)]
	}

	// maybe open badger cost more than 1 min for large data like 6亿, so log it
	delayLogger := NewDelayLogger(3 * time.Second)
	delayLogger.Printf("openning badger %s", path)
	db, err := badgerdb.Open(badgerdb.WithPath(path))
	if err != nil {
		panic(err)
	}
	delayLogger.Stop("openned badger %s", path)

	a.randomCh = make(chan []byte, 1000)
	go func() {
		defer db.Close()

		for a.ctx.Err() == nil {
			n := bytex.FromUint64(randx.Uint64N(max))
			db.Walk(func(k, v []byte) error {
				if v[0] != '{' {
					v, _ = jj.SetBytes([]byte(`{"v":""}`), "v", v)
				}
				select {
				case a.randomCh <- v:
				default:
					return io.EOF
				}
				return nil
			}, badgerdb.WithStart(n))
		}
	}()
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

func fixUrl(baseUrl, s string) string {
	if baseUrl != "" {
		return s
	}

	v, err := rest.FixURI(s)
	if err != nil {
		panic(err)
	}

	return v
}

func init() {
	jj.DefaultGen.RegisterFn("now", func(args string) interface{} { return time.Now().Format(time.RFC3339Nano) })
}
