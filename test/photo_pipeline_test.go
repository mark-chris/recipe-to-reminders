package test

import (
	"context"
	"testing"

	"recipe-to-reminders/internal/parser"
)

func TestExtractImage_TesseractPathAboveThreshold(t *testing.T) {
	ocrText := "2 cups flour\n1 tsp salt\n1/2 cup sugar\n3 large eggs\n1 cup milk\n2 tbsp butter\n"
	mock := &MockOCREngine{Text: ocrText, Confidence: 0.90}

	ext := parser.NewExtractor(nil,
		parser.WithOCREngine(mock),
		parser.WithConfidenceThreshold(0.5),
	)

	img := createTestPNG(t, 200, 200)
	result, err := ext.ExtractImage(context.Background(), img)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Method != "tesseract" {
		t.Errorf("method = %q, want tesseract", result.Method)
	}
	if len(result.Ingredients) == 0 {
		t.Fatal("expected ingredients")
	}
	if result.Confidence < 0.5 {
		t.Errorf("confidence = %.2f, want >= 0.5", result.Confidence)
	}
}

func TestExtractImage_FallsBackToClaude(t *testing.T) {
	mock := &MockOCREngine{Text: "asdf jkl\nxyz 123\n", Confidence: 0.20}
	claudeMock := &MockImageExtractor{
		Result: &parser.ClaudeResponse{
			Title: "Beef Stew",
			Ingredients: []parser.ClaudeIngredient{
				{Raw: "2 lbs beef chuck", Name: "beef chuck", Quantity: "2", Unit: "lbs"},
			},
		},
	}

	ext := parser.NewExtractor(nil,
		parser.WithOCREngine(mock),
		parser.WithImageExtractor(claudeMock),
		parser.WithConfidenceThreshold(0.65),
	)

	img := createTestPNG(t, 200, 200)
	result, err := ext.ExtractImage(context.Background(), img)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Method != "claude" {
		t.Errorf("method = %q, want claude", result.Method)
	}
	if result.Title != "Beef Stew" {
		t.Errorf("title = %q, want Beef Stew", result.Title)
	}
}

func TestExtractImage_NoFallbackReturnsLowConfidence(t *testing.T) {
	mock := &MockOCREngine{Text: "asdf jkl\n", Confidence: 0.20}

	ext := parser.NewExtractor(nil,
		parser.WithOCREngine(mock),
		parser.WithConfidenceThreshold(0.65),
	)

	img := createTestPNG(t, 200, 200)
	result, err := ext.ExtractImage(context.Background(), img)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Method != "tesseract" {
		t.Errorf("method = %q, want tesseract", result.Method)
	}
	if result.Confidence >= 0.65 {
		t.Errorf("confidence = %.2f, expected below threshold", result.Confidence)
	}
}

func TestExtractImage_ClaudeOnlyStrategy(t *testing.T) {
	claudeMock := &MockImageExtractor{
		Result: &parser.ClaudeResponse{
			Title: "Pancakes",
			Ingredients: []parser.ClaudeIngredient{
				{Raw: "2 cups flour", Name: "flour", Quantity: "2", Unit: "cups"},
			},
		},
	}

	ext := parser.NewExtractor(nil,
		parser.WithImageExtractor(claudeMock),
		parser.WithPhotoStrategy("claude-only"),
	)

	img := createTestPNG(t, 200, 200)
	result, err := ext.ExtractImage(context.Background(), img)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Method != "claude" {
		t.Errorf("method = %q, want claude", result.Method)
	}
}

func TestExtractImage_TesseractOnlyStrategy(t *testing.T) {
	mock := &MockOCREngine{Text: "asdf\n", Confidence: 0.10}

	ext := parser.NewExtractor(nil,
		parser.WithOCREngine(mock),
		parser.WithPhotoStrategy("tesseract-only"),
		parser.WithConfidenceThreshold(0.65),
	)

	img := createTestPNG(t, 200, 200)
	result, err := ext.ExtractImage(context.Background(), img)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Method != "tesseract" {
		t.Errorf("method = %q, want tesseract", result.Method)
	}
}
