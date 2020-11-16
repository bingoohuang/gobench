package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
)

func main() {
	addr := flag.String("addr", ":8812", "listen address")
	flag.Parse()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		dump, _ := httputil.DumpRequest(r, true)
		bytes, _ := json.Marshal(struct {
			Path   string
			Method string
			Addr   string
			Buddha string
			Detail string
		}{
			Path:   r.URL.Path,
			Method: r.Method,
			Addr:   *addr,
			Buddha: "起开，表烦我。思考人生，没空理你。未生我时谁是我，生我之时我是谁？",
			Detail: string(dump),
		})
		w.Write(bytes)
	})

	http.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		bytes, _ := json.Marshal(struct {
			Status int
		}{
			Status: 200,
		})
		w.Write(bytes)
	})

	http.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("OK\n"))
	})

	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "my own error message", http.StatusInternalServerError)
	})

	log.Println("start go rest server on", *addr)

	log.Fatal(http.ListenAndServe(*addr, nil))
}
