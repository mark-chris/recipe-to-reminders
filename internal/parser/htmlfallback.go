package parser

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"recipe-to-reminders/internal/models"
)

// HTMLFallbackParser extracts ingredients by scanning HTML structure heuristically.
type HTMLFallbackParser struct{}

// Parse attempts to find an ingredient list in the HTML using common patterns.
func (p HTMLFallbackParser) Parse(html []byte, sourceURL string) (*models.RawRecipe, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(html))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML")
	}

	var ingredientStrings []string
	var confidence float64

	// Strategy 1: Look for containers with "ingredient" in class/id
	selectors := []string{
		`[class*="ingredient"] li`,
		`[class*="ingredient"] ul li`,
		`[id*="ingredient"] li`,
		`[class*="Ingredient"] li`,
	}
	for _, sel := range selectors {
		items := doc.Find(sel)
		if items.Length() >= 2 {
			items.Each(func(_ int, s *goquery.Selection) {
				text := strings.TrimSpace(s.Text())
				if text != "" {
					ingredientStrings = append(ingredientStrings, text)
				}
			})
			confidence = 0.85
			break
		}
	}

	// Strategy 2: Look for itemprop="recipeIngredient" (Microdata)
	if len(ingredientStrings) == 0 {
		doc.Find(`[itemprop="recipeIngredient"], [itemprop="ingredients"]`).Each(func(_ int, s *goquery.Selection) {
			text := strings.TrimSpace(s.Text())
			if text != "" {
				ingredientStrings = append(ingredientStrings, text)
			}
		})
		if len(ingredientStrings) >= 2 {
			confidence = 0.9
		}
	}

	// Strategy 3: Look for a heading containing "Ingredient" followed by a list
	if len(ingredientStrings) == 0 {
		doc.Find("h1, h2, h3, h4").Each(func(_ int, heading *goquery.Selection) {
			if len(ingredientStrings) > 0 {
				return
			}
			text := strings.ToLower(strings.TrimSpace(heading.Text()))
			if strings.Contains(text, "ingredient") {
				// Look for the next <ul> or <ol> sibling
				for next := heading.Next(); next.Length() > 0; next = next.Next() {
					tag := goquery.NodeName(next)
					if tag == "ul" || tag == "ol" {
						next.Find("li").Each(func(_ int, li *goquery.Selection) {
							t := strings.TrimSpace(li.Text())
							if t != "" {
								ingredientStrings = append(ingredientStrings, t)
							}
						})
						confidence = 0.8
						break
					}
					// Stop if we hit another heading or non-list element
					if tag == "h1" || tag == "h2" || tag == "h3" || tag == "h4" || tag == "div" {
						break
					}
				}
			}
		})
	}

	if len(ingredientStrings) == 0 {
		return nil, fmt.Errorf("no ingredients found in HTML")
	}

	title := extractTitle(doc)

	return &models.RawRecipe{
		Title:       title,
		Source:      extractHost(sourceURL),
		Method:      "html",
		Confidence:  confidence,
		Ingredients: ingredientStrings,
	}, nil
}

func extractTitle(doc *goquery.Document) string {
	// Try <h1> first
	if h1 := doc.Find("h1").First(); h1.Length() > 0 {
		return strings.TrimSpace(h1.Text())
	}
	// Fall back to <title>
	if title := doc.Find("title").First(); title.Length() > 0 {
		return strings.TrimSpace(title.Text())
	}
	return ""
}
