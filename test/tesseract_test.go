package test

import (
	"context"
	"testing"

	"recipe-to-reminders/internal/parser"
)

func TestNewTesseractEngine_DefaultParams(t *testing.T) {
	engine, err := parser.NewTesseractEngine("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if engine == nil {
		t.Fatal("expected non-nil engine")
	}
}

func TestNewTesseractEngine_ValidPSM(t *testing.T) {
	for _, psm := range []string{"0", "3", "4", "6", "13"} {
		_, err := parser.NewTesseractEngine(psm, "eng")
		if err != nil {
			t.Errorf("PSM %s should be valid, got: %v", psm, err)
		}
	}
}

func TestNewTesseractEngine_InvalidPSM(t *testing.T) {
	invalid := []string{"-1", "14", "abc", "6; rm -rf /"}
	for _, psm := range invalid {
		_, err := parser.NewTesseractEngine(psm, "eng")
		if err == nil {
			t.Errorf("PSM %q should be rejected", psm)
		}
	}
}

func TestNewTesseractEngine_ValidLang(t *testing.T) {
	for _, lang := range []string{"eng", "fra", "spa"} {
		_, err := parser.NewTesseractEngine("6", lang)
		if err != nil {
			t.Errorf("lang %s should be valid, got: %v", lang, err)
		}
	}
}

func TestNewTesseractEngine_InvalidLang(t *testing.T) {
	invalid := []string{"en", "english", "eng;", "../etc", "ENG"}
	for _, lang := range invalid {
		_, err := parser.NewTesseractEngine("6", lang)
		if err == nil {
			t.Errorf("lang %q should be rejected", lang)
		}
	}
}

// MockOCREngine for use by other tests
type MockOCREngine struct {
	Text       string
	Confidence float64
	Err        error
}

func (m *MockOCREngine) Run(_ context.Context, _ []byte) (string, float64, error) {
	return m.Text, m.Confidence, m.Err
}
