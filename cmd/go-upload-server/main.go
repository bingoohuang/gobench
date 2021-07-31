package main

import (
	"flag"
	"fmt"
	"github.com/bingoohuang/gg/pkg/man"
	"net/http"
	"os"

	"github.com/valyala/fasthttp"
)

var (
	impl           string // nolint:gochecknoglobals
	sampleFileSize string // nolint:gochecknoglobals
	addr           string // nolint:gochecknoglobals
	child          bool   // nolint:gochecknoglobals
	fork           bool   // nolint:gochecknoglobals

	maxRequestBodySize string // nolint:gochecknoglobals
)

func init() { // nolint:gochecknoinits
	flag.StringVar(&maxRequestBodySize, "maxRequestBodySize", "10MiB", "max upload file size")
	flag.StringVar(&addr, "addr", ":8811", "listen address")
	flag.StringVar(&impl, "impl", "nethttp", "implementation: nethttp/fasthttp")
	flag.StringVar(&sampleFileSize, "sampleFileSize", "",
		"create sampling file with specified size （eg. 44kB, 17MB)） and exit")
	flag.BoolVar(&fork, "fork", false, "fork children processes when --imp=fasthttp")
	flag.BoolVar(&child, "child", false,
		"flag child process when --imp=fasthttp (CAUTION: only used internally by program)")

	flag.Parse()
}

type maxBytesHandler struct {
	h http.Handler
	n int64
}

func (h *maxBytesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.n)
	h.h.ServeHTTP(w, r)
}

func main() {
	createSampleFile()

	parsedMaxRequestBodySize := parseMaxRequestBodySize()

	fmt.Println("listen at", addr)

	switch impl {
	case "fasthttp":
		fasthttpImpl(parsedMaxRequestBodySize)

	default:
		nethttpImpl(parsedMaxRequestBodySize)
	}
}

func nethttpImpl(parsedMaxRequestBodySize uint64) {
	http.HandleFunc("/upload", NetHTTPUpload)

	if err := http.ListenAndServe(addr,
		&maxBytesHandler{
			h: http.DefaultServeMux,
			n: int64(parsedMaxRequestBodySize),
		},
	); err != nil {
		fmt.Println(err)
	}
}

func fasthttpImpl(parsedMaxRequestBodySize uint64) {
	m := func(ctx *fasthttp.RequestCtx) {
		switch string(ctx.Path()) {
		case "/upload":
			fasthttpUpload(ctx)
		default:
			ctx.Error("not found", fasthttp.StatusNotFound)
		}
	}
	s := &fasthttp.Server{
		Handler:            m,
		MaxRequestBodySize: int(parsedMaxRequestBodySize),
	}

	if fork {
		if err := s.Serve(createForkListener(addr)); err != nil {
			fmt.Println(err)
		}
	} else {
		if err := s.ListenAndServe(addr); err != nil {
			fmt.Println(err)
		}
	}
}

func createSampleFile() {
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

func parseMaxRequestBodySize() uint64 {
	var (
		m   uint64
		err error
	)

	if m, err = man.ParseBytes(maxRequestBodySize); err != nil {
		fmt.Println("ParseBytes", maxRequestBodySize, "error", err)
		os.Exit(1)
	}

	return m
}
