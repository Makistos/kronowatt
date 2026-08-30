package main

import (
	"log"
	"net/http"
	"os"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		runMigrate(os.Args[2:])
		return
	}
	runServer()
}

func runServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	addr := ":8080"
	log.Printf("kronowatt server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
