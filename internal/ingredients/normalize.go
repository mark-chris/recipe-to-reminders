package ingredients

import (
	"regexp"
	"strings"

	"recipe-to-reminders/internal/models"
)

var unicodeFractions = map[rune]string{
	'½': "1/2",
	'¼': "1/4",
	'¾': "3/4",
	'⅓': "1/3",
	'⅔': "2/3",
	'⅛': "1/8",
}

var unitNormMap = map[string]string{
	"tablespoon": "tbsp", "tablespoons": "tbsp", "tbsp": "tbsp", "tbs": "tbsp",
	"teaspoon": "tsp", "teaspoons": "tsp", "tsp": "tsp",
	"cup": "cups", "cups": "cups",
	"ounce": "oz", "ounces": "oz", "oz": "oz",
	"pound": "lbs", "pounds": "lbs", "lb": "lbs", "lbs": "lbs",
	"gram": "g", "grams": "g", "g": "g",
	"kilogram": "kg", "kilograms": "kg", "kg": "kg",
	"milliliter": "ml", "milliliters": "ml", "ml": "ml",
	"liter": "L", "liters": "L", "l": "L",
	"pinch": "pinch", "dash": "dash",
	"bunch": "bunch", "bunches": "bunch",
	"clove": "cloves", "cloves": "cloves",
	"can": "cans", "cans": "cans",
	"package": "pkg", "pkg": "pkg",
	"stick": "stick", "sticks": "stick",
	"head": "head", "heads": "head",
	"large": "large", "medium": "medium", "small": "small",
}

// Matches: quantity (digits, fractions, ranges), then unit, then name.
var mainPattern = regexp.MustCompile(
	`(?i)^([\d]+[\d/.\-–—]*(?:\s*[\d/]+)?)\s+` +
		`(tablespoons?|tbsp?|teaspoons?|tsp|cups?|ounces?|oz|pounds?|lbs?|lb|` +
		`grams?|g|kilograms?|kg|milliliters?|ml|liters?|l|` +
		`bunch(?:es)?|cloves?|cans?|pkg|packages?|sticks?|heads?|` +
		`large|medium|small|pinch|dash)\s+(.+)$`)

// Matches: quantity then name (no recognized unit).
var qtyNamePattern = regexp.MustCompile(
	`(?i)^([\d]+[\d/.\-–—]*(?:\s*[\d/]+)?)\s+(.+)$`)

var prepSuffixes = regexp.MustCompile(
	`(?i),\s+.*|` +
		`\s*\(.*?\)\s*|` +
		`\s+(?:finely |thinly |roughly )?(?:chopped|diced|minced|sliced|grated|` +
		`crushed|julienned|melted|softened|divided|optional|` +
		`peeled|trimmed|seeded|cored|at room temperature|to taste|` +
		`cut into .+)$`)

var toTastePattern = regexp.MustCompile(`(?i)^(.+?)[\s,]+to\s+taste$`)

// ParseRawIngredient converts a raw ingredient string into a structured Ingredient.
func ParseRawIngredient(raw string) models.Ingredient {
	s := replaceUnicodeFractions(strings.TrimSpace(raw))

	// Try "salt and pepper to taste" pattern.
	if m := toTastePattern.FindStringSubmatch(s); m != nil {
		return models.Ingredient{
			Raw:  raw,
			Name: strings.ToLower(strings.TrimSpace(m[1])),
		}
	}

	// Try main pattern: qty + unit + name.
	if m := mainPattern.FindStringSubmatch(s); m != nil {
		return models.Ingredient{
			Raw:      raw,
			Quantity: strings.TrimSpace(m[1]),
			Unit:     NormalizeUnit(strings.TrimSpace(m[2])),
			Name:     cleanName(m[3]),
		}
	}

	// Try qty + name (no recognized unit).
	if m := qtyNamePattern.FindStringSubmatch(s); m != nil {
		return models.Ingredient{
			Raw:      raw,
			Quantity: strings.TrimSpace(m[1]),
			Name:     cleanName(m[2]),
		}
	}

	// Fallback: treat entire string as name.
	return models.Ingredient{
		Raw:  raw,
		Name: strings.ToLower(strings.TrimSpace(s)),
	}
}

// NormalizeUnit maps unit variations to a standard form.
func NormalizeUnit(unit string) string {
	if normalized, ok := unitNormMap[strings.ToLower(unit)]; ok {
		return normalized
	}
	return strings.ToLower(unit)
}

func replaceUnicodeFractions(s string) string {
	for char, replacement := range unicodeFractions {
		s = strings.ReplaceAll(s, string(char), replacement)
	}
	// Clean up "11/2" → "1 1/2" (digit directly before fraction replacement).
	re := regexp.MustCompile(`(\d)(1/[2-8]|2/3|3/4)`)
	s = re.ReplaceAllString(s, "$1 $2")
	return s
}

func cleanName(s string) string {
	name := prepSuffixes.ReplaceAllString(strings.TrimSpace(s), "")
	name = strings.TrimSpace(name)
	return strings.ToLower(name)
}
