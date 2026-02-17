# Milestone 3: Saved Recipes (CRUD + S3) — Design

## Goal

Save, list, update, and delete recipes in S3. Multi-recipe shopping list with intelligent ingredient merging.

## Architecture

A `storage.Store` interface abstracts S3 access. The `S3Store` implementation reads/writes a single `recipes.json` object using ETag-based optimistic concurrency. All recipe CRUD goes through 6 new HTTP endpoints in `handler/recipes.go`. Multi-recipe shopping uses `ingredients/merge.go` for cross-recipe ingredient combination with source tracking.

```
Handler (recipes.go)
  → validates input (ID format, field limits)
  → calls Store.ReadAll() to get RecipeCollection + ETag
  → mutates in memory
  → calls Store.WriteAll() with ETag for conditional put
  → retries on ETag mismatch (3 attempts, exponential backoff)
```

## Storage Layer

- **`Store` interface** — `ReadAll(ctx) → (RecipeCollection, etag, error)`, `WriteAll(ctx, collection, etag) → error`
- **`S3Store`** — AWS SDK v2 `s3.Client`
  - `GetObject` captures ETag on read
  - `PutObject` uses `If-Match` conditional header on write
  - Retry on `PreconditionFailed`: 50ms, 200ms, 500ms (max 3 attempts)
  - Corruption recovery: if `json.Unmarshal` fails, try previous S3 object version
  - Log warning if file exceeds 1MB
- **Tests** — Mock `Store` interface (no localstack dependency)

## Models

New types in `internal/models/recipe.go`:

- `SavedRecipe` — id, name, source, source_url, servings, created_at, updated_at, tags, ingredients
- `RecipeCollection` — version (int), updated_at, recipes array (S3 file schema)
- `RecipeSummary` — id, name, tags, ingredient_count (list endpoint response)
- `SaveRecipeRequest` — name, source, source_url, servings, tags, ingredients
- `ShopRequest` — recipe_ids, deduplicate
- `ShopResponse` — recipes_included, ingredients (with sources field)

## API Endpoints

| Route | Method | Handler | Description |
|-------|--------|---------|-------------|
| `/recipes` | GET | List | Summary list (no full ingredients) |
| `/recipes/{id}` | GET | Get | Full recipe with ingredients |
| `/recipes` | POST | Save | Generate slug-6hex ID, save new |
| `/recipes/{id}` | PUT | Update | Partial update, only included fields |
| `/recipes/{id}` | DELETE | Delete | Remove by ID |
| `/recipes/shop` | POST | Shop | Merge ingredients from multiple recipes |

## Ingredient Merging

For `POST /recipes/shop`:

1. Normalize names via synonym map ("butter" = "unsalted butter")
2. Same unit → add quantities ("2 tbsp + 1.5 tbsp = 3.5 tbsp")
3. Convertible units → normalize first then add ("1 cup + 8 tbsp = 1.5 cups")
4. Incompatible units → keep as separate items
5. Track sources per merged ingredient (which recipes contributed)

## Input Validation

| Resource | Limit |
|----------|-------|
| Total saved recipes | 200 |
| Ingredients per recipe | 100 |
| Recipe name | 200 chars |
| Tag length | 50 chars |
| Tags per recipe | 20 |
| Ingredient raw field | 500 chars |
| Ingredient name field | 200 chars |
| recipe_ids in shop | 20 |

Recipe ID format: `^[a-z0-9][a-z0-9-]*-[a-f0-9]{6}$`

String sanitization: remove null bytes/control chars, validate UTF-8, trim whitespace, enforce limits.

## Testing Strategy

- **Storage tests** — Mock S3 client for CRUD, ETag-based retry, corruption recovery, max file size warning
- **Merge tests** — Same unit addition, compatible unit conversion, incompatible unit separation, synonym matching, source tracking
- **Handler tests** — All 6 endpoints with mock store, input validation, error responses
- **Validation tests** — Field limits, ID format, oversized payloads, path traversal rejection

## Tech Stack

- AWS SDK for Go v2 (`github.com/aws/aws-sdk-go-v2/service/s3`)
- Existing ingredient pipeline (normalize, categorize, deduplicate)
- New merge logic for cross-recipe combination

## Files

**Create:**
- `internal/storage/storage.go` — Store interface
- `internal/storage/s3store.go` — S3 implementation
- `internal/ingredients/merge.go` — Cross-recipe ingredient merging
- `internal/handler/recipes.go` — Recipe CRUD + shop handlers
- `test/storage_test.go`
- `test/merge_test.go`

**Modify:**
- `internal/models/recipe.go` — Add SavedRecipe, RecipeCollection, request/response types
- `internal/handler/handler.go` — Add recipe routes, accept Store dependency
- `cmd/lambda/main.go` — Wire S3 store from env vars
- `go.mod` — Add AWS SDK v2
