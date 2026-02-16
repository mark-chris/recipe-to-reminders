package test

import (
	"strings"
	"testing"

	"recipe-to-reminders/internal/parser"
)

func TestParseOCRText_StandardIngredients(t *testing.T) {
	text := `Ingredients

2 cups all-purpose flour
1 tsp baking soda
3/4 cup sugar
2 large eggs
1 cup milk
`
	result := parser.ParseOCRText(text)
	if len(result.IngredientLines) != 5 {
		t.Fatalf("got %d ingredient lines, want 5", len(result.IngredientLines))
	}
	if result.IngredientLines[0] != "2 cups all-purpose flour" {
		t.Errorf("line[0] = %q", result.IngredientLines[0])
	}
}

func TestParseOCRText_WithSectionHeaders(t *testing.T) {
	text := `For the crust:
1 cup flour
1/2 cup butter

For the filling:
2 cups sugar
3 eggs
`
	result := parser.ParseOCRText(text)
	if len(result.IngredientLines) != 4 {
		t.Fatalf("got %d ingredient lines, want 4", len(result.IngredientLines))
	}
	if result.HeaderLines != 2 {
		t.Errorf("headers = %d, want 2", result.HeaderLines)
	}
}

func TestParseOCRText_FiltersNoise(t *testing.T) {
	text := `Classic Chocolate Cake
Serves 8 | Prep time: 30 min

Ingredients

2 cups flour
1 cup cocoa powder

Page 42
www.recipes.com
`
	result := parser.ParseOCRText(text)
	if len(result.IngredientLines) != 2 {
		t.Fatalf("got %d ingredient lines, want 2", len(result.IngredientLines))
	}
}

func TestParseOCRText_UnicodeFractions(t *testing.T) {
	text := `½ tsp salt
¾ cup butter
`
	result := parser.ParseOCRText(text)
	if len(result.IngredientLines) != 2 {
		t.Fatalf("got %d ingredient lines, want 2", len(result.IngredientLines))
	}
}

func TestParseOCRText_EmptyInput(t *testing.T) {
	result := parser.ParseOCRText("")
	if len(result.IngredientLines) != 0 {
		t.Errorf("got %d lines for empty input", len(result.IngredientLines))
	}
}

func TestParseOCRText_QuantityWords(t *testing.T) {
	text := `a pinch of salt
one 14-oz can diced tomatoes
`
	result := parser.ParseOCRText(text)
	if len(result.IngredientLines) != 2 {
		t.Fatalf("got %d ingredient lines, want 2", len(result.IngredientLines))
	}
}

func TestParseOCRText_CapsLineLength(t *testing.T) {
	// Lines longer than 500 chars should be truncated
	long := "2 cups "
	for len(long) < 600 {
		long += "x"
	}
	result := parser.ParseOCRText(long)
	if len(result.IngredientLines) > 0 && len(result.IngredientLines[0]) > 500 {
		t.Error("expected line to be capped at 500 chars")
	}
}

func TestParseOCRText_CapsLineCount(t *testing.T) {
	// More than 200 lines should be capped
	var b strings.Builder
	for range 250 {
		b.WriteString("1 cup flour\n")
	}
	text := b.String()
	result := parser.ParseOCRText(text)
	if result.TotalLines > 200 {
		t.Errorf("total lines = %d, want capped at 200", result.TotalLines)
	}
}
