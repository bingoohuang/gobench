package main

import (
	"flag"
	"fmt"
	"github.com/valyala/fasthttp"
	"net/http"
)

var (
	impl           string
	sampleFileSize int64
	port           string
)

func init() {
	flag.StringVar(&port, "port", "8811", "listen port")
	flag.StringVar(&impl, "impl", "", "implementation: nethttp/fasthttp")
	flag.Int64Var(&sampleFileSize, "sampleFileSize", 0, "create sampling file with specified size and exit")

	flag.Parse()
}

func main() {
	if sampleFileSize > 0 {
		CreateFixedSizedFile(sampleFileSize)
		return
	}

	switch impl {
	case "nethttp":
		http.HandleFunc("/upload", NetHttpUpload)
		_ = http.ListenAndServe(":"+port, nil)
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

		_ = s.ListenAndServe(":" + port)
	default:
		fmt.Println("go upload server")
		flag.PrintDefaults()
	}
}
