package main

import "net/http"

func main() {
	http.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("OK"))
	})

	http.HandleFunc("/error", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "my own error message", http.StatusInternalServerError)
	})

	_ = http.ListenAndServe(":8812", nil)
}
