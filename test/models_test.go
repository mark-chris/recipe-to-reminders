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
