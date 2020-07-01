package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/dustin/go-humanize"
)

// NetHTTPUpload upload
// nolint:gomnd
func NetHTTPUpload(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	defer func() {
		duration := time.Since(start)
		fmt.Println("cost time", duration)
	}()

	if err := r.ParseMultipartForm(16 /*16 MiB */ << 20); err != nil {
		fmt.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		fmt.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	defer file.Close()

	duration := time.Since(start)
	fmt.Println("FormFile time", duration)

	tmpFile := CreateTmpFile()
	out, err := os.Create(tmpFile)

	if err != nil {
		fmt.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	defer out.Close()

	written, err := io.Copy(out, file)
	if err != nil {
		fmt.Println(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)

		return
	}

	writeJSON(w, uploadResult{
		CostTime: duration.String(),
		TempFile: tmpFile,
		FileSize: humanize.Bytes(uint64(written)),
	})
}

type uploadResult struct {
	CostTime string
	TempFile string
	FileSize string
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	js, err := json.Marshal(v)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(js)
}
