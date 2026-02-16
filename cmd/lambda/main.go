package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"recipe-to-reminders/internal/handler"
	"recipe-to-reminders/internal/parser"
)

func main() {
	fetcher := parser.NewFetcher()
	h := handler.New(fetcher)

	addr := ":8080"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("Listening on %s", addr) // #nosec G706 -- addr is from PORT env var, not user input
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
