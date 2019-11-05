package main

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync/atomic"
)

func CreateFixedSizedFile(size uint64) {
	fixFile := "/tmp/fix" + strconv.FormatUint(size, 10)
	fmt.Println("fixFile", fixFile)

	out, _ := os.Create(fixFile)
	_, _ = io.CopyN(out, rand.Reader, int64(size))
	_ = out.Close()
}

var x int64

func CreateTmpFile() string {
	seq := atomic.AddInt64(&x, 1)

	if seq >= 1000 {
		atomic.StoreInt64(&x, 1)
		seq = 1
	}

	tempFile := "/tmp/fub" + strconv.FormatInt(seq, 10)
	fmt.Println("tempFile", tempFile)

	return tempFile
}
