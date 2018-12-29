package main

import "net/http"

func main() {
	http.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("OK"))
	})

	_ = http.ListenAndServe(":8812", nil)
}
