package main

import (
	"log"
	"net/http"
	"os"

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

	log.Printf("Listening on %s", addr)
	if err := http.ListenAndServe(addr, h); err != nil {
		log.Fatal(err)
	}
}
