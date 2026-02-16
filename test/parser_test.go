package test

import (
	"os"
	"path/filepath"
	"testing"

	"recipe-to-reminders/internal/parser"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("fixtures", name))
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", name, err)
	}
	return data
}

func TestJSONLDParser_SimpleRecipe(t *testing.T) {
	html := loadFixture(t, "jsonld_simple.html")
	p := parser.JSONLDParser{}
	result, err := p.Parse(html, "https://example.com/beef-stew")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Title != "Classic Beef Stew" {
		t.Errorf("title = %q, want %q", result.Title, "Classic Beef Stew")
	}
	if result.Servings != "6 servings" {
		t.Errorf("servings = %q, want %q", result.Servings, "6 servings")
	}
	if len(result.Ingredients) != 6 {
		t.Fatalf("got %d ingredients, want 6", len(result.Ingredients))
	}
	if result.Ingredients[0] != "2 pounds beef chuck, cut into 1-inch cubes" {
		t.Errorf("ingredient[0] = %q", result.Ingredients[0])
	}
}

func TestJSONLDParser_GraphWrapper(t *testing.T) {
	html := loadFixture(t, "jsonld_graph.html")
	p := parser.JSONLDParser{}
	result, err := p.Parse(html, "https://example.com/pancakes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Title != "Buttermilk Pancakes" {
		t.Errorf("title = %q, want %q", result.Title, "Buttermilk Pancakes")
	}
	if len(result.Ingredients) != 7 {
		t.Fatalf("got %d ingredients, want 7", len(result.Ingredients))
	}
}

func TestJSONLDParser_NoRecipe(t *testing.T) {
	html := loadFixture(t, "no_recipe.html")
	p := parser.JSONLDParser{}
	_, err := p.Parse(html, "https://example.com/about")
	if err == nil {
		t.Fatal("expected error for page with no recipe")
	}
}

func TestJSONLDParser_HTMLOnly_ReturnsError(t *testing.T) {
	html := loadFixture(t, "html_only.html")
	p := parser.JSONLDParser{}
	_, err := p.Parse(html, "https://example.com/pasta")
	if err == nil {
		t.Fatal("expected error for page with no JSON-LD")
	}
}

func TestJSONLDParser_ExtractsSource(t *testing.T) {
	html := loadFixture(t, "jsonld_simple.html")
	p := parser.JSONLDParser{}
	result, err := p.Parse(html, "https://www.allrecipes.com/recipe/123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Source != "www.allrecipes.com" {
		t.Errorf("source = %q, want %q", result.Source, "www.allrecipes.com")
	}
}

func TestExtractor_JSONLDPath(t *testing.T) {
	html := loadFixture(t, "jsonld_simple.html")

	e := parser.NewExtractor(nil) // nil fetcher — we'll call ParseHTML separately
	result, err := e.ParseHTML(html, "https://example.com/recipe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Method != "jsonld" {
		t.Errorf("method = %q, want %q", result.Method, "jsonld")
	}
	if result.Title != "Classic Beef Stew" {
		t.Errorf("title = %q, want %q", result.Title, "Classic Beef Stew")
	}
	if len(result.Ingredients) != 6 {
		t.Fatalf("got %d ingredients, want 6", len(result.Ingredients))
	}
	// Check that ingredients are normalized
	first := result.Ingredients[0]
	if first.Name != "beef chuck" {
		t.Errorf("first ingredient name = %q, want %q", first.Name, "beef chuck")
	}
	if first.Quantity != "2" {
		t.Errorf("first ingredient qty = %q, want %q", first.Quantity, "2")
	}
	if first.Category == "" {
		t.Error("first ingredient should have a category")
	}
}

func TestExtractor_FallsBackToHTML(t *testing.T) {
	t.Skip("HTML fallback not yet implemented")
}

func TestExtractor_NoRecipeReturnsError(t *testing.T) {
	html := loadFixture(t, "no_recipe.html")

	e := parser.NewExtractor(nil)
	_, err := e.ParseHTML(html, "https://example.com/about")
	if err == nil {
		t.Fatal("expected error for page with no recipe")
	}
}
