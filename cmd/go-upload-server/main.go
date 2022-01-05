package main

import (
	"crypto/rand"
	"encoding/json"
	"flag"
	"github.com/bingoohuang/gg/pkg/man"
	"github.com/valyala/fasthttp/reuseport"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/valyala/fasthttp"
)

func main() {
	max := flag.String("max", "10MiB", "max upload file size")
	addr := flag.String("addr", ":8811", "listen address")
	impl := flag.String("impl", "net", "implementation: net/fast")
	sample := flag.String("sample", "",
		"create sampling file with specified size （eg. 44kB, 17MB)） and exit")
	fork := flag.Bool("fork", false, "fork children processes when --imp=fast")
	child := flag.Bool("child", false,
		"flag child process when --imp=fast (CAUTION: only used internally by program)")
	flag.Parse()

	createSampleFile(*sample)
	log.Printf("listen at %s", *addr)

	var err error
	maxBodySize := parseMaxRequestBodySize(*max)

	switch *impl {
	case "fast":
		err = fastServe(maxBodySize, *addr, *fork, *child)
	default:
		err = netServe(maxBodySize, *addr)
	}

	if err != nil {
		log.Printf("error %v", err)
	}
}

func netServe(maxBodySize uint64, addr string) error {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if err := NetHTTPUpload(w, r); err != nil {
			log.Printf("error occurred: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	return http.ListenAndServe(addr, &maxBytesHandler{h: http.DefaultServeMux, n: int64(maxBodySize)})
}

type maxBytesHandler struct {
	h http.Handler
	n int64
}

func (h *maxBytesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.n)
	h.h.ServeHTTP(w, r)
}

func fastServe(maxRequestBodySize uint64, addr string, fork, child bool) error {
	m := func(ctx *fasthttp.RequestCtx) {
		switch string(ctx.Path()) {
		case "/":
			if err := fastUpload(ctx); err != nil {
				log.Printf("error occurred: %v", err)
				ctx.Error(err.Error(), http.StatusInternalServerError)
			}
		default:
			ctx.Error("not found", fasthttp.StatusNotFound)
		}
	}
	s := &fasthttp.Server{
		Handler:            m,
		MaxRequestBodySize: int(maxRequestBodySize),
	}

	if fork {
		return s.Serve(createForkListener(addr, child))
	}

	return s.ListenAndServe(addr)
}

func createSampleFile(sampleFileSize string) {
	if sampleFileSize == "" {
		return
	}

	fixedSize, err := man.ParseBytes(sampleFileSize)
	if err != nil {
		panic(err)
	}

	CreateFixedSizedFile(fixedSize)
	os.Exit(0)
}

func parseMaxRequestBodySize(maxRequestBodySize string) uint64 {
	m, err := man.ParseBytes(maxRequestBodySize)
	if err != nil {
		log.Fatalf("ParseBytes %s error: %v", maxRequestBodySize, err)
	}

	return m
}

func fastUpload(ctx *fasthttp.RequestCtx) error {
	start := time.Now()
	defer log.Printf("cost time %s", time.Since(start))

	file, err := ctx.FormFile("file")
	if err != nil {
		return err
	}

	duration := time.Since(start)
	log.Printf("FormFile time %s", duration)

	tmpFile := CreateTmpFile()
	if err := fasthttp.SaveMultipartFile(file, tmpFile); err != nil {
		return err
	}

	stat, err := os.Stat(tmpFile)
	if err != nil {
		return err
	}
	n := stat.Size()

	return writeCtxJSON(ctx, uploadResult{
		CostTime: duration.String(),
		TempFile: tmpFile,
		FileSize: man.Bytes(uint64(n)),
	})
}

// refer https://github.com/networknt/microservices-framework-benchmark/blob/master/go-fasthttp/server.go
func createForkListener(listenAddr string, child bool) net.Listener {
	if child {
		runtime.GOMAXPROCS(1)
		ln, err := reuseport.Listen("tcp4", listenAddr)
		if err != nil {
			log.Fatal(err)
		}

		return ln
	}

	children := make([]*exec.Cmd, runtime.NumCPU())

	for i := range children {
		children[i] = exec.Command(os.Args[0], "-impl", "fast", "-child") //nolint:gosec
		children[i].Stdout = os.Stdout
		children[i].Stderr = os.Stderr
		if err := children[i].Start(); err != nil {
			log.Fatal(err)
		}
	}

	for _, ch := range children {
		if err := ch.Wait(); err != nil {
			log.Printf("wait error: %v", err)
		}
	}

	os.Exit(0)
	return nil
}

// NetHTTPUpload upload
func NetHTTPUpload(w http.ResponseWriter, r *http.Request) error {
	start := time.Now()
	defer log.Printf("cost time %s", time.Since(start))

	if err := r.ParseMultipartForm(16 /*16 MiB */ << 20); err != nil {
		return err
	}

	key := "file"
	if r.MultipartForm != nil && r.MultipartForm.File != nil {
		for k := range r.MultipartForm.File {
			key = k
			break
		}
	}

	file, _, err := r.FormFile(key)
	if err != nil {
		return err
	}

	if f, ok := file.(*os.File); ok {
		defer func(name string) {
			log.Printf("remove tmpfile: %s", name)
			if err := os.Remove(name); err != nil {
				log.Printf("tmpfile: %s remove failed: %v", name, err)
			}
		}(f.Name())
	}

	defer file.Close()

	duration := time.Since(start)
	log.Printf("FormFile time %s", duration)

	tmpFile := CreateTmpFile()
	out, err := os.Create(tmpFile)
	if err != nil {
		return err
	}

	defer out.Close()

	n, err := io.Copy(out, file)
	if err != nil {
		return err
	}

	return writeJSON(w, uploadResult{
		CostTime: duration.String(),
		TempFile: tmpFile,
		FileSize: man.Bytes(uint64(n)),
	})
}

type uploadResult struct {
	CostTime string
	TempFile string
	FileSize string
}

func writeCtxJSON(ctx *fasthttp.RequestCtx, v interface{}) error {
	js, err := json.Marshal(v)
	if err != nil {
		return err
	}

	ctx.Response.Header.Set("Content-Type", "application/json")
	_, err = ctx.Write(js)
	return err
}

func writeJSON(w http.ResponseWriter, v interface{}) error {
	js, err := json.Marshal(v)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(js)
	return err
}

// CreateFixedSizedFile creates a fixed sized file.
func CreateFixedSizedFile(size uint64) {
	fixFile := "/tmp/fix" + strconv.FormatUint(size, 10)
	log.Printf("fixFile %s", fixFile)

	out, _ := os.Create(fixFile)
	_, _ = io.CopyN(out, rand.Reader, int64(size))
	_ = out.Close()
}

var x int64

// CreateTmpFile creates a temp file.
func CreateTmpFile() string {
	seq := atomic.AddInt64(&x, 1)
	if seq >= 1000 {
		atomic.StoreInt64(&x, 1)
		seq = 1
	}

	tmpFile := "/tmp/fub" + strconv.FormatInt(seq, 10)
	log.Printf("tmpFile %s created", tmpFile)

	return tmpFile
}
