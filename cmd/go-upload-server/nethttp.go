package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func NetHttpUpload(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		fmt.Println("cost time", duration)
	}()

	// _ = r.ParseMultipartForm(16 << 20) // 16 MiB
	file, _, err := r.FormFile("file")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer file.Close()

	duration := time.Since(start)
	fmt.Println("FormFile time", duration)

	tmpFile := CreateTmpFile()
	out, err := os.Create(tmpFile)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer out.Close()
	_, _ = io.Copy(out, file)
}
