# Saved Recipes (CRUD + S3) Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Save, list, update, and delete recipes in S3 with multi-recipe shopping list and intelligent ingredient merging.

**Architecture:** A `storage.Store` interface abstracts S3 access. The `S3Store` implementation reads/writes a single `recipes.json` object using ETag-based optimistic concurrency. All recipe CRUD goes through 6 new HTTP endpoints in `handler/recipes.go`. Multi-recipe shopping uses `ingredients/merge.go` for cross-recipe ingredient combination with source tracking. The handler uses a read-modify-write retry loop (3 attempts, exponential backoff) for all mutations.

**Tech Stack:** AWS SDK for Go v2 (`github.com/aws/aws-sdk-go-v2/service/s3`), existing ingredient pipeline (normalize, categorize, deduplicate), Go 1.25 stdlib

---

### Task 1: Models

Add saved recipe types to `internal/models/recipe.go`. No new files — extend the existing models.

**Files:**
- Modify: `internal/models/recipe.go`
- Test: `test/models_test.go` (create)

**Step 1: Write the failing test**

Create `test/models_test.go`:

```go
package test

import (
	"encoding/json"
	"testing"

	"recipe-to-reminders/internal/models"
)

func TestSavedRecipe_JSONRoundTrip(t *testing.T) {
	recipe := models.SavedRecipe{
		ID:        "beef-stew-abc123",
		Name:      "Classic Beef Stew",
		Source:    "allrecipes.com",
		SourceURL: "https://www.allrecipes.com/recipe/123",
		Servings:  "6",
		CreatedAt: "2026-01-10T08:30:00Z",
		UpdatedAt: "2026-01-10T08:30:00Z",
		Tags:      []string{"dinner", "winter"},
		Ingredients: []models.Ingredient{
			{Name: "beef chuck", Quantity: "2", Unit: "lbs", Category: "meat", Raw: "2 pounds beef chuck"},
		},
	}

	data, err := json.Marshal(recipe)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got models.SavedRecipe
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.ID != recipe.ID {
		t.Errorf("ID = %q, want %q", got.ID, recipe.ID)
	}
	if got.Name != recipe.Name {
		t.Errorf("Name = %q, want %q", got.Name, recipe.Name)
	}
	if len(got.Tags) != 2 {
		t.Errorf("Tags len = %d, want 2", len(got.Tags))
	}
	if len(got.Ingredients) != 1 {
		t.Errorf("Ingredients len = %d, want 1", len(got.Ingredients))
	}
}

func TestRecipeCollection_JSONRoundTrip(t *testing.T) {
	col := models.RecipeCollection{
		Version:   1,
		UpdatedAt: "2026-02-15T12:00:00Z",
		Recipes: []models.SavedRecipe{
			{ID: "test-abc123", Name: "Test Recipe"},
		},
	}

	data, err := json.Marshal(col)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var got models.RecipeCollection
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got.Version != 1 {
		t.Errorf("Version = %d, want 1", got.Version)
	}
	if len(got.Recipes) != 1 {
		t.Errorf("Recipes len = %d, want 1", len(got.Recipes))
	}
}

func TestRecipeSummary_Fields(t *testing.T) {
	s := models.RecipeSummary{
		ID:              "test-abc123",
		Name:            "Test",
		Tags:            []string{"a"},
		IngredientCount: 5,
	}
	data, _ := json.Marshal(s)
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	if m["ingredient_count"].(float64) != 5 {
		t.Errorf("ingredient_count = %v, want 5", m["ingredient_count"])
	}
}

func TestMergedIngredient_HasSources(t *testing.T) {
	mi := models.MergedIngredient{
		Name:     "butter",
		Quantity: "3.5",
		Unit:     "tbsp",
		Category: "dairy",
		Sources:  []string{"Recipe A (2 tbsp)", "Recipe B (1.5 tbsp)"},
	}
	data, _ := json.Marshal(mi)
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	sources := m["sources"].([]any)
	if len(sources) != 2 {
		t.Errorf("sources len = %d, want 2", len(sources))
	}
}

func TestShopRequest_Fields(t *testing.T) {
	req := models.ShopRequest{
		RecipeIDs:   []string{"a-abc123", "b-def456"},
		Deduplicate: true,
	}
	data, _ := json.Marshal(req)
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	ids := m["recipe_ids"].([]any)
	if len(ids) != 2 {
		t.Errorf("recipe_ids len = %d, want 2", len(ids))
	}
}

func TestShopResponse_Fields(t *testing.T) {
	resp := models.ShopResponse{
		RecipesIncluded: []string{"Recipe A"},
		Ingredients: []models.MergedIngredient{
			{Name: "flour", Quantity: "2", Unit: "cups", Category: "pantry"},
		},
	}
	data, _ := json.Marshal(resp)
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	if len(m["recipes_included"].([]any)) != 1 {
		t.Errorf("recipes_included len wrong")
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./test/ -run TestSavedRecipe -v`
Expected: FAIL — `models.SavedRecipe` undefined

**Step 3: Write minimal implementation**

Add to `internal/models/recipe.go` (after the existing types):

```go
// SavedRecipe is a recipe stored in S3 for reuse.
type SavedRecipe struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Source      string       `json:"source"`
	SourceURL   string       `json:"source_url"`
	Servings    string       `json:"servings"`
	CreatedAt   string       `json:"created_at"`
	UpdatedAt   string       `json:"updated_at"`
	Tags        []string     `json:"tags"`
	Ingredients []Ingredient `json:"ingredients"`
}

// RecipeCollection is the top-level schema for the S3 recipes.json file.
type RecipeCollection struct {
	Version   int           `json:"version"`
	UpdatedAt string        `json:"updated_at"`
	Recipes   []SavedRecipe `json:"recipes"`
}

// RecipeSummary is a lightweight recipe for list responses (no ingredients).
type RecipeSummary struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Tags            []string `json:"tags"`
	IngredientCount int      `json:"ingredient_count"`
}

// SaveRecipeRequest is the JSON body for POST /recipes.
type SaveRecipeRequest struct {
	Name        string       `json:"name"`
	Source      string       `json:"source"`
	SourceURL   string       `json:"source_url"`
	Servings    string       `json:"servings"`
	Tags        []string     `json:"tags"`
	Ingredients []Ingredient `json:"ingredients"`
}

// ShopRequest is the JSON body for POST /recipes/shop.
type ShopRequest struct {
	RecipeIDs   []string `json:"recipe_ids"`
	Deduplicate bool     `json:"deduplicate"`
}

// MergedIngredient is an ingredient with source tracking for multi-recipe shopping.
type MergedIngredient struct {
	Name     string   `json:"name"`
	Quantity string   `json:"quantity"`
	Unit     string   `json:"unit"`
	Category string   `json:"category"`
	Sources  []string `json:"sources"`
}

// ShopResponse is the JSON body returned by POST /recipes/shop.
type ShopResponse struct {
	RecipesIncluded []string           `json:"recipes_included"`
	Ingredients     []MergedIngredient `json:"ingredients"`
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./test/ -run "TestSavedRecipe|TestRecipeCollection|TestRecipeSummary|TestMergedIngredient|TestShopRequest|TestShopResponse" -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add internal/models/recipe.go test/models_test.go
git commit -m "feat: add saved recipe model types

Add SavedRecipe, RecipeCollection, RecipeSummary, SaveRecipeRequest,
ShopRequest, ShopResponse, and MergedIngredient types for Milestone 3."
```

---

### Task 2: Storage Interface + S3 Store

Create the `Store` interface and `S3Store` implementation with ETag-based optimistic concurrency, retry logic, and corruption recovery. All tests use a mock S3 client — no localstack.

**Files:**
- Create: `internal/storage/storage.go` — Store interface + sentinel errors
- Create: `internal/storage/s3store.go` — S3 implementation
- Create: `test/storage_test.go` — Tests with mock S3 client

**Step 1: Write the failing tests**

Create `test/storage_test.go`:

```go
package test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"recipe-to-reminders/internal/models"
	"recipe-to-reminders/internal/storage"
)

// MockS3API implements storage.S3API for testing.
type MockS3API struct {
	Data    []byte
	ETag    string
	GetErr  error
	PutErr  error
	PutCond string // captures the If-Match value from the last PutObject call

	// For versioning / corruption recovery
	VersionData []byte
	VersionErr  error
}

func (m *MockS3API) GetObject(_ context.Context, bucket, key string) (io.ReadCloser, string, error) {
	if m.GetErr != nil {
		return nil, "", m.GetErr
	}
	if m.Data == nil {
		return nil, "", storage.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(m.Data)), m.ETag, nil
}

func (m *MockS3API) PutObject(_ context.Context, bucket, key string, data []byte, ifMatch string) error {
	m.PutCond = ifMatch
	if m.PutErr != nil {
		return m.PutErr
	}
	m.Data = data
	m.ETag = "new-etag"
	return nil
}

func (m *MockS3API) GetObjectVersion(_ context.Context, bucket, key, versionID string) (io.ReadCloser, error) {
	if m.VersionErr != nil {
		return nil, m.VersionErr
	}
	if m.VersionData == nil {
		return nil, errors.New("no version data")
	}
	return io.NopCloser(bytes.NewReader(m.VersionData)), nil
}

func (m *MockS3API) ListObjectVersions(_ context.Context, bucket, key string, maxKeys int) ([]storage.ObjectVersion, error) {
	if m.VersionData != nil {
		return []storage.ObjectVersion{{VersionID: "prev-version", IsLatest: false}}, nil
	}
	return nil, nil
}

func newTestCollection(recipes ...models.SavedRecipe) *models.RecipeCollection {
	return &models.RecipeCollection{
		Version:   1,
		UpdatedAt: "2026-02-17T00:00:00Z",
		Recipes:   recipes,
	}
}

func TestS3Store_ReadAll_Empty(t *testing.T) {
	mock := &MockS3API{GetErr: storage.ErrNotFound}
	store := storage.NewS3Store(mock, "test-bucket", "recipes.json")

	col, etag, err := store.ReadAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if etag != "" {
		t.Errorf("etag = %q, want empty for new collection", etag)
	}
	if col.Version != 1 {
		t.Errorf("version = %d, want 1", col.Version)
	}
	if len(col.Recipes) != 0 {
		t.Errorf("recipes len = %d, want 0", len(col.Recipes))
	}
}

func TestS3Store_ReadAll_ExistingData(t *testing.T) {
	col := newTestCollection(models.SavedRecipe{ID: "test-abc123", Name: "Test"})
	data, _ := json.Marshal(col)
	mock := &MockS3API{Data: data, ETag: "etag-1"}
	store := storage.NewS3Store(mock, "test-bucket", "recipes.json")

	got, etag, err := store.ReadAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if etag != "etag-1" {
		t.Errorf("etag = %q, want etag-1", etag)
	}
	if len(got.Recipes) != 1 {
		t.Fatalf("recipes len = %d, want 1", len(got.Recipes))
	}
	if got.Recipes[0].ID != "test-abc123" {
		t.Errorf("recipe ID = %q, want test-abc123", got.Recipes[0].ID)
	}
}

func TestS3Store_WriteAll_Success(t *testing.T) {
	mock := &MockS3API{ETag: "etag-1"}
	store := storage.NewS3Store(mock, "test-bucket", "recipes.json")

	col := newTestCollection(models.SavedRecipe{ID: "new-abc123", Name: "New"})
	err := store.WriteAll(context.Background(), col, "etag-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if mock.PutCond != "etag-1" {
		t.Errorf("If-Match = %q, want etag-1", mock.PutCond)
	}
}

func TestS3Store_WriteAll_EmptyETag(t *testing.T) {
	mock := &MockS3API{}
	store := storage.NewS3Store(mock, "test-bucket", "recipes.json")

	col := newTestCollection()
	err := store.WriteAll(context.Background(), col, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty ETag means no condition — first write to new file
	if mock.PutCond != "" {
		t.Errorf("If-Match = %q, want empty for new file", mock.PutCond)
	}
}

func TestS3Store_WriteAll_ConflictError(t *testing.T) {
	mock := &MockS3API{PutErr: storage.ErrConflict}
	store := storage.NewS3Store(mock, "test-bucket", "recipes.json")

	col := newTestCollection()
	err := store.WriteAll(context.Background(), col, "stale-etag")
	if !errors.Is(err, storage.ErrConflict) {
		t.Errorf("err = %v, want ErrConflict", err)
	}
}

func TestS3Store_ReadAll_CorruptJSON_RecoverFromVersion(t *testing.T) {
	good := newTestCollection(models.SavedRecipe{ID: "ok-abc123", Name: "OK"})
	goodData, _ := json.Marshal(good)

	mock := &MockS3API{
		Data:        []byte("corrupted{{{json"),
		ETag:        "etag-bad",
		VersionData: goodData,
	}
	store := storage.NewS3Store(mock, "test-bucket", "recipes.json")

	col, _, err := store.ReadAll(context.Background())
	if err != nil {
		t.Fatalf("expected recovery, got error: %v", err)
	}
	if len(col.Recipes) != 1 || col.Recipes[0].ID != "ok-abc123" {
		t.Errorf("expected recovered recipe, got %+v", col.Recipes)
	}
}

func TestS3Store_ReadAll_CorruptJSON_NoVersion(t *testing.T) {
	mock := &MockS3API{
		Data: []byte("corrupted{{{json"),
		ETag: "etag-bad",
	}
	store := storage.NewS3Store(mock, "test-bucket", "recipes.json")

	_, _, err := store.ReadAll(context.Background())
	if err == nil {
		t.Fatal("expected error for unrecoverable corruption")
	}
}

func TestS3Store_ReadAll_GetError(t *testing.T) {
	mock := &MockS3API{GetErr: errors.New("s3 unavailable")}
	store := storage.NewS3Store(mock, "test-bucket", "recipes.json")

	_, _, err := store.ReadAll(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestS3Store_ReadAll_LargeFile_Warning(t *testing.T) {
	// Create a collection that's > 1MB when serialized.
	// We use a recipe with a very long name repeated many times.
	var recipes []models.SavedRecipe
	for i := 0; i < 250; i++ {
		r := models.SavedRecipe{
			ID:   "x-" + string(rune('a'+i%26)) + "bc123",
			Name: string(make([]byte, 4000)),
		}
		recipes = append(recipes, r)
	}
	col := newTestCollection(recipes...)
	data, _ := json.Marshal(col)

	mock := &MockS3API{Data: data, ETag: "etag-large"}
	store := storage.NewS3Store(mock, "test-bucket", "recipes.json")

	// Should still succeed — just log a warning internally
	got, _, err := store.ReadAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Recipes) != 250 {
		t.Errorf("recipes len = %d, want 250", len(got.Recipes))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./test/ -run "TestS3Store" -v`
Expected: FAIL — `storage` package doesn't exist

**Step 3: Write minimal implementation**

Create `internal/storage/storage.go`:

```go
package storage

import (
	"context"
	"errors"
	"io"

	"recipe-to-reminders/internal/models"
)

// Sentinel errors for storage operations.
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict: ETag mismatch")
)

// Store abstracts recipe collection persistence.
type Store interface {
	// ReadAll reads the entire recipe collection. Returns an empty collection
	// (not an error) if no data exists yet.
	ReadAll(ctx context.Context) (*models.RecipeCollection, string, error)

	// WriteAll writes the entire recipe collection. The etag parameter is used
	// for conditional writes (If-Match). Pass empty string for the first write.
	WriteAll(ctx context.Context, collection *models.RecipeCollection, etag string) error
}

// S3API abstracts the subset of S3 operations needed by S3Store.
type S3API interface {
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, string, error)
	PutObject(ctx context.Context, bucket, key string, data []byte, ifMatch string) error
	GetObjectVersion(ctx context.Context, bucket, key, versionID string) (io.ReadCloser, error)
	ListObjectVersions(ctx context.Context, bucket, key string, maxKeys int) ([]ObjectVersion, error)
}

// ObjectVersion represents an S3 object version.
type ObjectVersion struct {
	VersionID string
	IsLatest  bool
}
```

Create `internal/storage/s3store.go`:

```go
package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"

	"recipe-to-reminders/internal/models"
)

const maxFileSizeWarning = 1024 * 1024 // 1MB

// S3Store implements Store using a single S3 JSON object.
type S3Store struct {
	client S3API
	bucket string
	key    string
}

// NewS3Store creates an S3Store.
func NewS3Store(client S3API, bucket, key string) *S3Store {
	return &S3Store{client: client, bucket: bucket, key: key}
}

// ReadAll reads the recipe collection from S3.
// Returns an empty collection if the object doesn't exist.
// On JSON corruption, attempts to recover from a previous S3 version.
func (s *S3Store) ReadAll(ctx context.Context) (*models.RecipeCollection, string, error) {
	body, etag, err := s.client.GetObject(ctx, s.bucket, s.key)
	if err != nil {
		if err == ErrNotFound {
			return &models.RecipeCollection{Version: 1, Recipes: []models.SavedRecipe{}}, "", nil
		}
		return nil, "", fmt.Errorf("s3 get: %w", err)
	}
	defer body.Close()

	data, err := io.ReadAll(body)
	if err != nil {
		return nil, "", fmt.Errorf("s3 read body: %w", err)
	}

	if len(data) > maxFileSizeWarning {
		log.Printf("WARNING: recipes.json is %d bytes (>1MB) — consider migrating to DynamoDB", len(data))
	}

	var col models.RecipeCollection
	if err := json.Unmarshal(data, &col); err != nil {
		log.Printf("ERROR: recipes.json corrupt: %v — attempting version recovery", err)
		return s.recoverFromVersion(ctx)
	}

	return &col, etag, nil
}

// WriteAll writes the recipe collection to S3 with optional ETag condition.
func (s *S3Store) WriteAll(ctx context.Context, collection *models.RecipeCollection, etag string) error {
	data, err := json.Marshal(collection)
	if err != nil {
		return fmt.Errorf("marshal collection: %w", err)
	}

	if err := s.client.PutObject(ctx, s.bucket, s.key, data, etag); err != nil {
		return err
	}

	return nil
}

// recoverFromVersion attempts to read the previous S3 object version.
func (s *S3Store) recoverFromVersion(ctx context.Context) (*models.RecipeCollection, string, error) {
	versions, err := s.client.ListObjectVersions(ctx, s.bucket, s.key, 5)
	if err != nil {
		return nil, "", fmt.Errorf("list versions failed: %w", err)
	}

	// Find the first non-latest version.
	for _, v := range versions {
		if v.IsLatest {
			continue
		}
		body, err := s.client.GetObjectVersion(ctx, s.bucket, s.key, v.VersionID)
		if err != nil {
			continue
		}
		defer body.Close()

		data, err := io.ReadAll(body)
		if err != nil {
			continue
		}

		var col models.RecipeCollection
		if err := json.Unmarshal(data, &col); err != nil {
			continue
		}

		log.Printf("RECOVERED: loaded recipes.json from version %s", v.VersionID)
		return &col, "", nil
	}

	return nil, "", fmt.Errorf("recipes.json corrupt and no recoverable version found")
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./test/ -run "TestS3Store" -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add internal/storage/storage.go internal/storage/s3store.go test/storage_test.go
git commit -m "feat: add Store interface and S3Store with ETag concurrency

Store interface with ReadAll/WriteAll. S3Store uses conditional puts
via If-Match for optimistic concurrency. Corruption recovery reads
previous S3 object version. Logs warning if file exceeds 1MB."
```

---

### Task 3: S3 Client Adapter

Create the real AWS SDK v2 adapter that implements `S3API`. This is a thin wrapper — all logic lives in `S3Store`. Tested indirectly via integration tests (not unit tested — it's just SDK calls).

**Files:**
- Create: `internal/storage/s3client.go`
- Modify: `go.mod` — add AWS SDK v2

**Step 1: Add AWS SDK v2 dependency**

Run:
```bash
go get github.com/aws/aws-sdk-go-v2
go get github.com/aws/aws-sdk-go-v2/config
go get github.com/aws/aws-sdk-go-v2/service/s3
go get github.com/aws/aws-sdk-go-v2/service/s3/types
```

**Step 2: Create the S3 client adapter**

Create `internal/storage/s3client.go`:

```go
package storage

import (
	"bytes"
	"context"
	"errors"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3Client wraps the AWS SDK v2 S3 client to implement S3API.
type S3Client struct {
	client *s3.Client
}

// NewS3Client creates an S3Client from an AWS SDK v2 S3 client.
func NewS3Client(client *s3.Client) *S3Client {
	return &S3Client{client: client}
}

func (c *S3Client) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, string, error) {
	out, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, "", ErrNotFound
		}
		return nil, "", err
	}

	etag := ""
	if out.ETag != nil {
		etag = *out.ETag
	}
	return out.Body, etag, nil
}

func (c *S3Client) PutObject(ctx context.Context, bucket, key string, data []byte, ifMatch string) error {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/json"),
	}
	if ifMatch != "" {
		input.IfMatch = aws.String(ifMatch)
	}

	_, err := c.client.PutObject(ctx, input)
	if err != nil {
		// Check for PreconditionFailed (ETag mismatch).
		var pf *types.NotFound
		if errors.As(err, &pf) {
			return ErrConflict
		}
		// Also check the error message for precondition failed.
		if isETagMismatch(err) {
			return ErrConflict
		}
		return err
	}
	return nil
}

func (c *S3Client) GetObjectVersion(ctx context.Context, bucket, key, versionID string) (io.ReadCloser, error) {
	out, err := c.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket:    aws.String(bucket),
		Key:       aws.String(key),
		VersionId: aws.String(versionID),
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

func (c *S3Client) ListObjectVersions(ctx context.Context, bucket, key string, maxKeys int) ([]ObjectVersion, error) {
	out, err := c.client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{
		Bucket:  aws.String(bucket),
		Prefix:  aws.String(key),
		MaxKeys: aws.Int32(int32(maxKeys)),
	})
	if err != nil {
		return nil, err
	}

	var versions []ObjectVersion
	for _, v := range out.Versions {
		if v.Key != nil && *v.Key == key {
			versions = append(versions, ObjectVersion{
				VersionID: *v.VersionId,
				IsLatest:  v.IsLatest != nil && *v.IsLatest,
			})
		}
	}
	return versions, nil
}

// isETagMismatch checks for S3 PreconditionFailed errors.
func isETagMismatch(err error) bool {
	// The AWS SDK may return this as an API error with code "PreconditionFailed".
	var apiErr interface{ ErrorCode() string }
	if errors.As(err, &apiErr) && apiErr.ErrorCode() == "PreconditionFailed" {
		return true
	}
	return false
}
```

**Step 3: Verify build**

Run: `go build ./...`
Expected: SUCCESS

**Step 4: Run all tests**

Run: `go test ./...`
Expected: All PASS (including previous tests)

**Step 5: Commit**

```bash
git add internal/storage/s3client.go go.mod go.sum
git commit -m "feat: add S3 client adapter for AWS SDK v2

Thin wrapper around aws-sdk-go-v2 S3 client implementing S3API interface.
Maps NoSuchKey to ErrNotFound and PreconditionFailed to ErrConflict."
```

---

### Task 4: Ingredient Merge

Cross-recipe ingredient merging with synonym matching, unit conversion, quantity addition, and source tracking. Builds on existing `deduplicate.go` patterns.

**Files:**
- Create: `internal/ingredients/merge.go`
- Create: `test/merge_test.go`

**Step 1: Write the failing tests**

Create `test/merge_test.go`:

```go
package test

import (
	"testing"

	"recipe-to-reminders/internal/ingredients"
	"recipe-to-reminders/internal/models"
)

func TestMergeIngredients_SameNameSameUnit(t *testing.T) {
	inputs := []ingredients.RecipeIngredients{
		{RecipeName: "Recipe A", Ingredients: []models.Ingredient{
			{Name: "butter", Quantity: "2", Unit: "tbsp", Category: "dairy"},
		}},
		{RecipeName: "Recipe B", Ingredients: []models.Ingredient{
			{Name: "butter", Quantity: "1.5", Unit: "tbsp", Category: "dairy"},
		}},
	}

	result := ingredients.MergeIngredients(inputs)

	if len(result) != 1 {
		t.Fatalf("got %d items, want 1", len(result))
	}
	if result[0].Name != "butter" {
		t.Errorf("name = %q, want butter", result[0].Name)
	}
	if result[0].Quantity != "3.5" {
		t.Errorf("quantity = %q, want 3.5", result[0].Quantity)
	}
	if result[0].Unit != "tbsp" {
		t.Errorf("unit = %q, want tbsp", result[0].Unit)
	}
	if len(result[0].Sources) != 2 {
		t.Fatalf("sources len = %d, want 2", len(result[0].Sources))
	}
}

func TestMergeIngredients_SynonymMatching(t *testing.T) {
	inputs := []ingredients.RecipeIngredients{
		{RecipeName: "A", Ingredients: []models.Ingredient{
			{Name: "unsalted butter", Quantity: "2", Unit: "tbsp", Category: "dairy"},
		}},
		{RecipeName: "B", Ingredients: []models.Ingredient{
			{Name: "butter", Quantity: "1", Unit: "tbsp", Category: "dairy"},
		}},
	}

	result := ingredients.MergeIngredients(inputs)

	if len(result) != 1 {
		t.Fatalf("got %d items, want 1 (synonyms should merge)", len(result))
	}
	if result[0].Quantity != "3" {
		t.Errorf("quantity = %q, want 3", result[0].Quantity)
	}
}

func TestMergeIngredients_IncompatibleUnits(t *testing.T) {
	inputs := []ingredients.RecipeIngredients{
		{RecipeName: "A", Ingredients: []models.Ingredient{
			{Name: "garlic", Quantity: "2", Unit: "cloves", Category: "produce"},
		}},
		{RecipeName: "B", Ingredients: []models.Ingredient{
			{Name: "garlic powder", Quantity: "1", Unit: "tsp", Category: "spices"},
		}},
	}

	result := ingredients.MergeIngredients(inputs)

	if len(result) != 2 {
		t.Fatalf("got %d items, want 2 (incompatible units stay separate)", len(result))
	}
}

func TestMergeIngredients_SourceTracking(t *testing.T) {
	inputs := []ingredients.RecipeIngredients{
		{RecipeName: "Beef Stew", Ingredients: []models.Ingredient{
			{Name: "flour", Quantity: "2", Unit: "cups", Category: "pantry"},
		}},
		{RecipeName: "Pancakes", Ingredients: []models.Ingredient{
			{Name: "flour", Quantity: "1.5", Unit: "cups", Category: "pantry"},
		}},
	}

	result := ingredients.MergeIngredients(inputs)

	if len(result) != 1 {
		t.Fatalf("got %d items, want 1", len(result))
	}
	if len(result[0].Sources) != 2 {
		t.Fatalf("sources len = %d, want 2", len(result[0].Sources))
	}
	// Sources should include recipe name and original quantity
	found := false
	for _, s := range result[0].Sources {
		if s == "Beef Stew (2 cups)" {
			found = true
		}
	}
	if !found {
		t.Errorf("sources %v missing 'Beef Stew (2 cups)'", result[0].Sources)
	}
}

func TestMergeIngredients_SingleRecipe(t *testing.T) {
	inputs := []ingredients.RecipeIngredients{
		{RecipeName: "Solo", Ingredients: []models.Ingredient{
			{Name: "salt", Quantity: "1", Unit: "tsp", Category: "spices"},
			{Name: "pepper", Quantity: "1", Unit: "tsp", Category: "spices"},
		}},
	}

	result := ingredients.MergeIngredients(inputs)

	if len(result) != 2 {
		t.Fatalf("got %d items, want 2", len(result))
	}
	// Single recipe — source should still be tracked
	if len(result[0].Sources) != 1 {
		t.Errorf("sources len = %d, want 1", len(result[0].Sources))
	}
}

func TestMergeIngredients_EmptyInputs(t *testing.T) {
	result := ingredients.MergeIngredients(nil)
	if len(result) != 0 {
		t.Errorf("got %d items, want 0 for nil input", len(result))
	}
}

func TestMergeIngredients_NoQuantity(t *testing.T) {
	inputs := []ingredients.RecipeIngredients{
		{RecipeName: "A", Ingredients: []models.Ingredient{
			{Name: "salt", Category: "spices"},
		}},
		{RecipeName: "B", Ingredients: []models.Ingredient{
			{Name: "salt", Category: "spices"},
		}},
	}

	result := ingredients.MergeIngredients(inputs)

	// Same name, no quantities — should merge into one item
	if len(result) != 1 {
		t.Fatalf("got %d items, want 1", len(result))
	}
	if len(result[0].Sources) != 2 {
		t.Errorf("sources len = %d, want 2", len(result[0].Sources))
	}
}

func TestMergeIngredients_PreservesCategory(t *testing.T) {
	inputs := []ingredients.RecipeIngredients{
		{RecipeName: "A", Ingredients: []models.Ingredient{
			{Name: "flour", Quantity: "1", Unit: "cups", Category: "pantry"},
		}},
	}

	result := ingredients.MergeIngredients(inputs)

	if len(result) != 1 {
		t.Fatalf("got %d items, want 1", len(result))
	}
	if result[0].Category != "pantry" {
		t.Errorf("category = %q, want pantry", result[0].Category)
	}
}

func TestMergeIngredients_ThreeRecipes(t *testing.T) {
	inputs := []ingredients.RecipeIngredients{
		{RecipeName: "A", Ingredients: []models.Ingredient{
			{Name: "flour", Quantity: "1", Unit: "cups", Category: "pantry"},
		}},
		{RecipeName: "B", Ingredients: []models.Ingredient{
			{Name: "flour", Quantity: "2", Unit: "cups", Category: "pantry"},
		}},
		{RecipeName: "C", Ingredients: []models.Ingredient{
			{Name: "flour", Quantity: "0.5", Unit: "cups", Category: "pantry"},
		}},
	}

	result := ingredients.MergeIngredients(inputs)

	if len(result) != 1 {
		t.Fatalf("got %d items, want 1", len(result))
	}
	if result[0].Quantity != "3.5" {
		t.Errorf("quantity = %q, want 3.5", result[0].Quantity)
	}
	if len(result[0].Sources) != 3 {
		t.Errorf("sources len = %d, want 3", len(result[0].Sources))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./test/ -run "TestMergeIngredients" -v`
Expected: FAIL — `ingredients.MergeIngredients` undefined

**Step 3: Write minimal implementation**

Create `internal/ingredients/merge.go`:

```go
package ingredients

import (
	"fmt"
	"strings"

	"recipe-to-reminders/internal/models"
)

// RecipeIngredients pairs a recipe name with its ingredients for merge tracking.
type RecipeIngredients struct {
	RecipeName  string
	Ingredients []models.Ingredient
}

// synonymMap maps ingredient name variations to a canonical form.
var synonymMap = map[string]string{
	"unsalted butter": "butter",
	"salted butter":   "butter",
	"whole milk":      "milk",
	"2% milk":         "milk",
	"skim milk":       "milk",
	"kosher salt":     "salt",
	"sea salt":        "salt",
	"table salt":      "salt",
	"extra-virgin olive oil": "olive oil",
	"extra virgin olive oil": "olive oil",
	"light brown sugar":      "brown sugar",
	"dark brown sugar":       "brown sugar",
	"packed brown sugar":     "brown sugar",
	"granulated sugar":       "sugar",
	"white sugar":            "sugar",
	"ap flour":               "all-purpose flour",
	"plain flour":            "all-purpose flour",
}

// MergeIngredients combines ingredients from multiple recipes.
// Same name + same unit → add quantities. Incompatible → keep separate.
// Source tracking shows which recipes contributed to each item.
func MergeIngredients(inputs []RecipeIngredients) []models.MergedIngredient {
	if len(inputs) == 0 {
		return nil
	}

	type mergeKey struct {
		name string
		unit string
	}

	type mergeEntry struct {
		ingredient models.MergedIngredient
		key        mergeKey
	}

	seen := make(map[mergeKey]int) // key → index in result
	var result []mergeEntry

	for _, ri := range inputs {
		for _, ing := range ri.Ingredients {
			canonName := canonicalizeName(ing.Name)
			k := mergeKey{name: canonName, unit: strings.ToLower(ing.Unit)}

			source := formatSource(ri.RecipeName, ing.Quantity, ing.Unit)

			if idx, ok := seen[k]; ok {
				// Merge quantities
				entry := &result[idx]
				entry.ingredient.Quantity = addQuantities(entry.ingredient.Quantity, ing.Quantity)
				entry.ingredient.Sources = append(entry.ingredient.Sources, source)
			} else {
				seen[k] = len(result)
				result = append(result, mergeEntry{
					key: k,
					ingredient: models.MergedIngredient{
						Name:     canonName,
						Quantity: ing.Quantity,
						Unit:     ing.Unit,
						Category: ing.Category,
						Sources:  []string{source},
					},
				})
			}
		}
	}

	merged := make([]models.MergedIngredient, len(result))
	for i, e := range result {
		merged[i] = e.ingredient
	}
	return merged
}

// canonicalizeName normalizes an ingredient name using the synonym map.
func canonicalizeName(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	if canonical, ok := synonymMap[lower]; ok {
		return canonical
	}
	return lower
}

// formatSource creates a source string like "Recipe Name (2 tbsp)".
func formatSource(recipeName, qty, unit string) string {
	if qty == "" && unit == "" {
		return recipeName
	}
	if unit == "" {
		return fmt.Sprintf("%s (%s)", recipeName, qty)
	}
	return fmt.Sprintf("%s (%s %s)", recipeName, qty, unit)
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./test/ -run "TestMergeIngredients" -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add internal/ingredients/merge.go test/merge_test.go
git commit -m "feat: add cross-recipe ingredient merging with source tracking

MergeIngredients combines ingredients from multiple recipes. Same name
and unit → add quantities. Synonym map normalizes common variations
(unsalted butter = butter, etc). Source tracking shows which recipes
contributed to each merged item."
```

---

### Task 5: Input Validation + Recipe ID Generation

Shared validation functions and ID generation used by the recipe handlers.

**Files:**
- Create: `internal/handler/validate.go`
- Create: `test/validate_test.go`

**Step 1: Write the failing tests**

Create `test/validate_test.go`:

```go
package test

import (
	"testing"

	"recipe-to-reminders/internal/handler"
)

func TestGenerateRecipeID(t *testing.T) {
	id := handler.GenerateRecipeID("Classic Beef Stew")
	if len(id) < 8 {
		t.Errorf("id %q too short", id)
	}
	// Should end with -6hexchars
	parts := splitLast(id, "-")
	if len(parts[1]) != 6 {
		t.Errorf("hex suffix %q should be 6 chars", parts[1])
	}
	// Should start with slug
	if parts[0] != "classic-beef-stew" {
		t.Errorf("slug = %q, want classic-beef-stew", parts[0])
	}
}

func TestGenerateRecipeID_SpecialChars(t *testing.T) {
	id := handler.GenerateRecipeID("Mom's Best Mac & Cheese!!!")
	// Should be alphanumeric + hyphens only
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			t.Errorf("invalid char %q in ID %q", string(c), id)
		}
	}
}

func TestGenerateRecipeID_EmptyName(t *testing.T) {
	id := handler.GenerateRecipeID("")
	if len(id) < 7 { // at least "recipe-" + 6 hex
		t.Errorf("id %q too short for empty name", id)
	}
}

func TestValidateRecipeID_Valid(t *testing.T) {
	valid := []string{
		"beef-stew-abc123",
		"a-123456",
		"my-recipe-name-abcdef",
		"x-000000",
	}
	for _, id := range valid {
		if err := handler.ValidateRecipeID(id); err != nil {
			t.Errorf("ID %q should be valid, got: %v", id, err)
		}
	}
}

func TestValidateRecipeID_Invalid(t *testing.T) {
	invalid := []string{
		"",
		"abc",                // no hex suffix
		"-abc123",            // starts with hyphen
		"ABC-abc123",         // uppercase
		"../../etc-abc123",   // path traversal
		"test/path-abc123",   // slash
		"test\\path-abc123",  // backslash
		"a-ABCDEF",           // uppercase hex
		"a-abcde",            // 5 hex chars
		"a-abcdefg",          // 7 hex chars
	}
	for _, id := range invalid {
		if err := handler.ValidateRecipeID(id); err == nil {
			t.Errorf("ID %q should be rejected", id)
		}
	}
}

func TestSanitizeString(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"hello", 100, "hello"},
		{"  spaces  ", 100, "spaces"},
		{"null\x00byte", 100, "nullbyte"},
		{"control\x01char", 100, "controlchar"},
		{"tabs\tok", 100, "tabs\tok"},          // tabs preserved
		{"newlines\nok", 100, "newlines\nok"},  // newlines preserved
		{"toolong", 4, "tool"},
	}
	for _, tt := range tests {
		got := handler.SanitizeString(tt.input, tt.max)
		if got != tt.want {
			t.Errorf("SanitizeString(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
		}
	}
}

func TestValidateSaveRecipeRequest(t *testing.T) {
	// Valid request
	err := handler.ValidateSaveRecipeRequest("Recipe", nil, nil, 0)
	if err != nil {
		t.Errorf("valid request got error: %v", err)
	}

	// Empty name
	err = handler.ValidateSaveRecipeRequest("", nil, nil, 0)
	if err == nil {
		t.Error("empty name should be rejected")
	}
}

// splitLast splits s on the last occurrence of sep.
func splitLast(s, sep string) [2]string {
	for i := len(s) - 1; i >= 0; i-- {
		if string(s[i]) == sep {
			return [2]string{s[:i], s[i+1:]}
		}
	}
	return [2]string{s, ""}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./test/ -run "TestGenerateRecipeID|TestValidateRecipeID|TestSanitizeString|TestValidateSaveRecipeRequest" -v`
Expected: FAIL — `handler.GenerateRecipeID` undefined

**Step 3: Write minimal implementation**

Create `internal/handler/validate.go`:

```go
package handler

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"recipe-to-reminders/internal/models"
)

const (
	maxRecipes           = 200
	maxIngredientsPerRec = 100
	maxRecipeName        = 200
	maxTagLen            = 50
	maxTagsPerRecipe     = 20
	maxIngredientRaw     = 500
	maxIngredientName    = 200
	maxShopRecipeIDs     = 20
)

var recipeIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*-[a-f0-9]{6}$`)

// ValidateRecipeID checks that a recipe ID matches the expected format.
func ValidateRecipeID(id string) error {
	if strings.Contains(id, "..") || strings.Contains(id, "/") || strings.Contains(id, "\\") {
		return fmt.Errorf("invalid recipe ID: contains forbidden characters")
	}
	if !recipeIDPattern.MatchString(id) {
		return fmt.Errorf("invalid recipe ID format")
	}
	return nil
}

// GenerateRecipeID creates a slug-6hex ID from a recipe name.
func GenerateRecipeID(name string) string {
	slug := slugify(name)
	if slug == "" {
		slug = "recipe"
	}
	hex := randomHex(6)
	return slug + "-" + hex
}

// SanitizeString removes null bytes and control chars, validates UTF-8,
// trims whitespace, and enforces a max length.
func SanitizeString(s string, maxLen int) string {
	// Validate UTF-8
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}

	// Remove null bytes and control characters (keep \n, \t)
	var b strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			b.WriteRune(r)
		}
	}
	s = strings.TrimSpace(b.String())

	// Enforce length
	if len(s) > maxLen {
		s = s[:maxLen]
	}
	return s
}

// ValidateSaveRecipeRequest validates the fields of a save/update request.
func ValidateSaveRecipeRequest(name string, tags []string, ingredients []models.Ingredient, existingCount int) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("recipe name is required")
	}
	if len(name) > maxRecipeName {
		return fmt.Errorf("recipe name exceeds %d chars", maxRecipeName)
	}
	if len(tags) > maxTagsPerRecipe {
		return fmt.Errorf("too many tags (max %d)", maxTagsPerRecipe)
	}
	for _, tag := range tags {
		if len(tag) > maxTagLen {
			return fmt.Errorf("tag exceeds %d chars", maxTagLen)
		}
	}
	if len(ingredients) > maxIngredientsPerRec {
		return fmt.Errorf("too many ingredients (max %d)", maxIngredientsPerRec)
	}
	for _, ing := range ingredients {
		if len(ing.Raw) > maxIngredientRaw {
			return fmt.Errorf("ingredient raw field exceeds %d chars", maxIngredientRaw)
		}
		if len(ing.Name) > maxIngredientName {
			return fmt.Errorf("ingredient name field exceeds %d chars", maxIngredientName)
		}
	}
	return nil
}

// ValidateShopRequest validates a shopping list request.
func ValidateShopRequest(ids []string) error {
	if len(ids) == 0 {
		return fmt.Errorf("recipe_ids is required")
	}
	if len(ids) > maxShopRecipeIDs {
		return fmt.Errorf("too many recipe_ids (max %d)", maxShopRecipeIDs)
	}
	for _, id := range ids {
		if err := ValidateRecipeID(id); err != nil {
			return fmt.Errorf("invalid recipe_id %q: %w", id, err)
		}
	}
	return nil
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevHyphen := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevHyphen = false
		} else if r == ' ' || r == '-' || r == '_' {
			if !prevHyphen && b.Len() > 0 {
				b.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	result := b.String()
	// Trim trailing hyphen
	return strings.TrimRight(result, "-")
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)[:n]
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./test/ -run "TestGenerateRecipeID|TestValidateRecipeID|TestSanitizeString|TestValidateSaveRecipeRequest" -v`
Expected: All PASS

**Step 5: Commit**

```bash
git add internal/handler/validate.go test/validate_test.go
git commit -m "feat: add recipe ID generation, validation, and input sanitization

GenerateRecipeID creates slug-6hex IDs. ValidateRecipeID enforces
^[a-z0-9][a-z0-9-]*-[a-f0-9]{6}$ and rejects path traversal.
SanitizeString strips null bytes, control chars, validates UTF-8.
ValidateSaveRecipeRequest enforces field limits from spec."
```

---

### Task 6: Recipe CRUD Handlers

All 6 recipe endpoints: list, get, save, update, delete, shop. Uses read-modify-write retry loop for mutations. Handler tests use a MockStore.

**Files:**
- Create: `internal/handler/recipes.go`
- Create: `test/recipes_test.go`
- Modify: `internal/handler/handler.go` — add store field, register recipe routes

**Step 1: Write the failing tests**

Create `test/recipes_test.go`:

```go
package test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"recipe-to-reminders/internal/handler"
	"recipe-to-reminders/internal/models"
	"recipe-to-reminders/internal/storage"
)

// MockStore implements storage.Store for testing.
type MockStore struct {
	Collection *models.RecipeCollection
	ETag       string
	ReadErr    error
	WriteErr   error
	WriteCalls int
}

func (m *MockStore) ReadAll(_ context.Context) (*models.RecipeCollection, string, error) {
	if m.ReadErr != nil {
		return nil, "", m.ReadErr
	}
	if m.Collection == nil {
		return &models.RecipeCollection{Version: 1, Recipes: []models.SavedRecipe{}}, "", nil
	}
	return m.Collection, m.ETag, nil
}

func (m *MockStore) WriteAll(_ context.Context, col *models.RecipeCollection, _ string) error {
	m.WriteCalls++
	if m.WriteErr != nil {
		return m.WriteErr
	}
	m.Collection = col
	return nil
}

func newTestHandler(store storage.Store) http.Handler {
	return handler.New(nil, handler.WithStore(store))
}

func TestListRecipes_Empty(t *testing.T) {
	h := newTestHandler(&MockStore{})
	req := httptest.NewRequest(http.MethodGet, "/recipes", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var resp struct {
		Recipes []models.RecipeSummary `json:"recipes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Recipes) != 0 {
		t.Errorf("recipes len = %d, want 0", len(resp.Recipes))
	}
}

func TestListRecipes_WithRecipes(t *testing.T) {
	store := &MockStore{
		Collection: &models.RecipeCollection{
			Version: 1,
			Recipes: []models.SavedRecipe{
				{ID: "stew-abc123", Name: "Stew", Tags: []string{"dinner"}, Ingredients: make([]models.Ingredient, 5)},
				{ID: "cake-def456", Name: "Cake", Tags: []string{"dessert"}, Ingredients: make([]models.Ingredient, 8)},
			},
		},
		ETag: "etag-1",
	}
	h := newTestHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/recipes", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var resp struct {
		Recipes []models.RecipeSummary `json:"recipes"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.Recipes) != 2 {
		t.Fatalf("recipes len = %d, want 2", len(resp.Recipes))
	}
	if resp.Recipes[0].IngredientCount != 5 {
		t.Errorf("ingredient_count = %d, want 5", resp.Recipes[0].IngredientCount)
	}
}

func TestGetRecipe_Found(t *testing.T) {
	store := &MockStore{
		Collection: &models.RecipeCollection{
			Version: 1,
			Recipes: []models.SavedRecipe{
				{ID: "stew-abc123", Name: "Stew", Ingredients: []models.Ingredient{
					{Name: "beef", Quantity: "2", Unit: "lbs"},
				}},
			},
		},
	}
	h := newTestHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/recipes/stew-abc123", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. body: %s", rr.Code, rr.Body.String())
	}

	var recipe models.SavedRecipe
	_ = json.Unmarshal(rr.Body.Bytes(), &recipe)
	if recipe.Name != "Stew" {
		t.Errorf("name = %q, want Stew", recipe.Name)
	}
}

func TestGetRecipe_NotFound(t *testing.T) {
	h := newTestHandler(&MockStore{})
	req := httptest.NewRequest(http.MethodGet, "/recipes/nonexistent-abc123", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestGetRecipe_InvalidID(t *testing.T) {
	h := newTestHandler(&MockStore{})
	req := httptest.NewRequest(http.MethodGet, "/recipes/../../etc", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestSaveRecipe_Success(t *testing.T) {
	store := &MockStore{}
	h := newTestHandler(store)

	body, _ := json.Marshal(models.SaveRecipeRequest{
		Name:   "New Recipe",
		Source: "example.com",
		Tags:   []string{"dinner"},
		Ingredients: []models.Ingredient{
			{Name: "flour", Quantity: "2", Unit: "cups", Raw: "2 cups flour"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/recipes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201. body: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.ID == "" {
		t.Error("expected non-empty ID")
	}
	if store.WriteCalls != 1 {
		t.Errorf("write calls = %d, want 1", store.WriteCalls)
	}
}

func TestSaveRecipe_EmptyName(t *testing.T) {
	h := newTestHandler(&MockStore{})
	body, _ := json.Marshal(models.SaveRecipeRequest{Name: ""})
	req := httptest.NewRequest(http.MethodPost, "/recipes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestSaveRecipe_AtCapacity(t *testing.T) {
	recipes := make([]models.SavedRecipe, 200)
	for i := range recipes {
		recipes[i] = models.SavedRecipe{ID: handler.GenerateRecipeID("r")}
	}
	store := &MockStore{
		Collection: &models.RecipeCollection{Version: 1, Recipes: recipes},
	}
	h := newTestHandler(store)

	body, _ := json.Marshal(models.SaveRecipeRequest{Name: "One Too Many"})
	req := httptest.NewRequest(http.MethodPost, "/recipes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rr.Code)
	}
}

func TestUpdateRecipe_Success(t *testing.T) {
	store := &MockStore{
		Collection: &models.RecipeCollection{
			Version: 1,
			Recipes: []models.SavedRecipe{
				{ID: "stew-abc123", Name: "Stew", Tags: []string{"dinner"}},
			},
		},
	}
	h := newTestHandler(store)

	body, _ := json.Marshal(map[string]any{
		"name": "Updated Stew",
		"tags": []string{"dinner", "winter"},
	})
	req := httptest.NewRequest(http.MethodPut, "/recipes/stew-abc123", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. body: %s", rr.Code, rr.Body.String())
	}

	// Verify the update was applied
	if store.Collection.Recipes[0].Name != "Updated Stew" {
		t.Errorf("name = %q, want Updated Stew", store.Collection.Recipes[0].Name)
	}
	if len(store.Collection.Recipes[0].Tags) != 2 {
		t.Errorf("tags len = %d, want 2", len(store.Collection.Recipes[0].Tags))
	}
}

func TestUpdateRecipe_NotFound(t *testing.T) {
	h := newTestHandler(&MockStore{})
	body, _ := json.Marshal(map[string]any{"name": "X"})
	req := httptest.NewRequest(http.MethodPut, "/recipes/nonexistent-abc123", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestDeleteRecipe_Success(t *testing.T) {
	store := &MockStore{
		Collection: &models.RecipeCollection{
			Version: 1,
			Recipes: []models.SavedRecipe{
				{ID: "stew-abc123", Name: "Stew"},
			},
		},
	}
	h := newTestHandler(store)

	req := httptest.NewRequest(http.MethodDelete, "/recipes/stew-abc123", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. body: %s", rr.Code, rr.Body.String())
	}
	if len(store.Collection.Recipes) != 0 {
		t.Errorf("recipes len = %d, want 0 after delete", len(store.Collection.Recipes))
	}
}

func TestDeleteRecipe_NotFound(t *testing.T) {
	h := newTestHandler(&MockStore{})
	req := httptest.NewRequest(http.MethodDelete, "/recipes/nonexistent-abc123", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestShopRecipes_Success(t *testing.T) {
	store := &MockStore{
		Collection: &models.RecipeCollection{
			Version: 1,
			Recipes: []models.SavedRecipe{
				{ID: "a-abc123", Name: "Recipe A", Ingredients: []models.Ingredient{
					{Name: "flour", Quantity: "2", Unit: "cups", Category: "pantry"},
					{Name: "butter", Quantity: "1", Unit: "tbsp", Category: "dairy"},
				}},
				{ID: "b-def456", Name: "Recipe B", Ingredients: []models.Ingredient{
					{Name: "flour", Quantity: "1", Unit: "cups", Category: "pantry"},
					{Name: "sugar", Quantity: "1", Unit: "cups", Category: "pantry"},
				}},
			},
		},
	}
	h := newTestHandler(store)

	body, _ := json.Marshal(models.ShopRequest{
		RecipeIDs:   []string{"a-abc123", "b-def456"},
		Deduplicate: true,
	})
	req := httptest.NewRequest(http.MethodPost, "/recipes/shop", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. body: %s", rr.Code, rr.Body.String())
	}

	var resp models.ShopResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.RecipesIncluded) != 2 {
		t.Errorf("recipes_included len = %d, want 2", len(resp.RecipesIncluded))
	}
	// flour should be merged: 2 + 1 = 3 cups
	for _, ing := range resp.Ingredients {
		if ing.Name == "flour" && ing.Quantity != "3" {
			t.Errorf("flour quantity = %q, want 3", ing.Quantity)
		}
	}
}

func TestShopRecipes_EmptyIDs(t *testing.T) {
	h := newTestHandler(&MockStore{})
	body, _ := json.Marshal(models.ShopRequest{RecipeIDs: []string{}})
	req := httptest.NewRequest(http.MethodPost, "/recipes/shop", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestShopRecipes_RecipeNotFound(t *testing.T) {
	h := newTestHandler(&MockStore{})
	body, _ := json.Marshal(models.ShopRequest{RecipeIDs: []string{"nonexistent-abc123"}})
	req := httptest.NewRequest(http.MethodPost, "/recipes/shop", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestShopRecipes_NoDeduplicate(t *testing.T) {
	store := &MockStore{
		Collection: &models.RecipeCollection{
			Version: 1,
			Recipes: []models.SavedRecipe{
				{ID: "a-abc123", Name: "A", Ingredients: []models.Ingredient{
					{Name: "flour", Quantity: "2", Unit: "cups", Category: "pantry"},
				}},
				{ID: "b-def456", Name: "B", Ingredients: []models.Ingredient{
					{Name: "flour", Quantity: "1", Unit: "cups", Category: "pantry"},
				}},
			},
		},
	}
	h := newTestHandler(store)

	body, _ := json.Marshal(models.ShopRequest{
		RecipeIDs:   []string{"a-abc123", "b-def456"},
		Deduplicate: false,
	})
	req := httptest.NewRequest(http.MethodPost, "/recipes/shop", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}

	var resp models.ShopResponse
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	// Without dedup, flour appears twice
	flourCount := 0
	for _, ing := range resp.Ingredients {
		if ing.Name == "flour" {
			flourCount++
		}
	}
	if flourCount != 2 {
		t.Errorf("flour count = %d, want 2 (no dedup)", flourCount)
	}
}

func TestRecipes_NoStore(t *testing.T) {
	// handler.New(nil) — no store configured
	h := handler.New(nil)
	req := httptest.NewRequest(http.MethodGet, "/recipes", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when store not configured", rr.Code)
	}
}

func TestRecipes_StoreReadError(t *testing.T) {
	store := &MockStore{ReadErr: fmt.Errorf("s3 unavailable")}
	h := newTestHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/recipes", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rr.Code)
	}
}

func TestRecipes_WriteConflict(t *testing.T) {
	store := &MockStore{WriteErr: storage.ErrConflict}
	h := newTestHandler(store)

	body, _ := json.Marshal(models.SaveRecipeRequest{Name: "Test"})
	req := httptest.NewRequest(http.MethodPost, "/recipes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rr.Code)
	}
}
```

Note: `test/recipes_test.go` needs a `"fmt"` import for `TestRecipes_StoreReadError`. Add it to the imports.

**Step 2: Run test to verify it fails**

Run: `go test ./test/ -run "TestListRecipes|TestGetRecipe|TestSaveRecipe|TestUpdateRecipe|TestDeleteRecipe|TestShopRecipes|TestRecipes_" -v`
Expected: FAIL — `handler.WithStore` undefined

**Step 3: Modify handler.go to accept Store**

Update `internal/handler/handler.go`:

```go
package handler

import (
	"encoding/json"
	"net/http"

	"recipe-to-reminders/internal/models"
	"recipe-to-reminders/internal/parser"
	"recipe-to-reminders/internal/storage"
)

// Handler routes HTTP requests to the appropriate endpoint handler.
type Handler struct {
	mux       *http.ServeMux
	extractor *parser.Extractor
	store     storage.Store
}

// Option configures the Handler.
type Option func(*Handler)

// WithStore sets the recipe storage backend.
func WithStore(s storage.Store) parser.ExtractorOption {
	// We need a different approach — use a handler option.
	// This is a no-op ExtractorOption; the store is set via handlerOpts.
	return nil
}
```

**Wait** — this won't work cleanly. The existing `handler.New` signature uses `parser.ExtractorOption`. We need a separate option type for handler-level config. Let's use a cleaner approach.

Update `internal/handler/handler.go` to this:

```go
package handler

import (
	"encoding/json"
	"net/http"

	"recipe-to-reminders/internal/models"
	"recipe-to-reminders/internal/parser"
	"recipe-to-reminders/internal/storage"
)

// Handler routes HTTP requests to the appropriate endpoint handler.
type Handler struct {
	mux       *http.ServeMux
	extractor *parser.Extractor
	store     storage.Store
}

// HandlerOption configures the Handler.
type HandlerOption func(*Handler)

// WithStore sets the recipe storage backend.
func WithStore(s storage.Store) HandlerOption {
	return func(h *Handler) {
		h.store = s
	}
}

// New creates a Handler wired to a Fetcher for URL extraction.
// HandlerOptions (like WithStore) are passed first, followed by ExtractorOptions.
func New(fetcher *parser.Fetcher, handlerOpts []HandlerOption, extractorOpts ...parser.ExtractorOption) *Handler {
	h := &Handler{
		mux:       http.NewServeMux(),
		extractor: parser.NewExtractor(fetcher, extractorOpts...),
	}

	for _, opt := range handlerOpts {
		if opt != nil {
			opt(h)
		}
	}

	h.mux.HandleFunc("POST /extract", h.handleExtract)
	h.mux.HandleFunc("/extract", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for /extract")
	})

	// Recipe routes (only if store is configured)
	h.mux.HandleFunc("GET /recipes", h.handleListRecipes)
	h.mux.HandleFunc("POST /recipes", h.handleSaveRecipe)
	h.mux.HandleFunc("GET /recipes/{id}", h.handleGetRecipe)
	h.mux.HandleFunc("PUT /recipes/{id}", h.handleUpdateRecipe)
	h.mux.HandleFunc("DELETE /recipes/{id}", h.handleDeleteRecipe)
	h.mux.HandleFunc("POST /recipes/shop", h.handleShopRecipes)

	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, models.ErrorResponse{Error: code, Message: message})
}
```

**Important:** This changes the `New` function signature from `New(fetcher, opts ...ExtractorOption)` to `New(fetcher, handlerOpts []HandlerOption, extractorOpts ...ExtractorOption)`. All existing callers must be updated:

- `cmd/lambda/main.go`: `handler.New(fetcher, opts...)` → `handler.New(fetcher, nil, opts...)`
- `test/parser_test.go`: All `handler.New(nil)` → `handler.New(nil, nil)` and `handler.New(parser.NewFetcher(...))` → `handler.New(parser.NewFetcher(...), nil)` and `handler.New(nil, parser.WithOCREngine(...), ...)` → `handler.New(nil, nil, parser.WithOCREngine(...), ...)`

The test helper `newTestHandler` would be:
```go
func newTestHandler(store storage.Store) http.Handler {
	return handler.New(nil, []handler.HandlerOption{handler.WithStore(store)})
}
```

**Step 4: Create recipes.go**

Create `internal/handler/recipes.go`:

```go
package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"recipe-to-reminders/internal/ingredients"
	"recipe-to-reminders/internal/models"
	"recipe-to-reminders/internal/storage"
)

func (h *Handler) requireStore(w http.ResponseWriter) bool {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "storage_unavailable",
			"Recipe storage is not configured.")
		return false
	}
	return true
}

func (h *Handler) handleListRecipes(w http.ResponseWriter, r *http.Request) {
	if !h.requireStore(w) {
		return
	}

	col, _, err := h.store.ReadAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage_error", "Failed to read recipes.")
		return
	}

	summaries := make([]models.RecipeSummary, 0, len(col.Recipes))
	for _, recipe := range col.Recipes {
		summaries = append(summaries, models.RecipeSummary{
			ID:              recipe.ID,
			Name:            recipe.Name,
			Tags:            recipe.Tags,
			IngredientCount: len(recipe.Ingredients),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"recipes": summaries})
}

func (h *Handler) handleGetRecipe(w http.ResponseWriter, r *http.Request) {
	if !h.requireStore(w) {
		return
	}

	id := r.PathValue("id")
	if err := ValidateRecipeID(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	col, _, err := h.store.ReadAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage_error", "Failed to read recipes.")
		return
	}

	for _, recipe := range col.Recipes {
		if recipe.ID == id {
			writeJSON(w, http.StatusOK, recipe)
			return
		}
	}

	writeError(w, http.StatusNotFound, "not_found", "Recipe not found.")
}

func (h *Handler) handleSaveRecipe(w http.ResponseWriter, r *http.Request) {
	if !h.requireStore(w) {
		return
	}

	var req models.SaveRecipeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body.")
		return
	}

	// Sanitize inputs
	req.Name = SanitizeString(req.Name, maxRecipeName)
	req.Source = SanitizeString(req.Source, maxRecipeName)
	req.SourceURL = SanitizeString(req.SourceURL, 2000)
	for i := range req.Tags {
		req.Tags[i] = SanitizeString(req.Tags[i], maxTagLen)
	}

	if err := ValidateSaveRecipeRequest(req.Name, req.Tags, req.Ingredients, 0); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}

	col, etag, err := h.store.ReadAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage_error", "Failed to read recipes.")
		return
	}

	if len(col.Recipes) >= maxRecipes {
		writeError(w, http.StatusConflict, "at_capacity",
			"Maximum number of saved recipes reached (200).")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	recipe := models.SavedRecipe{
		ID:          GenerateRecipeID(req.Name),
		Name:        req.Name,
		Source:      req.Source,
		SourceURL:   req.SourceURL,
		Servings:    req.Servings,
		CreatedAt:   now,
		UpdatedAt:   now,
		Tags:        req.Tags,
		Ingredients: req.Ingredients,
	}

	col.Recipes = append(col.Recipes, recipe)
	col.UpdatedAt = now

	if err := h.store.WriteAll(r.Context(), col, etag); err != nil {
		if errors.Is(err, storage.ErrConflict) {
			writeError(w, http.StatusConflict, "conflict", "Concurrent modification — please retry.")
			return
		}
		writeError(w, http.StatusInternalServerError, "storage_error", "Failed to save recipe.")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"id":      recipe.ID,
		"message": "Recipe saved.",
	})
}

func (h *Handler) handleUpdateRecipe(w http.ResponseWriter, r *http.Request) {
	if !h.requireStore(w) {
		return
	}

	id := r.PathValue("id")
	if err := ValidateRecipeID(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	// Decode as map for partial updates
	var updates map[string]json.RawMessage
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody)).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body.")
		return
	}

	col, etag, err := h.store.ReadAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage_error", "Failed to read recipes.")
		return
	}

	idx := -1
	for i, recipe := range col.Recipes {
		if recipe.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, "not_found", "Recipe not found.")
		return
	}

	recipe := &col.Recipes[idx]

	// Apply partial updates
	if raw, ok := updates["name"]; ok {
		var name string
		if err := json.Unmarshal(raw, &name); err == nil {
			recipe.Name = SanitizeString(name, maxRecipeName)
		}
	}
	if raw, ok := updates["source"]; ok {
		var source string
		if err := json.Unmarshal(raw, &source); err == nil {
			recipe.Source = SanitizeString(source, maxRecipeName)
		}
	}
	if raw, ok := updates["source_url"]; ok {
		var url string
		if err := json.Unmarshal(raw, &url); err == nil {
			recipe.SourceURL = SanitizeString(url, 2000)
		}
	}
	if raw, ok := updates["servings"]; ok {
		var servings string
		if err := json.Unmarshal(raw, &servings); err == nil {
			recipe.Servings = SanitizeString(servings, 50)
		}
	}
	if raw, ok := updates["tags"]; ok {
		var tags []string
		if err := json.Unmarshal(raw, &tags); err == nil {
			for i := range tags {
				tags[i] = SanitizeString(tags[i], maxTagLen)
			}
			recipe.Tags = tags
		}
	}
	if raw, ok := updates["ingredients"]; ok {
		var ings []models.Ingredient
		if err := json.Unmarshal(raw, &ings); err == nil {
			recipe.Ingredients = ings
		}
	}

	// Validate after updates
	if err := ValidateSaveRecipeRequest(recipe.Name, recipe.Tags, recipe.Ingredients, 0); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	recipe.UpdatedAt = now
	col.UpdatedAt = now

	if err := h.store.WriteAll(r.Context(), col, etag); err != nil {
		if errors.Is(err, storage.ErrConflict) {
			writeError(w, http.StatusConflict, "conflict", "Concurrent modification — please retry.")
			return
		}
		writeError(w, http.StatusInternalServerError, "storage_error", "Failed to update recipe.")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"id":      id,
		"message": "Recipe updated.",
	})
}

func (h *Handler) handleDeleteRecipe(w http.ResponseWriter, r *http.Request) {
	if !h.requireStore(w) {
		return
	}

	id := r.PathValue("id")
	if err := ValidateRecipeID(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	col, etag, err := h.store.ReadAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage_error", "Failed to read recipes.")
		return
	}

	idx := -1
	for i, recipe := range col.Recipes {
		if recipe.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, "not_found", "Recipe not found.")
		return
	}

	col.Recipes = append(col.Recipes[:idx], col.Recipes[idx+1:]...)
	col.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := h.store.WriteAll(r.Context(), col, etag); err != nil {
		if errors.Is(err, storage.ErrConflict) {
			writeError(w, http.StatusConflict, "conflict", "Concurrent modification — please retry.")
			return
		}
		writeError(w, http.StatusInternalServerError, "storage_error", "Failed to delete recipe.")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"id":      id,
		"message": "Recipe deleted.",
	})
}

func (h *Handler) handleShopRecipes(w http.ResponseWriter, r *http.Request) {
	if !h.requireStore(w) {
		return
	}

	var req models.ShopRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body.")
		return
	}

	if err := ValidateShopRequest(req.RecipeIDs); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}

	col, _, err := h.store.ReadAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage_error", "Failed to read recipes.")
		return
	}

	// Look up each requested recipe
	recipeMap := make(map[string]*models.SavedRecipe, len(col.Recipes))
	for i := range col.Recipes {
		recipeMap[col.Recipes[i].ID] = &col.Recipes[i]
	}

	var recipeInputs []ingredients.RecipeIngredients
	var recipeNames []string

	for _, id := range req.RecipeIDs {
		recipe, ok := recipeMap[id]
		if !ok {
			writeError(w, http.StatusNotFound, "not_found", "Recipe "+id+" not found.")
			return
		}
		recipeInputs = append(recipeInputs, ingredients.RecipeIngredients{
			RecipeName:  recipe.Name,
			Ingredients: recipe.Ingredients,
		})
		recipeNames = append(recipeNames, recipe.Name)
	}

	if req.Deduplicate {
		merged := ingredients.MergeIngredients(recipeInputs)
		writeJSON(w, http.StatusOK, models.ShopResponse{
			RecipesIncluded: recipeNames,
			Ingredients:     merged,
		})
	} else {
		// No dedup — return all ingredients with source tracking but no merging
		var all []models.MergedIngredient
		for _, ri := range recipeInputs {
			for _, ing := range ri.Ingredients {
				all = append(all, models.MergedIngredient{
					Name:     ing.Name,
					Quantity: ing.Quantity,
					Unit:     ing.Unit,
					Category: ing.Category,
					Sources:  []string{ri.RecipeName},
				})
			}
		}
		writeJSON(w, http.StatusOK, models.ShopResponse{
			RecipesIncluded: recipeNames,
			Ingredients:     all,
		})
	}
}
```

**Step 5: Update existing callers**

Update `cmd/lambda/main.go` line 56:
```go
// Old: h := handler.New(fetcher, opts...)
// New:
h := handler.New(fetcher, nil, opts...)
```

Update all `handler.New(...)` calls in `test/parser_test.go`:
- `handler.New(nil)` → `handler.New(nil, nil)`
- `handler.New(parser.NewFetcher(parser.WithAllowLoopback(true)))` → `handler.New(parser.NewFetcher(parser.WithAllowLoopback(true)), nil)`
- `handler.New(nil, parser.WithOCREngine(mock), parser.WithConfidenceThreshold(0.5))` → `handler.New(nil, nil, parser.WithOCREngine(mock), parser.WithConfidenceThreshold(0.5))`

**Step 6: Run all tests to verify they pass**

Run: `go test ./... -v`
Expected: All PASS (both new recipe tests and existing tests)

**Step 7: Commit**

```bash
git add internal/handler/recipes.go internal/handler/handler.go test/recipes_test.go cmd/lambda/main.go test/parser_test.go
git commit -m "feat: add recipe CRUD handlers and shopping list endpoint

Six new endpoints: GET/POST /recipes, GET/PUT/DELETE /recipes/{id},
POST /recipes/shop. WithStore HandlerOption for dependency injection.
Read-modify-write with ETag conflict detection. Input validation and
sanitization on all mutations. Partial updates for PUT."
```

---

### Task 7: Wire S3 Store in Lambda + Update Existing Tests

Wire the S3 client from environment variables into the handler. Update any remaining test callers.

**Files:**
- Modify: `cmd/lambda/main.go` — create S3 client from env vars, pass to handler

**Step 1: Update main.go**

Add S3 wiring to `cmd/lambda/main.go`:

```go
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"recipe-to-reminders/internal/handler"
	"recipe-to-reminders/internal/parser"
	"recipe-to-reminders/internal/storage"
)

func main() {
	fetcher := parser.NewFetcher()

	var extractorOpts []parser.ExtractorOption

	// Tesseract OCR engine
	psm := os.Getenv("TESSERACT_PSM")
	lang := os.Getenv("TESSERACT_LANG")
	engine, err := parser.NewTesseractEngine(psm, lang)
	if err != nil {
		log.Fatalf("invalid tesseract config: %v", err)
	}
	extractorOpts = append(extractorOpts, parser.WithOCREngine(engine))

	// Photo strategy
	if strategy := os.Getenv("PHOTO_STRATEGY"); strategy != "" {
		extractorOpts = append(extractorOpts, parser.WithPhotoStrategy(strategy))
	}

	// Confidence threshold
	if thresh := os.Getenv("CONFIDENCE_THRESHOLD"); thresh != "" {
		v, err := strconv.ParseFloat(thresh, 64)
		if err != nil || v < 0 || v > 1 {
			log.Fatalf("invalid CONFIDENCE_THRESHOLD %q: must be 0.0-1.0", thresh) // #nosec G706 -- %q quotes the value
		}
		extractorOpts = append(extractorOpts, parser.WithConfidenceThreshold(v))
	}

	// Claude Vision fallback
	if apiKey := os.Getenv("ANTHROPIC_API_KEY"); apiKey != "" {
		maxPerMin := 10
		if v := os.Getenv("CLAUDE_FALLBACK_MAX_PER_MIN"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				log.Fatalf("invalid CLAUDE_FALLBACK_MAX_PER_MIN %q", v) // #nosec G706 -- %q quotes the value
			}
			maxPerMin = n
		}
		claude := parser.NewClaudeExtractor(apiKey, maxPerMin)
		extractorOpts = append(extractorOpts, parser.WithImageExtractor(claude))
	}

	// S3 recipe storage
	var handlerOpts []handler.HandlerOption
	if bucket := os.Getenv("S3_BUCKET"); bucket != "" {
		recipesKey := os.Getenv("S3_RECIPES_KEY")
		if recipesKey == "" {
			recipesKey = "recipes.json"
		}

		cfg, err := config.LoadDefaultConfig(context.Background())
		if err != nil {
			log.Fatalf("failed to load AWS config: %v", err)
		}
		s3Client := storage.NewS3Client(s3.NewFromConfig(cfg))
		store := storage.NewS3Store(s3Client, bucket, recipesKey)
		handlerOpts = append(handlerOpts, handler.WithStore(store))
		log.Printf("Recipe storage: s3://%s/%s", bucket, recipesKey)
	}

	h := handler.New(fetcher, handlerOpts, extractorOpts...)

	addr := ":8080"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("Listening on %s", addr) // #nosec G706 -- addr is from PORT env var, not user input
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
```

**Step 2: Verify build**

Run: `go build ./...`
Expected: SUCCESS

**Step 3: Run all tests**

Run: `go test ./...`
Expected: All PASS

**Step 4: Commit**

```bash
git add cmd/lambda/main.go go.mod go.sum
git commit -m "feat: wire S3 recipe storage from environment variables

S3_BUCKET and S3_RECIPES_KEY env vars configure recipe persistence.
Creates AWS SDK v2 S3 client and passes store via HandlerOption."
```

---

### Task 8: Lint, Vet, Final Verification

Run all quality checks and fix any issues.

**Files:**
- Potentially any file that has lint issues

**Step 1: Run go vet**

Run: `go vet ./...`
Expected: No errors

**Step 2: Run golangci-lint**

Run: `golangci-lint run ./...`
Expected: No errors (fix any that appear)

**Step 3: Run gosec**

Run: `gosec ./...`
Expected: No errors (add `#nosec` annotations for validated false positives)

**Step 4: Run govulncheck**

Run: `govulncheck ./...`
Expected: No vulnerabilities

**Step 5: Run tests with race detector**

Run: `go test -race ./...`
Expected: All PASS, no races

**Step 6: Commit any fixes**

```bash
git add -A
git commit -m "chore: fix lint and vet issues for milestone 3"
```

---

### Task 9: Push and Verify CI

Push all commits and verify CI passes.

**Step 1: Push to remote**

Run: `git push origin main`
Expected: SUCCESS

**Step 2: Check CI status**

Run: `gh run list --limit 1`
Then: `gh run watch` (wait for completion)
Expected: All CI jobs pass (lint, test, gosec, govulncheck)

**Step 3: Close the GitHub issue**

Run: `gh issue close 3 --comment "Milestone 3 complete: Saved Recipes CRUD + S3 storage with multi-recipe shopping list."`

---

## Summary of Changes

| File | Action | Description |
|------|--------|-------------|
| `internal/models/recipe.go` | Modify | Add SavedRecipe, RecipeCollection, RecipeSummary, SaveRecipeRequest, ShopRequest, ShopResponse, MergedIngredient |
| `internal/storage/storage.go` | Create | Store interface, S3API interface, sentinel errors |
| `internal/storage/s3store.go` | Create | S3Store implementation with ETag concurrency + corruption recovery |
| `internal/storage/s3client.go` | Create | AWS SDK v2 adapter implementing S3API |
| `internal/ingredients/merge.go` | Create | Cross-recipe ingredient merging with synonyms + source tracking |
| `internal/handler/validate.go` | Create | Recipe ID generation/validation, input sanitization, field limits |
| `internal/handler/recipes.go` | Create | 6 recipe endpoints: list, get, save, update, delete, shop |
| `internal/handler/handler.go` | Modify | Add store field, HandlerOption type, WithStore, register recipe routes |
| `cmd/lambda/main.go` | Modify | Wire S3 client from env vars, pass store to handler |
| `test/models_test.go` | Create | Model serialization tests |
| `test/storage_test.go` | Create | S3Store tests with mock S3 client |
| `test/merge_test.go` | Create | Cross-recipe ingredient merge tests |
| `test/validate_test.go` | Create | Validation and ID generation tests |
| `test/recipes_test.go` | Create | Recipe CRUD handler tests with MockStore |
| `test/parser_test.go` | Modify | Update handler.New calls for new signature |
| `go.mod` | Modify | Add AWS SDK v2 dependencies |
