package main

import (
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
)

func main() {
	addr := flag.String("addr", ":8812", "listen address")
	flag.Parse()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("起开，表烦我。思考人生，没空理你。未生我时谁是我，生我之时我是谁？"))
	})

	http.HandleFunc("/dump", func(w http.ResponseWriter, r *http.Request) {
		dump, _ := httputil.DumpRequest(r, true)
		_, _ = w.Write(dump)
	})

	http.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("OK\n"))
	})

	http.HandleFunc("/error", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "my own error message", http.StatusInternalServerError)
	})

	log.Println("start go rest server on", *addr)

	log.Fatal(http.ListenAndServe(*addr, nil))
}
