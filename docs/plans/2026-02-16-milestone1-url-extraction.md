# Milestone 1: URL Extraction — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** `POST /extract` with a recipe URL returns normalized, categorized ingredients as JSON.

**Architecture:** Vertical slice — JSON-LD parser (happy path) first, then ingredient processing, HTTP handler, and HTML fallback. Parsers are pure functions that take HTML bytes; a `Fetcher` handles URL validation/SSRF/HTTP. A dispatcher wires fetching → parsing → normalization → categorization → deduplication.

**Tech Stack:** Go 1.25+, goquery for HTML/DOM parsing, net/http for serving, httptest for test servers.

---

### Task 1: Project Scaffold + Models

**Files:**
- Create: `go.mod`
- Create: `internal/models/recipe.go`

**Step 1: Initialize Go module and install goquery**

Run:
```bash
cd /home/mark/Projects/recipe-to-reminders
go mod init recipe-to-reminders
go get github.com/PuerkitoBio/goquery@v1.11.0
```
Expected: `go.mod` and `go.sum` created.

**Step 2: Create directory structure**

Run:
```bash
mkdir -p cmd/lambda internal/handler internal/parser internal/ingredients internal/models test/fixtures
```

**Step 3: Write models**

Create `internal/models/recipe.go`:
```go
package models

// Ingredient is a single parsed ingredient with structured fields.
type Ingredient struct {
	Name     string `json:"name"`
	Quantity string `json:"quantity"`
	Unit     string `json:"unit"`
	Category string `json:"category"`
	Raw      string `json:"raw"`
}

// RawRecipe holds the output of an HTML parser before ingredient normalization.
type RawRecipe struct {
	Title       string
	Source      string
	Servings    string
	Method      string
	Confidence  float64
	Ingredients []string // raw ingredient strings as extracted from the page
}

// ExtractRequest is the JSON body for POST /extract.
type ExtractRequest struct {
	URL         string `json:"url"`
	ImageBase64 string `json:"image_base64"`
	ListName    string `json:"list_name"`
}

// ExtractResponse is the JSON body returned by POST /extract.
type ExtractResponse struct {
	Title       string       `json:"title"`
	Source      string       `json:"source"`
	Servings    string       `json:"servings"`
	Method      string       `json:"method"`
	Confidence  float64      `json:"confidence"`
	Ingredients []Ingredient `json:"ingredients"`
}

// ErrorResponse is returned on failure.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}
```

**Step 4: Verify it compiles**

Run: `go build ./...`
Expected: clean exit, no errors.

**Step 5: Commit**

```bash
git add go.mod go.sum internal/models/recipe.go
git commit -m "scaffold: Go module, directory structure, model types"
```

---

### Task 2: Test Fixtures

**Files:**
- Create: `test/fixtures/jsonld_simple.html`
- Create: `test/fixtures/jsonld_graph.html`
- Create: `test/fixtures/html_only.html`
- Create: `test/fixtures/no_recipe.html`

**Step 1: Create JSON-LD simple fixture**

Create `test/fixtures/jsonld_simple.html` — a minimal recipe page with a top-level JSON-LD Recipe object:
```html
<!DOCTYPE html>
<html>
<head>
  <title>Classic Beef Stew</title>
  <script type="application/ld+json">
  {
    "@context": "https://schema.org",
    "@type": "Recipe",
    "name": "Classic Beef Stew",
    "recipeYield": "6 servings",
    "recipeIngredient": [
      "2 pounds beef chuck, cut into 1-inch cubes",
      "3 large carrots, peeled and sliced",
      "1 cup all-purpose flour",
      "2 tablespoons olive oil",
      "1/2 teaspoon black pepper",
      "3 cloves garlic, minced"
    ]
  }
  </script>
</head>
<body><h1>Classic Beef Stew</h1></body>
</html>
```

**Step 2: Create JSON-LD @graph fixture**

Create `test/fixtures/jsonld_graph.html` — Recipe nested inside an `@graph` array (common on WordPress sites):
```html
<!DOCTYPE html>
<html>
<head>
  <title>Buttermilk Pancakes</title>
  <script type="application/ld+json">
  {
    "@context": "https://schema.org",
    "@graph": [
      {
        "@type": "WebPage",
        "name": "Buttermilk Pancakes"
      },
      {
        "@type": "Recipe",
        "name": "Buttermilk Pancakes",
        "recipeYield": "12 pancakes",
        "recipeIngredient": [
          "1½ cups all-purpose flour",
          "2 tablespoons sugar",
          "1 teaspoon baking powder",
          "½ teaspoon baking soda",
          "1 cup buttermilk",
          "1 large egg",
          "2 tablespoons unsalted butter, melted"
        ]
      }
    ]
  }
  </script>
</head>
<body><h1>Buttermilk Pancakes</h1></body>
</html>
```

**Step 3: Create HTML-only fixture**

Create `test/fixtures/html_only.html` — a page with no JSON-LD but ingredients in a recognizable HTML structure:
```html
<!DOCTYPE html>
<html>
<head><title>Simple Pasta - MyRecipes</title></head>
<body>
  <h1>Simple Pasta</h1>
  <div class="recipe-ingredients">
    <h2>Ingredients</h2>
    <ul>
      <li>1 pound spaghetti</li>
      <li>3 tablespoons olive oil</li>
      <li>4 cloves garlic, sliced</li>
      <li>1/4 teaspoon red pepper flakes</li>
      <li>1/2 cup grated Parmesan cheese</li>
      <li>Salt and pepper to taste</li>
    </ul>
  </div>
  <div class="recipe-instructions">
    <h2>Instructions</h2>
    <ol>
      <li>Cook spaghetti according to package directions.</li>
      <li>Heat olive oil in a large skillet.</li>
    </ol>
  </div>
</body>
</html>
```

**Step 4: Create no-recipe fixture**

Create `test/fixtures/no_recipe.html`:
```html
<!DOCTYPE html>
<html>
<head><title>About Us - Food Blog</title></head>
<body>
  <h1>About Us</h1>
  <p>We are a food blog dedicated to sharing recipes from around the world.</p>
  <p>Contact us at hello@foodblog.example.</p>
</body>
</html>
```

**Step 5: Commit**

```bash
git add test/fixtures/
git commit -m "test: add HTML fixture files for parser tests"
```

---

### Task 3: URL Validator

**Files:**
- Create: `internal/parser/urlvalidator.go`
- Create: `test/security_test.go`

This is the SSRF protection layer. We split it into pure validation functions (easy to test) and a `Fetcher` struct that handles the full HTTP pipeline.

**Step 1: Write the failing tests**

Create `test/security_test.go`:
```go
package test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"recipe-to-reminders/internal/parser"
)

func TestValidateURL_AcceptsHTTP(t *testing.T) {
	_, err := parser.ValidateURL("http://example.com/recipe")
	if err != nil {
		t.Fatalf("expected http to be accepted, got: %v", err)
	}
}

func TestValidateURL_AcceptsHTTPS(t *testing.T) {
	_, err := parser.ValidateURL("https://example.com/recipe")
	if err != nil {
		t.Fatalf("expected https to be accepted, got: %v", err)
	}
}

func TestValidateURL_RejectsFileScheme(t *testing.T) {
	_, err := parser.ValidateURL("file:///etc/passwd")
	if err == nil {
		t.Fatal("expected file:// to be rejected")
	}
}

func TestValidateURL_RejectsFTPScheme(t *testing.T) {
	_, err := parser.ValidateURL("ftp://example.com/recipe")
	if err == nil {
		t.Fatal("expected ftp:// to be rejected")
	}
}

func TestValidateURL_RejectsNoScheme(t *testing.T) {
	_, err := parser.ValidateURL("example.com/recipe")
	if err == nil {
		t.Fatal("expected missing scheme to be rejected")
	}
}

func TestValidateURL_RejectsEmptyHost(t *testing.T) {
	_, err := parser.ValidateURL("http://")
	if err == nil {
		t.Fatal("expected empty host to be rejected")
	}
}

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1",
		"10.0.0.1",
		"172.16.0.1",
		"192.168.1.1",
		"169.254.169.254",
		"0.0.0.0",
		"224.0.0.1",
		"::1",
		"fe80::1",
	}
	for _, ip := range blocked {
		t.Run(ip, func(t *testing.T) {
			if !parser.IsBlockedIP(net.ParseIP(ip)) {
				t.Errorf("expected %s to be blocked", ip)
			}
		})
	}

	allowed := []string{
		"93.184.216.34",
		"8.8.8.8",
		"2606:2800:220:1:248:1893:25c8:1946",
	}
	for _, ip := range allowed {
		t.Run(ip, func(t *testing.T) {
			if parser.IsBlockedIP(net.ParseIP(ip)) {
				t.Errorf("expected %s to be allowed", ip)
			}
		})
	}
}

func TestFetcher_LimitsResponseSize(t *testing.T) {
	// Server returns 10MB of data
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		for i := 0; i < 10*1024; i++ {
			fmt.Fprint(w, "x")
			for j := 0; j < 1024; j++ {
				fmt.Fprint(w, "x")
			}
		}
	}))
	defer ts.Close()

	f := parser.NewFetcher(parser.WithMaxBodySize(1024), parser.WithAllowLoopback(true))
	body, err := f.Fetch(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if len(body) > 1024 {
		t.Errorf("body size %d exceeds limit 1024", len(body))
	}
}

func TestFetcher_RespectsRedirectLimit(t *testing.T) {
	redirectCount := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCount++
		if redirectCount <= 5 {
			http.Redirect(w, r, fmt.Sprintf("/?attempt=%d", redirectCount), http.StatusFound)
			return
		}
		fmt.Fprint(w, "final")
	}))
	defer ts.Close()

	f := parser.NewFetcher(parser.WithAllowLoopback(true))
	_, err := f.Fetch(context.Background(), ts.URL)
	if err == nil {
		t.Fatal("expected redirect limit error")
	}
}

func TestFetcher_SuccessfulFetch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><body>Hello</body></html>")
	}))
	defer ts.Close()

	f := parser.NewFetcher(parser.WithAllowLoopback(true))
	body, err := f.Fetch(context.Background(), ts.URL)
	if err != nil {
		t.Fatalf("fetch failed: %v", err)
	}
	if string(body) != "<html><body>Hello</body></html>" {
		t.Errorf("unexpected body: %s", body)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./test/ -v -run "TestValidateURL|TestIsBlockedIP|TestFetcher" 2>&1 | head -20`
Expected: compilation error — `parser` package doesn't export these functions yet.

**Step 3: Implement the URL validator**

Create `internal/parser/urlvalidator.go`:
```go
package parser

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"
)

const (
	defaultMaxBodySize = 5 * 1024 * 1024 // 5MB
	maxRedirects       = 3
	connectTimeout     = 5 * time.Second
	tlsTimeout         = 5 * time.Second
	headerTimeout      = 10 * time.Second
	requestTimeout     = 15 * time.Second
)

var blockedNetworks []*net.IPNet

func init() {
	cidrs := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",
		"224.0.0.0/4",
		"0.0.0.0/8",
		"::1/128",
		"fe80::/10",
		"ff00::/8",
		"::/128",
	}
	for _, cidr := range cidrs {
		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(fmt.Sprintf("invalid CIDR %s: %v", cidr, err))
		}
		blockedNetworks = append(blockedNetworks, network)
	}
}

// ValidateURL checks that the URL has an allowed scheme and a non-empty host.
func ValidateURL(rawURL string) (*url.URL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("URL scheme must be http or https")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("URL must include a host")
	}
	return u, nil
}

// IsBlockedIP returns true if the IP falls within any blocked CIDR range.
func IsBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	for _, network := range blockedNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// Fetcher handles safe HTTP fetching with SSRF protections.
type Fetcher struct {
	maxBodySize    int64
	allowLoopback  bool
}

// FetcherOption configures a Fetcher.
type FetcherOption func(*Fetcher)

// WithMaxBodySize overrides the default response body size limit.
func WithMaxBodySize(n int64) FetcherOption {
	return func(f *Fetcher) { f.maxBodySize = n }
}

// WithAllowLoopback permits fetching from loopback addresses (for testing only).
func WithAllowLoopback(allow bool) FetcherOption {
	return func(f *Fetcher) { f.allowLoopback = allow }
}

// NewFetcher creates a Fetcher with the given options.
func NewFetcher(opts ...FetcherOption) *Fetcher {
	f := &Fetcher{
		maxBodySize: defaultMaxBodySize,
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// Fetch validates a URL, resolves DNS, checks IPs, follows redirects safely,
// and returns the response body (capped at maxBodySize).
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) ([]byte, error) {
	u, err := ValidateURL(rawURL)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	transport := &http.Transport{
		DialContext: func(dialCtx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch recipe from URL")
			}
			ips, err := net.DefaultResolver.LookupIPAddr(dialCtx, host)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch recipe from URL")
			}
			for _, ip := range ips {
				if !f.allowLoopback && IsBlockedIP(ip.IP) {
					return nil, fmt.Errorf("failed to fetch recipe from URL")
				}
			}
			dialer := &net.Dialer{Timeout: connectTimeout}
			return dialer.DialContext(dialCtx, network, net.JoinHostPort(ips[0].IP.String(), port))
		},
		TLSHandshakeTimeout:   tlsTimeout,
		ResponseHeaderTimeout:  headerTimeout,
	}

	client := &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("too many redirects")
			}
			// Re-validate each redirect target
			_, err := ValidateURL(req.URL.String())
			return err
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch recipe from URL")
	}
	req.Header.Set("User-Agent", "RecipeToReminders/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch recipe from URL")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch recipe from URL")
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, f.maxBodySize))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch recipe from URL")
	}
	return body, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./test/ -v -run "TestValidateURL|TestIsBlockedIP|TestFetcher"`
Expected: all PASS.

**Step 5: Commit**

```bash
git add internal/parser/urlvalidator.go test/security_test.go
git commit -m "feat: URL validator with SSRF protection

Scheme allowlist (http/https only), IP blocklist (RFC 1918, loopback,
link-local/IMDS, multicast), redirect re-validation (max 3 hops),
response body size limit, custom timeouts."
```

---

### Task 4: JSON-LD Parser

**Files:**
- Create: `internal/parser/jsonld.go`
- Create: `test/parser_test.go`

**Step 1: Write the failing tests**

Create `test/parser_test.go`:
```go
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
```

**Step 2: Run tests to verify they fail**

Run: `go test ./test/ -v -run "TestJSONLD" 2>&1 | head -10`
Expected: compilation error — `parser.JSONLDParser` doesn't exist yet.

**Step 3: Implement the JSON-LD parser**

Create `internal/parser/jsonld.go`:
```go
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
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err == nil {
		if r := tryExtractRecipe(obj); r != nil {
			return r
		}
		// Check for @graph array
		if graph, ok := obj["@graph"]; ok {
			if items, ok := graph.([]interface{}); ok {
				for _, item := range items {
					if m, ok := item.(map[string]interface{}); ok {
						if r := tryExtractRecipe(m); r != nil {
							return r
						}
					}
				}
			}
		}
	}

	// Try as an array of objects
	var arr []map[string]interface{}
	if err := json.Unmarshal(data, &arr); err == nil {
		for _, obj := range arr {
			if r := tryExtractRecipe(obj); r != nil {
				return r
			}
		}
	}

	return nil
}

func tryExtractRecipe(obj map[string]interface{}) *models.RawRecipe {
	typ, _ := obj["@type"].(string)
	if !strings.EqualFold(typ, "Recipe") {
		// @type can also be an array: ["Recipe"]
		if types, ok := obj["@type"].([]interface{}); ok {
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

func extractStringArray(v interface{}) []string {
	switch val := v.(type) {
	case []interface{}:
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
```

**Step 4: Run tests to verify they pass**

Run: `go test ./test/ -v -run "TestJSONLD"`
Expected: all PASS.

**Step 5: Commit**

```bash
git add internal/parser/jsonld.go test/parser_test.go
git commit -m "feat: JSON-LD parser extracts Recipe schema from HTML

Handles top-level objects, @graph arrays, and array-of-objects formats.
Extracts title, servings, and recipeIngredient strings."
```

---

### Task 5: Ingredient Normalization

**Files:**
- Create: `internal/ingredients/normalize.go`
- Create: `test/normalize_test.go`

**Step 1: Write the failing tests**

Create `test/normalize_test.go`:
```go
package test

import (
	"testing"

	"recipe-to-reminders/internal/ingredients"
)

func TestParseRawIngredient_Standard(t *testing.T) {
	tests := []struct {
		raw      string
		wantQty  string
		wantUnit string
		wantName string
	}{
		{"2 cups all-purpose flour", "2", "cups", "all-purpose flour"},
		{"1 tablespoon olive oil", "1", "tbsp", "olive oil"},
		{"3 large carrots, peeled and sliced", "3", "large", "carrots"},
		{"2 pounds beef chuck, cut into 1-inch cubes", "2", "lbs", "beef chuck"},
		{"1/2 teaspoon black pepper", "1/2", "tsp", "black pepper"},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			ing := ingredients.ParseRawIngredient(tt.raw)
			if ing.Quantity != tt.wantQty {
				t.Errorf("quantity = %q, want %q", ing.Quantity, tt.wantQty)
			}
			if ing.Unit != tt.wantUnit {
				t.Errorf("unit = %q, want %q", ing.Unit, tt.wantUnit)
			}
			if ing.Name != tt.wantName {
				t.Errorf("name = %q, want %q", ing.Name, tt.wantName)
			}
			if ing.Raw != tt.raw {
				t.Errorf("raw = %q, want %q", ing.Raw, tt.raw)
			}
		})
	}
}

func TestParseRawIngredient_UnicodeFractions(t *testing.T) {
	tests := []struct {
		raw      string
		wantQty  string
		wantUnit string
		wantName string
	}{
		{"½ teaspoon salt", "1/2", "tsp", "salt"},
		{"¼ cup sugar", "1/4", "cups", "sugar"},
		{"1½ cups all-purpose flour", "1 1/2", "cups", "all-purpose flour"},
		{"¾ teaspoon baking soda", "3/4", "tsp", "baking soda"},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			ing := ingredients.ParseRawIngredient(tt.raw)
			if ing.Quantity != tt.wantQty {
				t.Errorf("quantity = %q, want %q", ing.Quantity, tt.wantQty)
			}
			if ing.Unit != tt.wantUnit {
				t.Errorf("unit = %q, want %q", ing.Unit, tt.wantUnit)
			}
			if ing.Name != tt.wantName {
				t.Errorf("name = %q, want %q", ing.Name, tt.wantName)
			}
		})
	}
}

func TestParseRawIngredient_PrepStripping(t *testing.T) {
	tests := []struct {
		raw      string
		wantName string
	}{
		{"3 cloves garlic, minced", "garlic"},
		{"2 tablespoons butter (softened)", "butter"},
		{"1 cup onion, finely diced", "onion"},
		{"2 tablespoons unsalted butter, melted", "unsalted butter"},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			ing := ingredients.ParseRawIngredient(tt.raw)
			if ing.Name != tt.wantName {
				t.Errorf("name = %q, want %q", ing.Name, tt.wantName)
			}
		})
	}
}

func TestParseRawIngredient_EdgeCases(t *testing.T) {
	// "Salt and pepper to taste" — no quantity or standard unit
	ing := ingredients.ParseRawIngredient("Salt and pepper to taste")
	if ing.Name != "salt and pepper" {
		t.Errorf("name = %q, want %q", ing.Name, "salt and pepper")
	}
	if ing.Quantity != "" {
		t.Errorf("quantity = %q, want empty", ing.Quantity)
	}

	// Range quantity
	ing2 := ingredients.ParseRawIngredient("2-3 cloves garlic")
	if ing2.Quantity != "2-3" {
		t.Errorf("quantity = %q, want %q", ing2.Quantity, "2-3")
	}
	if ing2.Unit != "cloves" {
		t.Errorf("unit = %q, want %q", ing2.Unit, "cloves")
	}
	if ing2.Name != "garlic" {
		t.Errorf("name = %q, want %q", ing2.Name, "garlic")
	}
}

func TestNormalizeUnit(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"tablespoon", "tbsp"},
		{"tablespoons", "tbsp"},
		{"tbsp", "tbsp"},
		{"teaspoon", "tsp"},
		{"teaspoons", "tsp"},
		{"tsp", "tsp"},
		{"pound", "lbs"},
		{"pounds", "lbs"},
		{"lb", "lbs"},
		{"ounce", "oz"},
		{"ounces", "oz"},
		{"cup", "cups"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ingredients.NormalizeUnit(tt.input)
			if got != tt.want {
				t.Errorf("NormalizeUnit(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./test/ -v -run "TestParseRawIngredient|TestNormalizeUnit" 2>&1 | head -10`
Expected: compilation error.

**Step 3: Implement ingredient normalization**

Create `internal/ingredients/normalize.go`:
```go
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

// Matches: quantity (digits, fractions, ranges), then unit, then name
var mainPattern = regexp.MustCompile(
	`(?i)^([\d]+[\d/.\-–—]*(?:\s*[\d/]+)?)\s+` +
		`(tablespoons?|tbsp?|teaspoons?|tsp|cups?|ounces?|oz|pounds?|lbs?|lb|` +
		`grams?|g|kilograms?|kg|milliliters?|ml|liters?|l|` +
		`bunch(?:es)?|cloves?|cans?|pkg|packages?|sticks?|heads?|` +
		`large|medium|small|pinch|dash)\s+(.+)$`)

// Matches: quantity then name (no recognized unit)
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

	// Try "salt and pepper to taste" pattern
	if m := toTastePattern.FindStringSubmatch(s); m != nil {
		return models.Ingredient{
			Raw:  raw,
			Name: strings.ToLower(strings.TrimSpace(m[1])),
		}
	}

	// Try main pattern: qty + unit + name
	if m := mainPattern.FindStringSubmatch(s); m != nil {
		return models.Ingredient{
			Raw:      raw,
			Quantity: strings.TrimSpace(m[1]),
			Unit:     NormalizeUnit(strings.TrimSpace(m[2])),
			Name:     cleanName(m[3]),
		}
	}

	// Try qty + name (no recognized unit)
	if m := qtyNamePattern.FindStringSubmatch(s); m != nil {
		return models.Ingredient{
			Raw:      raw,
			Quantity: strings.TrimSpace(m[1]),
			Name:     cleanName(m[2]),
		}
	}

	// Fallback: treat entire string as name
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
	// Clean up "11/2" → "1 1/2" (digit directly before fraction replacement)
	re := regexp.MustCompile(`(\d)(1/[2-8]|2/3|3/4)`)
	s = re.ReplaceAllString(s, "$1 $2")
	return s
}

func cleanName(s string) string {
	name := prepSuffixes.ReplaceAllString(strings.TrimSpace(s), "")
	name = strings.TrimSpace(name)
	return strings.ToLower(name)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./test/ -v -run "TestParseRawIngredient|TestNormalizeUnit"`
Expected: all PASS. If any fail, adjust the regex patterns or normalization logic.

**Step 5: Commit**

```bash
git add internal/ingredients/normalize.go test/normalize_test.go
git commit -m "feat: ingredient normalization — parse raw strings into structured fields

Handles standard formats, Unicode fractions, ranges, prep instruction
stripping, and unit normalization."
```

---

### Task 6: Categorization

**Files:**
- Create: `internal/ingredients/categorize.go`
- Modify: `test/normalize_test.go` (add categorization tests)

**Step 1: Write the failing tests**

Append to `test/normalize_test.go`:
```go
func TestCategorize(t *testing.T) {
	tests := []struct {
		name     string
		wantCat  string
	}{
		{"carrots", "produce"},
		{"beef chuck", "meat"},
		{"butter", "dairy"},
		{"all-purpose flour", "pantry"},
		{"black pepper", "spices"},
		{"frozen peas", "frozen"},
		{"something unknown", "other"},
		{"chicken breast", "meat"},
		{"milk", "dairy"},
		{"olive oil", "pantry"},
		{"basil", "spices"},
		{"garlic", "produce"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ingredients.Categorize(tt.name)
			if got != tt.wantCat {
				t.Errorf("Categorize(%q) = %q, want %q", tt.name, got, tt.wantCat)
			}
		})
	}
}
```

**Step 2: Run to verify failure**

Run: `go test ./test/ -v -run "TestCategorize" 2>&1 | head -5`
Expected: compilation error.

**Step 3: Implement categorization**

Create `internal/ingredients/categorize.go`:
```go
package ingredients

import "strings"

var categoryKeywords = map[string][]string{
	"produce": {
		"carrot", "onion", "garlic", "potato", "tomato", "lettuce", "spinach",
		"celery", "pepper", "broccoli", "cauliflower", "zucchini", "squash",
		"cucumber", "mushroom", "avocado", "lemon", "lime", "orange", "apple",
		"banana", "berry", "strawberry", "blueberry", "grape", "mango",
		"pineapple", "peach", "pear", "ginger", "jalapeño", "shallot",
		"scallion", "green onion", "corn", "cabbage", "kale", "arugula",
		"beet", "radish", "turnip", "parsnip", "sweet potato", "eggplant",
	},
	"meat": {
		"beef", "chicken", "pork", "lamb", "turkey", "bacon", "sausage",
		"steak", "ground", "ham", "veal", "duck", "shrimp", "salmon",
		"tuna", "cod", "tilapia", "crab", "lobster", "scallop", "fish",
		"prosciutto", "pancetta", "anchov",
	},
	"dairy": {
		"milk", "butter", "cheese", "cream", "yogurt", "sour cream",
		"buttermilk", "parmesan", "mozzarella", "cheddar", "ricotta",
		"mascarpone", "whipping cream", "half-and-half", "egg",
	},
	"pantry": {
		"flour", "sugar", "oil", "vinegar", "soy sauce", "rice",
		"pasta", "noodle", "bread", "broth", "stock", "honey",
		"maple syrup", "mustard", "ketchup", "mayonnaise", "tomato paste",
		"tomato sauce", "coconut milk", "beans", "lentil", "chickpea",
		"peanut butter", "jam", "cornstarch", "baking powder", "baking soda",
		"yeast", "chocolate", "cocoa", "vanilla", "oat", "cereal",
		"cracker", "tortilla", "spaghetti", "penne",
	},
	"spices": {
		"salt", "pepper", "cumin", "paprika", "cinnamon", "nutmeg",
		"oregano", "basil", "thyme", "rosemary", "parsley", "cilantro",
		"dill", "bay leaf", "chili powder", "curry", "turmeric",
		"cayenne", "garlic powder", "onion powder", "clove",
		"cardamom", "coriander", "fennel seed", "red pepper flake",
		"italian seasoning", "sage", "tarragon", "mint",
	},
	"frozen": {
		"frozen",
	},
	"bakery": {
		"bread", "bun", "roll", "bagel", "croissant", "pita",
		"tortilla", "naan", "baguette",
	},
}

// Categorize assigns a grocery aisle category to an ingredient name.
func Categorize(name string) string {
	lower := strings.ToLower(name)

	// Check each category for keyword matches
	// Order matters: check more specific categories first
	for _, cat := range []string{"spices", "meat", "dairy", "produce", "frozen", "bakery", "pantry"} {
		for _, keyword := range categoryKeywords[cat] {
			if strings.Contains(lower, keyword) {
				return cat
			}
		}
	}
	return "other"
}
```

**Step 4: Run tests**

Run: `go test ./test/ -v -run "TestCategorize"`
Expected: all PASS.

**Step 5: Commit**

```bash
git add internal/ingredients/categorize.go test/normalize_test.go
git commit -m "feat: ingredient categorization by grocery aisle

Keyword-based assignment to produce, dairy, meat, pantry, spices,
frozen, bakery, or other."
```

---

### Task 7: Deduplication

**Files:**
- Create: `internal/ingredients/deduplicate.go`
- Modify: `test/normalize_test.go` (add dedup tests)

**Step 1: Write the failing tests**

Append to `test/normalize_test.go`:
```go
func TestDeduplicate_MergesSameNameSameUnit(t *testing.T) {
	input := []models.Ingredient{
		{Name: "flour", Quantity: "1", Unit: "cups", Category: "pantry", Raw: "1 cup flour"},
		{Name: "flour", Quantity: "2", Unit: "tbsp", Category: "pantry", Raw: "2 tbsp flour for dusting"},
	}
	result := ingredients.Deduplicate(input)
	// Different units — should NOT merge, kept as separate items
	if len(result) != 2 {
		t.Fatalf("got %d ingredients, want 2 (incompatible units)", len(result))
	}
}

func TestDeduplicate_MergesCompatibleUnits(t *testing.T) {
	input := []models.Ingredient{
		{Name: "olive oil", Quantity: "2", Unit: "tbsp", Category: "pantry", Raw: "2 tbsp olive oil"},
		{Name: "olive oil", Quantity: "1", Unit: "tbsp", Category: "pantry", Raw: "1 tbsp olive oil"},
	}
	result := ingredients.Deduplicate(input)
	if len(result) != 1 {
		t.Fatalf("got %d ingredients, want 1", len(result))
	}
	if result[0].Quantity != "3" {
		t.Errorf("quantity = %q, want %q", result[0].Quantity, "3")
	}
}

func TestDeduplicate_PreservesDistinctItems(t *testing.T) {
	input := []models.Ingredient{
		{Name: "flour", Quantity: "2", Unit: "cups", Category: "pantry", Raw: "2 cups flour"},
		{Name: "sugar", Quantity: "1", Unit: "cups", Category: "pantry", Raw: "1 cup sugar"},
	}
	result := ingredients.Deduplicate(input)
	if len(result) != 2 {
		t.Fatalf("got %d ingredients, want 2", len(result))
	}
}

func TestDeduplicate_NoQuantity(t *testing.T) {
	input := []models.Ingredient{
		{Name: "salt and pepper", Raw: "Salt and pepper to taste"},
	}
	result := ingredients.Deduplicate(input)
	if len(result) != 1 {
		t.Fatalf("got %d, want 1", len(result))
	}
}
```

Add the models import at the top of `test/normalize_test.go` if not already present:
```go
import (
	"testing"

	"recipe-to-reminders/internal/ingredients"
	"recipe-to-reminders/internal/models"
)
```

**Step 2: Run to verify failure**

Run: `go test ./test/ -v -run "TestDeduplicate" 2>&1 | head -5`
Expected: compilation error.

**Step 3: Implement deduplication**

Create `internal/ingredients/deduplicate.go`:
```go
package ingredients

import (
	"fmt"
	"strconv"
	"strings"

	"recipe-to-reminders/internal/models"
)

// Deduplicate merges ingredients with the same name and compatible units.
// Ingredients with different units are kept separate.
func Deduplicate(items []models.Ingredient) []models.Ingredient {
	type key struct {
		name string
		unit string
	}

	seen := make(map[key]int) // key → index in result
	var result []models.Ingredient

	for _, item := range items {
		k := key{name: strings.ToLower(item.Name), unit: strings.ToLower(item.Unit)}
		if idx, ok := seen[k]; ok {
			// Merge quantities
			merged := addQuantities(result[idx].Quantity, item.Quantity)
			result[idx].Quantity = merged
			result[idx].Raw = result[idx].Raw + " + " + item.Raw
		} else {
			seen[k] = len(result)
			result = append(result, item)
		}
	}
	return result
}

func addQuantities(a, b string) string {
	fa, errA := parseQuantity(a)
	fb, errB := parseQuantity(b)
	if errA != nil || errB != nil {
		// Can't parse — just concatenate
		if a == "" {
			return b
		}
		return a + " + " + b
	}
	sum := fa + fb
	// Return clean integer if possible
	if sum == float64(int(sum)) {
		return strconv.Itoa(int(sum))
	}
	return fmt.Sprintf("%.2g", sum)
}

func parseQuantity(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty")
	}

	// Handle fractions like "1/2"
	if parts := strings.SplitN(s, "/", 2); len(parts) == 2 {
		num, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		den, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if err1 != nil || err2 != nil || den == 0 {
			return 0, fmt.Errorf("invalid fraction")
		}
		return num / den, nil
	}

	// Handle "1 1/2" (mixed number) — space-separated
	if parts := strings.Fields(s); len(parts) == 2 {
		whole, err1 := strconv.ParseFloat(parts[0], 64)
		frac, err2 := parseQuantity(parts[1])
		if err1 == nil && err2 == nil {
			return whole + frac, nil
		}
	}

	return strconv.ParseFloat(s, 64)
}
```

**Step 4: Run tests**

Run: `go test ./test/ -v -run "TestDeduplicate"`
Expected: all PASS.

**Step 5: Commit**

```bash
git add internal/ingredients/deduplicate.go test/normalize_test.go
git commit -m "feat: ingredient deduplication — merge same name + same unit

Adds quantities when name and unit match. Keeps separate items for
different units. Handles fractions and mixed numbers in quantities."
```

---

### Task 8: Parser Dispatcher

**Files:**
- Create: `internal/parser/parser.go`
- Modify: `test/parser_test.go` (add dispatcher tests)

**Step 1: Write the failing tests**

Append to `test/parser_test.go`:
```go
func TestExtractor_JSONLDPath(t *testing.T) {
	html := loadFixture(t, "jsonld_simple.html")

	e := parser.NewExtractor(nil) // nil fetcher — we'll call ParseURL separately
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

func TestExtractor_NoRecipeReturnsError(t *testing.T) {
	html := loadFixture(t, "no_recipe.html")

	e := parser.NewExtractor(nil)
	_, err := e.ParseHTML(html, "https://example.com/about")
	if err == nil {
		t.Fatal("expected error for page with no recipe")
	}
}
```

**Step 2: Run to verify failure**

Run: `go test ./test/ -v -run "TestExtractor" 2>&1 | head -5`
Expected: compilation error.

**Step 3: Implement the dispatcher**

Create `internal/parser/parser.go`:
```go
package parser

import (
	"context"
	"fmt"

	"recipe-to-reminders/internal/ingredients"
	"recipe-to-reminders/internal/models"
)

// HTMLParser can extract a RawRecipe from HTML bytes.
type HTMLParser interface {
	Parse(html []byte, sourceURL string) (*models.RawRecipe, error)
}

// Extractor coordinates fetching and parsing a recipe URL.
type Extractor struct {
	fetcher      *Fetcher
	jsonld       HTMLParser
	htmlFallback HTMLParser
}

// NewExtractor creates an Extractor. Pass nil for fetcher if only using ParseHTML directly.
func NewExtractor(fetcher *Fetcher) *Extractor {
	return &Extractor{
		fetcher:      fetcher,
		jsonld:       JSONLDParser{},
		htmlFallback: nil, // set after HTML fallback is implemented
	}
}

// SetHTMLFallback sets the HTML fallback parser. Called after it's implemented.
func (e *Extractor) SetHTMLFallback(p HTMLParser) {
	e.htmlFallback = p
}

// Extract fetches a URL and extracts recipe ingredients.
func (e *Extractor) Extract(ctx context.Context, rawURL string) (*models.ExtractResponse, error) {
	if e.fetcher == nil {
		return nil, fmt.Errorf("no fetcher configured")
	}
	body, err := e.fetcher.Fetch(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	return e.ParseHTML(body, rawURL)
}

// ParseHTML extracts recipe data from raw HTML bytes. Tries JSON-LD first, then HTML fallback.
func (e *Extractor) ParseHTML(html []byte, sourceURL string) (*models.ExtractResponse, error) {
	// Try JSON-LD
	raw, err := e.jsonld.Parse(html, sourceURL)
	if err == nil && len(raw.Ingredients) > 0 {
		return e.processRaw(raw), nil
	}

	// Try HTML fallback
	if e.htmlFallback != nil {
		raw, err = e.htmlFallback.Parse(html, sourceURL)
		if err == nil && len(raw.Ingredients) > 0 {
			return e.processRaw(raw), nil
		}
	}

	return nil, fmt.Errorf("no recipe found")
}

func (e *Extractor) processRaw(raw *models.RawRecipe) *models.ExtractResponse {
	var parsed []models.Ingredient
	for _, s := range raw.Ingredients {
		ing := ingredients.ParseRawIngredient(s)
		ing.Category = ingredients.Categorize(ing.Name)
		parsed = append(parsed, ing)
	}
	parsed = ingredients.Deduplicate(parsed)

	return &models.ExtractResponse{
		Title:       raw.Title,
		Source:      raw.Source,
		Servings:    raw.Servings,
		Method:      raw.Method,
		Confidence:  raw.Confidence,
		Ingredients: parsed,
	}
}
```

**Step 4: Run tests**

Run: `go test ./test/ -v -run "TestExtractor"`
Expected: `TestExtractor_JSONLDPath` and `TestExtractor_NoRecipeReturnsError` PASS. `TestExtractor_FallsBackToHTML` FAILS (HTML fallback not implemented yet — that's expected and OK for now).

Note: if `TestExtractor_FallsBackToHTML` causes a test failure, skip it temporarily by adding `t.Skip("HTML fallback not yet implemented")` at the top. We'll remove the skip in Task 10.

**Step 5: Commit**

```bash
git add internal/parser/parser.go test/parser_test.go
git commit -m "feat: parser dispatcher — JSON-LD first, HTML fallback, normalization pipeline

Coordinates parsing flow: JSON-LD → HTML fallback → error. Processes
raw ingredient strings through normalization, categorization, and
deduplication."
```

---

### Task 9: HTTP Handler

**Files:**
- Create: `internal/handler/handler.go`
- Create: `internal/handler/extract.go`
- Modify: `test/parser_test.go` (add handler tests)

**Step 1: Write the failing tests**

Add to `test/parser_test.go`:
```go
import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	// ... existing imports ...

	"recipe-to-reminders/internal/handler"
	"recipe-to-reminders/internal/models"
)

func TestExtractHandler_ValidURL(t *testing.T) {
	// Set up a test recipe server
	recipeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write(loadFixture(t, "jsonld_simple.html"))
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

func TestExtractHandler_ImageBase64Unsupported(t *testing.T) {
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
	json.Unmarshal(rr.Body.Bytes(), &errResp)
	if errResp.Error != "image_not_supported" {
		t.Errorf("error = %q, want %q", errResp.Error, "image_not_supported")
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
```

**Step 2: Run to verify failure**

Run: `go test ./test/ -v -run "TestExtractHandler" 2>&1 | head -5`
Expected: compilation error.

**Step 3: Implement the handler**

Create `internal/handler/handler.go`:
```go
package handler

import (
	"encoding/json"
	"net/http"

	"recipe-to-reminders/internal/models"
	"recipe-to-reminders/internal/parser"
)

// Handler routes HTTP requests to the appropriate endpoint handler.
type Handler struct {
	mux       *http.ServeMux
	extractor *parser.Extractor
}

// New creates a Handler wired to a Fetcher for URL extraction.
func New(fetcher *parser.Fetcher) *Handler {
	h := &Handler{
		mux:       http.NewServeMux(),
		extractor: parser.NewExtractor(fetcher),
	}
	h.mux.HandleFunc("POST /extract", h.handleExtract)
	// Return 405 for non-POST on /extract
	h.mux.HandleFunc("/extract", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for /extract")
	})
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, models.ErrorResponse{Error: code, Message: message})
}
```

Create `internal/handler/extract.go`:
```go
package handler

import (
	"encoding/json"
	"net/http"

	"recipe-to-reminders/internal/models"
)

const maxRequestBody = 10 * 1024 * 1024 // 10MB (API Gateway limit)

func (h *Handler) handleExtract(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)

	var req models.ExtractRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	// image_base64 not yet supported
	if req.ImageBase64 != "" {
		writeError(w, http.StatusBadRequest, "image_not_supported",
			"Image extraction is not yet supported. Please provide a URL.")
		return
	}

	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "missing_input",
			"Provide a URL in the request body.")
		return
	}

	result, err := h.extractor.Extract(r.Context(), req.URL)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "no_recipe_found",
			"Could not extract recipe data from the provided URL.")
		return
	}

	writeJSON(w, http.StatusOK, result)
}
```

**Step 4: Run tests**

Run: `go test ./test/ -v -run "TestExtractHandler"`
Expected: all PASS.

**Step 5: Commit**

```bash
git add internal/handler/handler.go internal/handler/extract.go test/parser_test.go
git commit -m "feat: HTTP handler for POST /extract

Routes requests, validates input (reject image_base64 with 'not yet
supported'), calls parser extractor, returns JSON response."
```

---

### Task 10: HTML Fallback Parser

**Files:**
- Create: `internal/parser/htmlfallback.go`
- Modify: `internal/parser/parser.go` (wire fallback into extractor)
- Modify: `test/parser_test.go` (remove skip, add fallback-specific tests)

**Step 1: Write the failing tests**

Add to `test/parser_test.go`:
```go
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
```

Also remove the `t.Skip` from `TestExtractor_FallsBackToHTML` if it was added.

**Step 2: Run to verify failure**

Run: `go test ./test/ -v -run "TestHTMLFallback" 2>&1 | head -5`
Expected: compilation error.

**Step 3: Implement HTML fallback parser**

Create `internal/parser/htmlfallback.go`:
```go
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
```

**Step 4: Wire the HTML fallback into the Extractor**

Modify `internal/parser/parser.go` — update `NewExtractor`:
```go
func NewExtractor(fetcher *Fetcher) *Extractor {
	return &Extractor{
		fetcher:      fetcher,
		jsonld:       JSONLDParser{},
		htmlFallback: HTMLFallbackParser{},
	}
}
```

Remove the `SetHTMLFallback` method — it's no longer needed.

**Step 5: Run all tests**

Run: `go test ./test/ -v`
Expected: all PASS, including `TestExtractor_FallsBackToHTML`.

**Step 6: Commit**

```bash
git add internal/parser/htmlfallback.go internal/parser/parser.go test/parser_test.go
git commit -m "feat: HTML fallback parser for sites without JSON-LD

Heuristic strategies: class/id containing 'ingredient', microdata
itemprop, heading + adjacent list. Wired into extractor as fallback
after JSON-LD."
```

---

### Task 11: Lambda Entrypoint + Smoke Test

**Files:**
- Create: `cmd/lambda/main.go`

**Step 1: Write the entrypoint**

Create `cmd/lambda/main.go`:
```go
package main

import (
	"log"
	"net/http"
	"os"

	"recipe-to-reminders/internal/handler"
	"recipe-to-reminders/internal/parser"
)

func main() {
	fetcher := parser.NewFetcher()
	h := handler.New(fetcher)

	addr := ":8080"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}

	log.Printf("Listening on %s", addr)
	if err := http.ListenAndServe(addr, h); err != nil {
		log.Fatal(err)
	}
}
```

**Step 2: Build and verify**

Run: `go build ./cmd/lambda/`
Expected: produces a `lambda` binary, clean exit.

**Step 3: Run the full test suite**

Run: `go test ./test/ -v -count=1`
Expected: all tests PASS.

**Step 4: Run with coverage**

Run: `go test -cover -coverprofile=coverage.out ./test/`
Expected: coverage report printed. Check that core packages have reasonable coverage.

**Step 5: Commit**

```bash
git add cmd/lambda/main.go
git commit -m "feat: local dev server entrypoint on :8080

Plain net/http server for local development. Lambda adapter will be
added in a later milestone."
```

**Step 6: Manual smoke test**

Run: `go run ./cmd/lambda/ &`

Then in another terminal:
```bash
curl -s -X POST http://localhost:8080/extract \
  -H "Content-Type: application/json" \
  -d '{"url":"https://www.allrecipes.com/recipe/26317/chicken-pot-pie-ix/"}' | python3 -m json.tool
```

Expected: JSON response with title, ingredients, method "jsonld" or "html", confidence score. Kill the server with `kill %1`.

Note: this hits a live URL. If it fails (site blocks requests, format changed), that's OK — it's a smoke test, not a CI test. The unit tests with fixtures are the source of truth.

**Step 7: Clean up build artifacts**

```bash
rm -f lambda coverage.out
echo -e "lambda\ncoverage.out" >> .gitignore
git add .gitignore
git commit -m "chore: add .gitignore for build artifacts"
```

---

## Summary

After completing all 11 tasks, you'll have:

- **Working `POST /extract` endpoint** that accepts a recipe URL and returns normalized, categorized ingredients
- **JSON-LD parser** (primary) and **HTML fallback parser** (secondary)
- **SSRF protection** with scheme allowlist, IP blocklist, redirect validation, and response size limiting
- **Ingredient normalization** with Unicode fraction handling, unit normalization, and prep instruction stripping
- **Categorization** by grocery aisle and **deduplication** of same-ingredient entries
- **Unit tests** covering parsers, normalization, security, handler, and the full pipeline
- **Local dev server** on `:8080` for manual testing

Total: ~11 commits, each building on the last. Run `go test ./test/ -v` at any point to verify everything still works.
