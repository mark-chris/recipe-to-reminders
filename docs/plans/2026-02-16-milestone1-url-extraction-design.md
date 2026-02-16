# Milestone 1: URL Extraction

## Goal

`POST /extract` with a URL returns normalized, categorized ingredients as JSON. Local dev server with unit tests. No photos, saved recipes, or cloud deployment.

## Components

### Models (`internal/models/recipe.go`)
- `Ingredient`: name, quantity, unit, category, raw
- `ParseResult`: title, source, servings, method, confidence, ingredients
- `ExtractRequest` / `ExtractResponse` for the handler

### URL Validator (`internal/parser/urlvalidator.go`)
- `ValidateURL(rawURL string) error` — scheme allowlist, DNS resolve, IP blocklist
- `SafeFetch(ctx context.Context, rawURL string) (*http.Response, error)` — validate + fetch with redirect re-validation (max 3 hops), `io.LimitReader` on body
- Custom `http.Client` with timeouts: connect 5s, TLS 5s, header 10s, overall 15s
- Blocked ranges: RFC 1918, loopback, link-local/IMDS, multicast, unspecified

### JSON-LD Parser (`internal/parser/jsonld.go`)
- Fetch HTML via `SafeFetch`
- Find `<script type="application/ld+json">` via goquery
- Handle `@type: "Recipe"` in objects, arrays, and `@graph` wrappers
- Extract `recipeIngredient` → raw strings
- Return ParseResult with method `"jsonld"`, confidence 1.0

### HTML Fallback Parser (`internal/parser/htmlfallback.go`)
- Triggered when JSON-LD finds no Recipe schema
- Heuristic DOM search: ingredient list containers, `<li>` items, microdata/RDFa
- Confidence 0.8–0.9 depending on signal strength

### Ingredient Normalization (`internal/ingredients/normalize.go`)
- Parse raw strings → quantity, unit, name
- Strip prep instructions (comma clauses, parentheticals, known suffixes)
- Normalize unit abbreviations
- Handle Unicode fractions, ranges

### Categorization (`internal/ingredients/categorize.go`)
- Keyword lookup map → produce, dairy, meat, pantry, spices, frozen, bakery, other

### Deduplication (`internal/ingredients/deduplicate.go`)
- Merge identical ingredient names within a single recipe when units are compatible

### Parser Dispatcher (`internal/parser/parser.go`)
- `Extract(ctx, url) (*ParseResult, error)` — JSON-LD → HTML fallback → error
- Wires: URL validation → fetch → parse → normalize → categorize → dedup

### HTTP Handler (`internal/handler/`)
- `handler.go`: route registration, JSON response helpers
- `extract.go`: validate request, call dispatcher, return response
- Reject `image_base64` with "not yet supported"

### Entrypoint (`cmd/lambda/main.go`)
- Plain `net/http` server for local dev (Lambda adapter added later)

## Test Fixtures (`test/fixtures/`)
- Valid JSON-LD Recipe schema
- JSON-LD inside `@graph` array
- No JSON-LD, parseable HTML ingredient list
- No recipe at all (error case)
- Edge case ingredient strings for normalization

## Out of Scope
- Photo extraction (Tesseract, Claude, confidence scoring)
- Saved recipes (CRUD, S3, merge/shop)
- Terraform / Lambda deployment
- Integration tests against live URLs
