package test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"recipe-to-reminders/internal/handler"
	"recipe-to-reminders/internal/models"
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
	html := loadFixture(t, "html_only.html")

	e := parser.NewExtractor(nil)
	result, err := e.ParseHTML(html, "https://example.com/pasta")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Method != "html" {
		t.Errorf("method = %q, want %q", result.Method, "html")
	}
	if len(result.Ingredients) == 0 {
		t.Fatal("expected ingredients from HTML fallback")
	}
}

func TestHTMLFallbackParser_FindsIngredientList(t *testing.T) {
	html := loadFixture(t, "html_only.html")
	p := parser.HTMLFallbackParser{}
	result, err := p.Parse(html, "https://example.com/pasta")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Title == "" {
		t.Error("expected a title")
	}
	if len(result.Ingredients) < 4 {
		t.Fatalf("got %d ingredients, want at least 4", len(result.Ingredients))
	}
	if result.Method != "html" {
		t.Errorf("method = %q, want %q", result.Method, "html")
	}
	if result.Confidence < 0.7 || result.Confidence > 1.0 {
		t.Errorf("confidence = %f, want 0.7-1.0", result.Confidence)
	}
}

func TestHTMLFallbackParser_NoRecipe(t *testing.T) {
	html := loadFixture(t, "no_recipe.html")
	p := parser.HTMLFallbackParser{}
	_, err := p.Parse(html, "https://example.com/about")
	if err == nil {
		t.Fatal("expected error for page with no recipe")
	}
}

func TestExtractor_NoRecipeReturnsError(t *testing.T) {
	html := loadFixture(t, "no_recipe.html")

	e := parser.NewExtractor(nil)
	_, err := e.ParseHTML(html, "https://example.com/about")
	if err == nil {
		t.Fatal("expected error for page with no recipe")
	}
}

func TestExtractHandler_ValidURL(t *testing.T) {
	// Set up a test recipe server
	recipeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write(loadFixture(t, "jsonld_simple.html"))
	}))
	defer recipeServer.Close()

	h := handler.New(parser.NewFetcher(parser.WithAllowLoopback(true)))

	body, _ := json.Marshal(models.ExtractRequest{URL: recipeServer.URL})
	req := httptest.NewRequest(http.MethodPost, "/extract", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. body: %s", rr.Code, rr.Body.String())
	}

	var resp models.ExtractResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Title != "Classic Beef Stew" {
		t.Errorf("title = %q, want %q", resp.Title, "Classic Beef Stew")
	}
	if len(resp.Ingredients) == 0 {
		t.Fatal("expected ingredients")
	}
	if resp.Method != "jsonld" {
		t.Errorf("method = %q, want %q", resp.Method, "jsonld")
	}
}

func TestExtractHandler_MissingURL(t *testing.T) {
	h := handler.New(nil)

	body, _ := json.Marshal(models.ExtractRequest{})
	req := httptest.NewRequest(http.MethodPost, "/extract", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestExtractHandler_InvalidImageBase64(t *testing.T) {
	h := handler.New(nil)

	body, _ := json.Marshal(models.ExtractRequest{ImageBase64: "abc123"})
	req := httptest.NewRequest(http.MethodPost, "/extract", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}

	var errResp models.ErrorResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &errResp)
	if errResp.Error != "invalid_image" {
		t.Errorf("error = %q, want %q", errResp.Error, "invalid_image")
	}
}

func TestExtractHandler_WrongMethod(t *testing.T) {
	h := handler.New(nil)
	req := httptest.NewRequest(http.MethodGet, "/extract", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rr.Code)
	}
}

func TestExtractHandler_ImageBase64(t *testing.T) {
	mock := &MockOCREngine{
		Text:       "2 cups flour\n1 tsp salt\n1 cup sugar\n3 large eggs\n1 cup milk\n",
		Confidence: 0.90,
	}
	h := handler.New(nil,
		parser.WithOCREngine(mock),
		parser.WithConfidenceThreshold(0.5),
	)

	img := createTestPNG(t, 100, 100)
	b64 := base64.StdEncoding.EncodeToString(img)
	body, _ := json.Marshal(models.ExtractRequest{ImageBase64: b64})
	req := httptest.NewRequest(http.MethodPost, "/extract", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. body: %s", rr.Code, rr.Body.String())
	}

	var resp models.ExtractResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Method != "tesseract" {
		t.Errorf("method = %q, want tesseract", resp.Method)
	}
	if len(resp.Ingredients) == 0 {
		t.Fatal("expected ingredients")
	}
}

func TestExtractHandler_ImageTooLarge(t *testing.T) {
	h := handler.New(nil)

	large := base64.StdEncoding.EncodeToString(make([]byte, 8*1024*1024))
	body, _ := json.Marshal(models.ExtractRequest{ImageBase64: large})
	req := httptest.NewRequest(http.MethodPost, "/extract", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestExtractHandler_BothURLAndImage(t *testing.T) {
	h := handler.New(nil)

	body, _ := json.Marshal(models.ExtractRequest{URL: "https://example.com", ImageBase64: "abc"})
	req := httptest.NewRequest(http.MethodPost, "/extract", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}
