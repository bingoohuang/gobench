package go_upload_server

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

func NetHttpUpload(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseMultipartForm(10240)
	file, _, err := r.FormFile("file")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer file.Close()

	tmpFile := CreateTmpFile()
	out, err := os.Create(tmpFile)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer out.Close()
	_, _ = io.Copy(out, file)
}
