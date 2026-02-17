package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"recipe-to-reminders/internal/handler"
	"recipe-to-reminders/internal/parser"
)

func main() {
	fetcher := parser.NewFetcher()

	var opts []parser.ExtractorOption

	// Tesseract OCR engine
	psm := os.Getenv("TESSERACT_PSM")
	lang := os.Getenv("TESSERACT_LANG")
	engine, err := parser.NewTesseractEngine(psm, lang)
	if err != nil {
		log.Fatalf("invalid tesseract config: %v", err)
	}
	opts = append(opts, parser.WithOCREngine(engine))

	// Photo strategy
	if strategy := os.Getenv("PHOTO_STRATEGY"); strategy != "" {
		opts = append(opts, parser.WithPhotoStrategy(strategy))
	}

	// Confidence threshold
	if thresh := os.Getenv("CONFIDENCE_THRESHOLD"); thresh != "" {
		v, err := strconv.ParseFloat(thresh, 64)
		if err != nil || v < 0 || v > 1 {
			log.Fatalf("invalid CONFIDENCE_THRESHOLD %q: must be 0.0-1.0", thresh)
		}
		opts = append(opts, parser.WithConfidenceThreshold(v))
	}

	// Claude Vision fallback
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		maxPerMin := 10
		if v := os.Getenv("CLAUDE_FALLBACK_MAX_PER_MIN"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				log.Fatalf("invalid CLAUDE_FALLBACK_MAX_PER_MIN %q", v)
			}
			maxPerMin = n
		}
		claude := parser.NewClaudeExtractor(apiKey, maxPerMin)
		opts = append(opts, parser.WithImageExtractor(claude))
	}

	h := handler.New(fetcher, opts...)

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
