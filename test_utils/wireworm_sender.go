package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run wireworm_sender.go <file_to_send>")
		os.Exit(2)
	}
	fileName := os.Args[1]

	if info, err := os.Stat(fileName); err != nil {
		log.Fatalf("File error: %v", err)
	} else if info.IsDir() {
		log.Fatalf("File error: %q is a directory", fileName)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/download", func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("Receiver connected (%s)! Sending %s...\n", r.RemoteAddr, fileName)
		http.ServeFile(w, r, fileName)
	})

	srv := &http.Server{
		Addr:        "127.0.0.1:8080",
		Handler:     mux,
		ReadTimeout: 30 * time.Second,
		// WriteTimeout intentionally 0: large file transfers can take
		// arbitrarily long over a hole-punched WireGuard link, and a
		// hard cap would kill mid-transfer.
		IdleTimeout: 60 * time.Second,
	}

	fmt.Println("File server internal listening on 127.0.0.1:8080")
	fmt.Println("Endpoint: http://10.0.0.1:9000/download (via WireGuard)")
	log.Fatal(srv.ListenAndServe())
}
