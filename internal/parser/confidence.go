package parser

// Scoring weights per CLAUDE.md spec.
const (
	weightIngredientRatio = 0.4
	weightParseSuccess    = 0.3
	weightOCRConfidence   = 0.2
	weightCountPlausible  = 0.1
)

// ScoreConfidence computes a 0.0–1.0 confidence score for OCR extraction quality.
func ScoreConfidence(result OCRParseResult, parsedCount int, ocrCharConfidence float64) float64 {
	if result.TotalLines == 0 {
		return 0
	}

	ingredientCount := len(result.IngredientLines)

	// Factor 1: Ingredient line ratio (0.0–1.0)
	ingredientRatio := float64(ingredientCount) / float64(result.TotalLines)

	// Factor 2: Parse success rate (0.0–1.0)
	var parseSuccess float64
	if ingredientCount > 0 {
		parseSuccess = float64(parsedCount) / float64(ingredientCount)
	}

	// Factor 3: OCR character confidence (already 0.0–1.0)
	ocrConf := clamp(ocrCharConfidence, 0, 1)

	// Factor 4: Ingredient count plausibility (0.0–1.0)
	countPlausible := plausibilityScore(ingredientCount)

	score := weightIngredientRatio*ingredientRatio +
		weightParseSuccess*parseSuccess +
		weightOCRConfidence*ocrConf +
		weightCountPlausible*countPlausible

	return clamp(score, 0, 1)
}

// plausibilityScore returns 1.0 for 5–25 ingredients, with linear falloff outside that range.
func plausibilityScore(count int) float64 {
	if count >= 5 && count <= 25 {
		return 1.0
	}
	if count < 5 {
		if count == 0 {
			return 0
		}
		return float64(count) / 5.0
	}
	// count > 25: penalize gradually, bottoming at 0 around 50
	if count >= 50 {
		return 0
	}
	return 1.0 - float64(count-25)/25.0
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
