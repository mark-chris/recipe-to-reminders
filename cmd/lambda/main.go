package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"recipe-to-reminders/internal/handler"
	"recipe-to-reminders/internal/parser"
	"recipe-to-reminders/internal/storage"
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
			log.Fatalf("invalid CONFIDENCE_THRESHOLD %q: must be 0.0-1.0", thresh) // #nosec G706 -- %q quotes the value
		}
		opts = append(opts, parser.WithConfidenceThreshold(v))
	}

	// Claude Vision fallback
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		maxPerMin := 10
		if v := os.Getenv("CLAUDE_FALLBACK_MAX_PER_MIN"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				log.Fatalf("invalid CLAUDE_FALLBACK_MAX_PER_MIN %q", v) // #nosec G706 -- %q quotes the value
			}
			maxPerMin = n
		}
		claude := parser.NewClaudeExtractor(apiKey, maxPerMin)
		opts = append(opts, parser.WithImageExtractor(claude))
	}

	// S3 recipe storage
	var handlerOpts []handler.HandlerOption
	if bucket := os.Getenv("S3_BUCKET"); bucket != "" {
		recipesKey := os.Getenv("S3_RECIPES_KEY")
		if recipesKey == "" {
			recipesKey = "recipes.json"
		}

		cfg, err := config.LoadDefaultConfig(context.Background())
		if err != nil {
			log.Fatalf("failed to load AWS config: %v", err)
		}
		s3Client := storage.NewS3Client(s3.NewFromConfig(cfg))
		store := storage.NewS3Store(s3Client, bucket, recipesKey)
		handlerOpts = append(handlerOpts, handler.WithStore(store))
		log.Printf("Recipe storage: s3://%s/%s", bucket, recipesKey)
	}

	h := handler.New(fetcher, handlerOpts, opts...)

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
