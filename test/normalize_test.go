package test

import (
	"testing"

	"recipe-to-reminders/internal/ingredients"
	"recipe-to-reminders/internal/models"
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

func TestCategorize(t *testing.T) {
	tests := []struct {
		name    string
		wantCat string
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
