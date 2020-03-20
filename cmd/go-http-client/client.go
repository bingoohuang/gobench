// nolint gomnd
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
	keepalive := flag.Bool("keepalive", true, "keep alive or not")

	flag.Parse()

	sleepSpan, err := time.ParseDuration(*sleep)
	if err != nil {
		fmt.Printf("fail to parse %s error %v\n", *sleep, err)

		sleepSpan = 1 * time.Minute
	}

	var getFn func(url string) (resp *http.Response, err error)

	log.Printf("server %v\n", *server)
	log.Printf("sleep %v\n", sleepSpan)
	log.Printf("keepalive %v\n", *keepalive)
	log.Printf("retry mode %v\n", *retry)

	if *retry {
		transport := cleanhttp.DefaultPooledTransport()
		transport.DisableKeepAlives = !(*keepalive)
		transport.MaxIdleConnsPerHost = -1

		client := &retryablehttp.Client{
			HTTPClient:   &http.Client{Transport: transport},
			RetryWaitMin: 1 * time.Second,
			RetryWaitMax: 30 * time.Second,
			RetryMax:     3,
			CheckRetry:   retryablehttp.DefaultRetryPolicy,
			Backoff:      retryablehttp.DefaultBackoff,
		}

		getFn = client.Get
	} else {
		client := &http.Client{Transport: &http.Transport{
			MaxIdleConnsPerHost: 1024,
			DisableKeepAlives:   !*keepalive,
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
