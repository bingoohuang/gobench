package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/hashicorp/go-cleanhttp"

	"github.com/hashicorp/go-retryablehttp"
)

func main() {
	server := flag.String("server", "http://127.0.1:8812", "server get url")
	sleep := flag.String("sleep", "1m", "sleep span")
	retry := flag.Bool("retry", false, "retry if fail")
	flag.Parse()

	sleepSpan, err := time.ParseDuration(*sleep)
	if err != nil {
		fmt.Printf("fail to parse %s error %v\n", *sleep, err)
		fmt.Printf("use default sleep=1m\n")

		sleepSpan = 1 * time.Minute
	}

	var getFn func(url string) (resp *http.Response, err error)

	if *retry {
		log.Printf("retry(true) mode\n")

		client := &retryablehttp.Client{
			HTTPClient:   cleanhttp.DefaultPooledClient(),
			RetryWaitMin: 1 * time.Second,
			RetryWaitMax: 30 * time.Second,
			RetryMax:     3,
			CheckRetry:   retryablehttp.DefaultRetryPolicy,
			Backoff:      retryablehttp.DefaultBackoff,
		}

		getFn = client.Get
	} else {
		log.Printf("retry(false) mode\n")

		client := &http.Client{Transport: &http.Transport{
			MaxIdleConnsPerHost: 1024,
		}}

		getFn = client.Get
	}

	for {
		response, err := getFn(*server)
		if err != nil {
			log.Printf("fail to get  %v\n", err)
		} else {
			func() {
				defer response.Body.Close()
				dumpResponse, _ := httputil.DumpResponse(response, true)
				log.Printf("dumpResponse %s", string(dumpResponse))
			}()
		}

		log.Printf("start sleep %v\n", sleepSpan)
		time.Sleep(sleepSpan)
	}
}
