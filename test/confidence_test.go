package test

import (
	"math"
	"testing"

	"recipe-to-reminders/internal/parser"
)

func TestScoreConfidence_HighQuality(t *testing.T) {
	result := parser.OCRParseResult{
		IngredientLines: make([]string, 10),
		HeaderLines:     1,
		NoiseLines:      1,
		TotalLines:      12,
	}
	parsedCount := 10
	ocrConfidence := 0.92

	score := parser.ScoreConfidence(result, parsedCount, ocrConfidence)
	if score < 0.65 {
		t.Errorf("high quality OCR scored %.2f, want >= 0.65", score)
	}
}

func TestScoreConfidence_LowQuality(t *testing.T) {
	result := parser.OCRParseResult{
		IngredientLines: make([]string, 2),
		HeaderLines:     0,
		NoiseLines:      15,
		TotalLines:      17,
	}
	parsedCount := 0
	ocrConfidence := 0.35

	score := parser.ScoreConfidence(result, parsedCount, ocrConfidence)
	if score > 0.40 {
		t.Errorf("low quality OCR scored %.2f, want < 0.40", score)
	}
}

func TestScoreConfidence_EmptyResult(t *testing.T) {
	result := parser.OCRParseResult{}
	score := parser.ScoreConfidence(result, 0, 0)
	if score != 0 {
		t.Errorf("empty result scored %.2f, want 0", score)
	}
}

func TestScoreConfidence_PlausibleCount(t *testing.T) {
	// 12 ingredients is ideal range (5-25)
	result := parser.OCRParseResult{
		IngredientLines: make([]string, 12),
		TotalLines:      14,
	}
	score12 := parser.ScoreConfidence(result, 12, 0.9)

	// 1 ingredient is outside range
	result2 := parser.OCRParseResult{
		IngredientLines: make([]string, 1),
		TotalLines:      3,
	}
	score1 := parser.ScoreConfidence(result2, 1, 0.9)

	if score1 >= score12 {
		t.Errorf("1 ingredient (%.2f) should score lower than 12 (%.2f)", score1, score12)
	}
}

func TestScoreConfidence_RangeZeroToOne(t *testing.T) {
	cases := []struct {
		ingredients int
		total       int
		parsed      int
		ocrConf     float64
	}{
		{0, 0, 0, 0},
		{100, 100, 100, 1.0},
		{5, 200, 2, 0.1},
		{25, 25, 25, 1.0},
	}
	for _, c := range cases {
		result := parser.OCRParseResult{
			IngredientLines: make([]string, c.ingredients),
			TotalLines:      c.total,
		}
		score := parser.ScoreConfidence(result, c.parsed, c.ocrConf)
		if score < 0 || score > 1 || math.IsNaN(score) {
			t.Errorf("score %.2f out of range [0,1] for input %+v", score, c)
		}
	}
}
