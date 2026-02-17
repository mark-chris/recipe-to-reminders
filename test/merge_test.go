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
