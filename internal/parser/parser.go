package parser

import (
	"context"
	"fmt"

	"recipe-to-reminders/internal/ingredients"
	"recipe-to-reminders/internal/models"
)

// HTMLParser can extract a RawRecipe from HTML bytes.
type HTMLParser interface {
	Parse(html []byte, sourceURL string) (*models.RawRecipe, error)
}

// ExtractorOption configures an Extractor.
type ExtractorOption func(*Extractor)

// WithOCREngine sets the OCR engine for photo extraction.
func WithOCREngine(engine OCREngine) ExtractorOption {
	return func(e *Extractor) { e.ocrEngine = engine }
}

// WithImageExtractor sets the Claude Vision fallback.
func WithImageExtractor(ext ImageExtractor) ExtractorOption {
	return func(e *Extractor) { e.imageExtractor = ext }
}

// WithConfidenceThreshold sets the minimum confidence to accept Tesseract results.
func WithConfidenceThreshold(t float64) ExtractorOption {
	return func(e *Extractor) { e.confidenceThreshold = t }
}

// WithPhotoStrategy sets the photo extraction strategy: "tesseract-first", "claude-only", "tesseract-only".
func WithPhotoStrategy(s string) ExtractorOption {
	return func(e *Extractor) { e.photoStrategy = s }
}

// Extractor coordinates fetching and parsing a recipe URL or image.
type Extractor struct {
	fetcher             *Fetcher
	jsonld              HTMLParser
	htmlFallback        HTMLParser
	ocrEngine           OCREngine
	imageExtractor      ImageExtractor
	confidenceThreshold float64
	photoStrategy       string
}

// NewExtractor creates an Extractor. Pass nil for fetcher if only using ParseHTML directly.
func NewExtractor(fetcher *Fetcher, opts ...ExtractorOption) *Extractor {
	e := &Extractor{
		fetcher:             fetcher,
		jsonld:              JSONLDParser{},
		htmlFallback:        HTMLFallbackParser{},
		confidenceThreshold: 0.65,
		photoStrategy:       "tesseract-first",
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Extract fetches a URL and extracts recipe ingredients.
func (e *Extractor) Extract(ctx context.Context, rawURL string) (*models.ExtractResponse, error) {
	if e.fetcher == nil {
		return nil, fmt.Errorf("no fetcher configured")
	}
	body, err := e.fetcher.Fetch(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	return e.ParseHTML(body, rawURL)
}

// ParseHTML extracts recipe data from raw HTML bytes. Tries JSON-LD first, then HTML fallback.
func (e *Extractor) ParseHTML(html []byte, sourceURL string) (*models.ExtractResponse, error) {
	raw, err := e.jsonld.Parse(html, sourceURL)
	if err == nil && len(raw.Ingredients) > 0 {
		return e.processRaw(raw), nil
	}

	if e.htmlFallback != nil {
		raw, err = e.htmlFallback.Parse(html, sourceURL)
		if err == nil && len(raw.Ingredients) > 0 {
			return e.processRaw(raw), nil
		}
	}

	return nil, fmt.Errorf("no recipe found")
}

// ExtractImage processes a decoded image and extracts recipe ingredients.
func (e *Extractor) ExtractImage(ctx context.Context, imageBytes []byte) (*models.ExtractResponse, error) {
	// Claude-only strategy: skip Tesseract entirely
	if e.photoStrategy == "claude-only" {
		return e.extractViaClaude(ctx, imageBytes)
	}

	// Tesseract path
	if e.ocrEngine == nil {
		return nil, fmt.Errorf("no OCR engine configured")
	}

	// Preprocess for OCR
	prepared, err := PrepareForOCR(imageBytes, 3000)
	if err != nil {
		return nil, fmt.Errorf("image preprocessing failed: %v", err)
	}

	ocrText, charConf, err := e.ocrEngine.Run(ctx, prepared)
	if err != nil {
		return nil, fmt.Errorf("OCR failed: %v", err)
	}

	// Parse OCR text into ingredient lines
	parseResult := ParseOCRText(ocrText)

	// Count successfully parsed ingredients (all 3 fields: qty, unit, name)
	parsedCount := 0
	for _, line := range parseResult.IngredientLines {
		ing := ingredients.ParseRawIngredient(line)
		if ing.Quantity != "" && ing.Unit != "" && ing.Name != "" {
			parsedCount++
		}
	}

	confidence := ScoreConfidence(parseResult, parsedCount, charConf)

	// Check if we should fall back to Claude
	if confidence < e.confidenceThreshold && e.photoStrategy != "tesseract-only" && e.imageExtractor != nil {
		claudeResult, err := e.extractViaClaude(ctx, imageBytes)
		if err == nil {
			return claudeResult, nil
		}
		// Claude failed — fall through to Tesseract results
	}

	// Build response from Tesseract results
	raw := &models.RawRecipe{
		Method:      "tesseract",
		Confidence:  confidence,
		Ingredients: parseResult.IngredientLines,
	}
	return e.processRaw(raw), nil
}

func (e *Extractor) extractViaClaude(ctx context.Context, imageBytes []byte) (*models.ExtractResponse, error) {
	if e.imageExtractor == nil {
		return nil, fmt.Errorf("no image extractor configured")
	}
	resp, err := e.imageExtractor.Extract(ctx, imageBytes)
	if err != nil {
		return nil, err
	}
	raw := ClaudeResponseToRaw(resp)
	return e.processRaw(raw), nil
}

func (e *Extractor) processRaw(raw *models.RawRecipe) *models.ExtractResponse {
	var parsed []models.Ingredient
	for _, s := range raw.Ingredients {
		ing := ingredients.ParseRawIngredient(s)
		ing.Category = ingredients.Categorize(ing.Name)
		parsed = append(parsed, ing)
	}
	parsed = ingredients.Deduplicate(parsed)

	return &models.ExtractResponse{
		Title:       raw.Title,
		Source:      raw.Source,
		Servings:    raw.Servings,
		Method:      raw.Method,
		Confidence:  raw.Confidence,
		Ingredients: parsed,
	}
}
