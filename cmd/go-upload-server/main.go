package main

import (
	"flag"
	"net/http"

	"github.com/dustin/go-humanize"
	"github.com/valyala/fasthttp"
)

var (
	impl           string
	sampleFileSize string
	port           string
	child          bool
	fork           bool
)

func init() {
	flag.StringVar(&port, "port", "8811", "listen port")
	flag.StringVar(&impl, "impl", "nethttp", "implementation: nethttp/fasthttp")
	flag.StringVar(&sampleFileSize, "sampleFileSize", "", "create sampling file with specified size （eg. 44kB, 17MB)） and exit")
	flag.BoolVar(&fork, "fork", false, "fork children processes when --imp=fasthttp")
	flag.BoolVar(&child, "child", false, "flag child process when --imp=fasthttp (CAUTION: only used internally by program)")

	flag.Parse()
}

func main() {
	if sampleFileSize != "" {
		fixedSize, err := humanize.ParseBytes(sampleFileSize)
		if err != nil {
			panic(err)
		}
		CreateFixedSizedFile(fixedSize)
		return
	}

	switch impl {
	case "fasthttp":
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
			MaxRequestBodySize: 10 << 20, // 10 MiB
		}

		if fork {
			ln := createForkListener(":" + port)
			_ = s.Serve(ln)
		} else {
			_ = s.ListenAndServe(":" + port)
		}
	default:
		http.HandleFunc("/upload", NetHttpUpload)
		_ = http.ListenAndServe(":"+port, nil)
	}
}
