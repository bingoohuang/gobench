package main

import (
	"fmt"
	"github.com/valyala/fasthttp"
)

func fasthttpUpload(ctx *fasthttp.RequestCtx) {
	file, err := ctx.FormFile("file")
	if err != nil {
		fmt.Println(err)
		return
	}

	tmpFile := CreateTmpFile()
	err = fasthttp.SaveMultipartFile(file, tmpFile)
	if err != nil {
		fmt.Println(err)
		return
	}
}
