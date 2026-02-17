package test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"recipe-to-reminders/internal/handler"
	"recipe-to-reminders/internal/models"
	"recipe-to-reminders/internal/storage"
)

// MockStore implements storage.Store for handler tests.
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
	return handler.New(nil, []handler.HandlerOption{handler.WithStore(store)})
}

func TestListRecipes_Empty(t *testing.T) {
	h := newTestHandler(&MockStore{})

	req := httptest.NewRequest(http.MethodGet, "/recipes", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. body: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Recipes []models.RecipeSummary `json:"recipes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
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
				{
					ID:   "stew-abc123",
					Name: "Beef Stew",
					Tags: []string{"dinner"},
					Ingredients: []models.Ingredient{
						{Name: "beef", Quantity: "2", Unit: "lbs"},
						{Name: "carrots", Quantity: "3", Unit: "large"},
					},
				},
				{
					ID:   "pancakes-def456",
					Name: "Pancakes",
					Tags: []string{"breakfast"},
					Ingredients: []models.Ingredient{
						{Name: "flour", Quantity: "2", Unit: "cups"},
					},
				},
			},
		},
		ETag: "etag-1",
	}
	h := newTestHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/recipes", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. body: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Recipes []models.RecipeSummary `json:"recipes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.Recipes) != 2 {
		t.Fatalf("recipes len = %d, want 2", len(resp.Recipes))
	}
	if resp.Recipes[0].IngredientCount != 2 {
		t.Errorf("first recipe ingredient_count = %d, want 2", resp.Recipes[0].IngredientCount)
	}
	if resp.Recipes[1].IngredientCount != 1 {
		t.Errorf("second recipe ingredient_count = %d, want 1", resp.Recipes[1].IngredientCount)
	}
}

func TestGetRecipe_Found(t *testing.T) {
	store := &MockStore{
		Collection: &models.RecipeCollection{
			Version: 1,
			Recipes: []models.SavedRecipe{
				{
					ID:   "stew-abc123",
					Name: "Beef Stew",
					Ingredients: []models.Ingredient{
						{Name: "beef", Quantity: "2", Unit: "lbs"},
					},
				},
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
	if err := json.Unmarshal(rr.Body.Bytes(), &recipe); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if recipe.ID != "stew-abc123" {
		t.Errorf("id = %q, want stew-abc123", recipe.ID)
	}
	if recipe.Name != "Beef Stew" {
		t.Errorf("name = %q, want Beef Stew", recipe.Name)
	}
}

func TestGetRecipe_NotFound(t *testing.T) {
	store := &MockStore{
		Collection: &models.RecipeCollection{
			Version: 1,
			Recipes: []models.SavedRecipe{},
		},
	}
	h := newTestHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/recipes/nonexistent-abc123", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404. body: %s", rr.Code, rr.Body.String())
	}
}

func TestGetRecipe_InvalidID(t *testing.T) {
	h := newTestHandler(&MockStore{})

	// ID with uppercase letters fails the regex pattern ^[a-z0-9][a-z0-9-]*-[a-f0-9]{6}$
	req := httptest.NewRequest(http.MethodGet, "/recipes/INVALID_ID", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400. body: %s", rr.Code, rr.Body.String())
	}
}

func TestSaveRecipe_Success(t *testing.T) {
	store := &MockStore{}
	h := newTestHandler(store)

	body, _ := json.Marshal(models.SaveRecipeRequest{
		Name:   "Test Recipe",
		Source: "example.com",
		Tags:   []string{"dinner"},
		Ingredients: []models.Ingredient{
			{Name: "flour", Quantity: "2", Unit: "cups"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/recipes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201. body: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["id"] == "" {
		t.Error("expected non-empty recipe ID")
	}
	if resp["message"] != "Recipe saved." {
		t.Errorf("message = %q, want 'Recipe saved.'", resp["message"])
	}
	if store.WriteCalls != 1 {
		t.Errorf("write calls = %d, want 1", store.WriteCalls)
	}
	if len(store.Collection.Recipes) != 1 {
		t.Fatalf("stored recipes = %d, want 1", len(store.Collection.Recipes))
	}
}

func TestSaveRecipe_EmptyName(t *testing.T) {
	h := newTestHandler(&MockStore{})

	body, _ := json.Marshal(models.SaveRecipeRequest{
		Name: "",
		Ingredients: []models.Ingredient{
			{Name: "flour", Quantity: "2", Unit: "cups"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/recipes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400. body: %s", rr.Code, rr.Body.String())
	}
}

func TestSaveRecipe_AtCapacity(t *testing.T) {
	recipes := make([]models.SavedRecipe, 200)
	for i := range recipes {
		recipes[i] = models.SavedRecipe{
			ID:   fmt.Sprintf("recipe%d-%06x", i, i),
			Name: fmt.Sprintf("Recipe %d", i),
		}
	}
	store := &MockStore{
		Collection: &models.RecipeCollection{
			Version: 1,
			Recipes: recipes,
		},
	}
	h := newTestHandler(store)

	body, _ := json.Marshal(models.SaveRecipeRequest{
		Name: "One Too Many",
		Ingredients: []models.Ingredient{
			{Name: "flour", Quantity: "1", Unit: "cup"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/recipes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409. body: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateRecipe_Success(t *testing.T) {
	store := &MockStore{
		Collection: &models.RecipeCollection{
			Version: 1,
			Recipes: []models.SavedRecipe{
				{
					ID:   "stew-abc123",
					Name: "Beef Stew",
					Tags: []string{"dinner"},
					Ingredients: []models.Ingredient{
						{Name: "beef", Quantity: "2", Unit: "lbs"},
					},
				},
			},
		},
		ETag: "etag-1",
	}
	h := newTestHandler(store)

	body := []byte(`{"name": "Updated Beef Stew", "tags": ["dinner", "winter"]}`)
	req := httptest.NewRequest(http.MethodPut, "/recipes/stew-abc123", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. body: %s", rr.Code, rr.Body.String())
	}

	// Verify partial update: name and tags changed, ingredients preserved
	updated := store.Collection.Recipes[0]
	if updated.Name != "Updated Beef Stew" {
		t.Errorf("name = %q, want 'Updated Beef Stew'", updated.Name)
	}
	if len(updated.Tags) != 2 || updated.Tags[1] != "winter" {
		t.Errorf("tags = %v, want [dinner winter]", updated.Tags)
	}
	if len(updated.Ingredients) != 1 {
		t.Errorf("ingredients should be preserved, got %d", len(updated.Ingredients))
	}
}

func TestUpdateRecipe_NotFound(t *testing.T) {
	store := &MockStore{
		Collection: &models.RecipeCollection{
			Version: 1,
			Recipes: []models.SavedRecipe{},
		},
	}
	h := newTestHandler(store)

	body := []byte(`{"name": "Updated"}`)
	req := httptest.NewRequest(http.MethodPut, "/recipes/nonexistent-abc123", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404. body: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteRecipe_Success(t *testing.T) {
	store := &MockStore{
		Collection: &models.RecipeCollection{
			Version: 1,
			Recipes: []models.SavedRecipe{
				{
					ID:   "stew-abc123",
					Name: "Beef Stew",
					Ingredients: []models.Ingredient{
						{Name: "beef", Quantity: "2", Unit: "lbs"},
					},
				},
			},
		},
		ETag: "etag-1",
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
	store := &MockStore{
		Collection: &models.RecipeCollection{
			Version: 1,
			Recipes: []models.SavedRecipe{},
		},
	}
	h := newTestHandler(store)

	req := httptest.NewRequest(http.MethodDelete, "/recipes/nonexistent-abc123", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404. body: %s", rr.Code, rr.Body.String())
	}
}

func TestShopRecipes_Success(t *testing.T) {
	store := &MockStore{
		Collection: &models.RecipeCollection{
			Version: 1,
			Recipes: []models.SavedRecipe{
				{
					ID:   "stew-abc123",
					Name: "Beef Stew",
					Ingredients: []models.Ingredient{
						{Name: "butter", Quantity: "2", Unit: "tbsp", Category: "dairy"},
						{Name: "beef", Quantity: "2", Unit: "lbs", Category: "meat"},
					},
				},
				{
					ID:   "pancakes-def456",
					Name: "Pancakes",
					Ingredients: []models.Ingredient{
						{Name: "butter", Quantity: "1", Unit: "tbsp", Category: "dairy"},
						{Name: "flour", Quantity: "2", Unit: "cups", Category: "pantry"},
					},
				},
			},
		},
	}
	h := newTestHandler(store)

	body, _ := json.Marshal(models.ShopRequest{
		RecipeIDs:   []string{"stew-abc123", "pancakes-def456"},
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
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(resp.RecipesIncluded) != 2 {
		t.Errorf("recipes_included len = %d, want 2", len(resp.RecipesIncluded))
	}
	// With deduplication, butter should be merged (2 tbsp + 1 tbsp = 3 tbsp)
	// So we expect 3 unique items: butter, beef, flour
	if len(resp.Ingredients) != 3 {
		t.Errorf("ingredients len = %d, want 3 (merged). body: %s", len(resp.Ingredients), rr.Body.String())
	}
}

func TestShopRecipes_EmptyIDs(t *testing.T) {
	h := newTestHandler(&MockStore{})

	body, _ := json.Marshal(models.ShopRequest{
		RecipeIDs:   []string{},
		Deduplicate: true,
	})
	req := httptest.NewRequest(http.MethodPost, "/recipes/shop", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400. body: %s", rr.Code, rr.Body.String())
	}
}

func TestShopRecipes_RecipeNotFound(t *testing.T) {
	store := &MockStore{
		Collection: &models.RecipeCollection{
			Version: 1,
			Recipes: []models.SavedRecipe{},
		},
	}
	h := newTestHandler(store)

	body, _ := json.Marshal(models.ShopRequest{
		RecipeIDs:   []string{"missing-abc123"},
		Deduplicate: true,
	})
	req := httptest.NewRequest(http.MethodPost, "/recipes/shop", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404. body: %s", rr.Code, rr.Body.String())
	}
}

func TestShopRecipes_NoDeduplicate(t *testing.T) {
	store := &MockStore{
		Collection: &models.RecipeCollection{
			Version: 1,
			Recipes: []models.SavedRecipe{
				{
					ID:   "stew-abc123",
					Name: "Beef Stew",
					Ingredients: []models.Ingredient{
						{Name: "butter", Quantity: "2", Unit: "tbsp", Category: "dairy"},
					},
				},
				{
					ID:   "pancakes-def456",
					Name: "Pancakes",
					Ingredients: []models.Ingredient{
						{Name: "butter", Quantity: "1", Unit: "tbsp", Category: "dairy"},
					},
				},
			},
		},
	}
	h := newTestHandler(store)

	body, _ := json.Marshal(models.ShopRequest{
		RecipeIDs:   []string{"stew-abc123", "pancakes-def456"},
		Deduplicate: false,
	})
	req := httptest.NewRequest(http.MethodPost, "/recipes/shop", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200. body: %s", rr.Code, rr.Body.String())
	}

	var resp models.ShopResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	// Without deduplication, butter appears twice (one from each recipe)
	if len(resp.Ingredients) != 2 {
		t.Errorf("ingredients len = %d, want 2 (no merge). body: %s", len(resp.Ingredients), rr.Body.String())
	}
}

func TestRecipes_NoStore(t *testing.T) {
	// handler without store configured
	h := handler.New(nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/recipes", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503. body: %s", rr.Code, rr.Body.String())
	}

	var resp models.ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error != "storage_unavailable" {
		t.Errorf("error = %q, want storage_unavailable", resp.Error)
	}
}

func TestRecipes_StoreReadError(t *testing.T) {
	store := &MockStore{
		ReadErr: errors.New("s3 unavailable"),
	}
	h := newTestHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/recipes", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500. body: %s", rr.Code, rr.Body.String())
	}
}

func TestRecipes_WriteConflict(t *testing.T) {
	store := &MockStore{
		WriteErr: storage.ErrConflict,
	}
	h := newTestHandler(store)

	body, _ := json.Marshal(models.SaveRecipeRequest{
		Name: "Conflict Recipe",
		Ingredients: []models.Ingredient{
			{Name: "flour", Quantity: "1", Unit: "cup"},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/recipes", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409. body: %s", rr.Code, rr.Body.String())
	}

	var resp models.ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error != "conflict" {
		t.Errorf("error = %q, want conflict", resp.Error)
	}
}
