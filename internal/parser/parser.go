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

// Extractor coordinates fetching and parsing a recipe URL.
type Extractor struct {
	fetcher      *Fetcher
	jsonld       HTMLParser
	htmlFallback HTMLParser
}

// NewExtractor creates an Extractor. Pass nil for fetcher if only using ParseHTML directly.
func NewExtractor(fetcher *Fetcher) *Extractor {
	return &Extractor{
		fetcher:      fetcher,
		jsonld:       JSONLDParser{},
		htmlFallback: nil, // set after HTML fallback is implemented
	}
}

// SetHTMLFallback sets the HTML fallback parser. Called after it's implemented.
func (e *Extractor) SetHTMLFallback(p HTMLParser) {
	e.htmlFallback = p
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
	// Try JSON-LD
	raw, err := e.jsonld.Parse(html, sourceURL)
	if err == nil && len(raw.Ingredients) > 0 {
		return e.processRaw(raw), nil
	}

	// Try HTML fallback
	if e.htmlFallback != nil {
		raw, err = e.htmlFallback.Parse(html, sourceURL)
		if err == nil && len(raw.Ingredients) > 0 {
			return e.processRaw(raw), nil
		}
	}

	return nil, fmt.Errorf("no recipe found")
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
