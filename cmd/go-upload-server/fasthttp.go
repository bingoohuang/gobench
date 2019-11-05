package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"time"

	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/reuseport"
)

func fasthttpUpload(ctx *fasthttp.RequestCtx) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		fmt.Println("cost time", duration)
	}()

	file, err := ctx.FormFile("file")
	if err != nil {
		fmt.Println(err)
		return
	}
	duration := time.Since(start)
	fmt.Println("FormFile time", duration)

	tmpFile := CreateTmpFile()

	err = fasthttp.SaveMultipartFile(file, tmpFile)
	if err != nil {
		fmt.Println(err)
		return
	}
}

// refer https://github.com/networknt/microservices-framework-benchmark/blob/master/go-fasthttp/server.go
func createForkListener(listenAddr string) net.Listener {
	if !child {
		children := make([]*exec.Cmd, runtime.NumCPU())
		for i := range children {
			children[i] = exec.Command(os.Args[0], "-impl", "fasthttp", "-child")
			children[i].Stdout = os.Stdout
			children[i].Stderr = os.Stderr
			if err := children[i].Start(); err != nil {
				log.Fatal(err)
			}
		}
		for _, ch := range children {
			if err := ch.Wait(); err != nil {
				log.Print(err)
			}
		}
		os.Exit(0)
	}

	runtime.GOMAXPROCS(1)
	ln, err := reuseport.Listen("tcp4", listenAddr)
	if err != nil {
		log.Fatal(err)
	}
	return ln
}
