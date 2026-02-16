package parser

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"recipe-to-reminders/internal/models"
)

// JSONLDParser extracts recipe data from JSON-LD script tags.
type JSONLDParser struct{}

// Parse scans HTML for a JSON-LD Recipe object and extracts ingredient strings.
func (p JSONLDParser) Parse(html []byte, sourceURL string) (*models.RawRecipe, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML")
	}

	var recipe *models.RawRecipe
	doc.Find(`script[type="application/ld+json"]`).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		text := strings.TrimSpace(s.Text())
		if r := extractRecipeFromJSON([]byte(text)); r != nil {
			recipe = r
			return false // stop iterating
		}
		return true
	})

	if recipe == nil {
		return nil, fmt.Errorf("no JSON-LD Recipe found")
	}

	recipe.Source = extractHost(sourceURL)
	recipe.Method = "jsonld"
	recipe.Confidence = 1.0
	return recipe, nil
}

func extractRecipeFromJSON(data []byte) *models.RawRecipe {
	// Try as a single object first
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err == nil {
		if r := tryExtractRecipe(obj); r != nil {
			return r
		}
		// Check for @graph array
		if graph, ok := obj["@graph"]; ok {
			if items, ok := graph.([]any); ok {
				for _, item := range items {
					if m, ok := item.(map[string]any); ok {
						if r := tryExtractRecipe(m); r != nil {
							return r
						}
					}
				}
			}
		}
	}

	// Try as an array of objects
	var arr []map[string]any
	if err := json.Unmarshal(data, &arr); err == nil {
		for _, obj := range arr {
			if r := tryExtractRecipe(obj); r != nil {
				return r
			}
		}
	}

	return nil
}

func tryExtractRecipe(obj map[string]any) *models.RawRecipe {
	typ, _ := obj["@type"].(string)
	if !strings.EqualFold(typ, "Recipe") {
		// @type can also be an array: ["Recipe"]
		if types, ok := obj["@type"].([]any); ok {
			found := false
			for _, t := range types {
				if s, ok := t.(string); ok && strings.EqualFold(s, "Recipe") {
					found = true
					break
				}
			}
			if !found {
				return nil
			}
		} else if typ == "" {
			return nil
		}
	}

	ingredients := extractStringArray(obj["recipeIngredient"])
	if len(ingredients) == 0 {
		return nil
	}

	name, _ := obj["name"].(string)
	yield, _ := obj["recipeYield"].(string)
	if yield == "" {
		// recipeYield can also be an array
		if yields := extractStringArray(obj["recipeYield"]); len(yields) > 0 {
			yield = yields[0]
		}
	}

	return &models.RawRecipe{
		Title:       name,
		Servings:    yield,
		Ingredients: ingredients,
	}
}

func extractStringArray(v any) []string {
	switch val := v.(type) {
	case []any:
		var result []string
		for _, item := range val {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	case string:
		return []string{val}
	default:
		return nil
	}
}

func extractHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Host
}
