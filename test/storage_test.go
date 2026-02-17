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
	PutCond string

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

	got, _, err := store.ReadAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.Recipes) != 250 {
		t.Errorf("recipes len = %d, want 250", len(got.Recipes))
	}
}
