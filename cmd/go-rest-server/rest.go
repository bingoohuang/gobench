package main

import (
	"flag"
	"log"
	"net/http"
)

func main() {
	addr := flag.String("addr", ":8812", "listen address")
	flag.Parse()

	http.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("OK"))
	})

	http.HandleFunc("/error", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "my own error message", http.StatusInternalServerError)
	})

	log.Println("start go rest server on", *addr)

	log.Fatal(http.ListenAndServe(*addr, nil))
}
