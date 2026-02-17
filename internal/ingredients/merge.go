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
	"unsalted butter":        "butter",
	"salted butter":          "butter",
	"whole milk":             "milk",
	"2% milk":               "milk",
	"skim milk":             "milk",
	"kosher salt":           "salt",
	"sea salt":              "salt",
	"table salt":            "salt",
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
// Same name + same unit -> add quantities. Incompatible -> keep separate.
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

	seen := make(map[mergeKey]int) // key -> index in result
	var result []mergeEntry

	for _, ri := range inputs {
		for _, ing := range ri.Ingredients {
			canonName := canonicalizeName(ing.Name)
			k := mergeKey{name: canonName, unit: strings.ToLower(ing.Unit)}

			source := formatSource(ri.RecipeName, ing.Quantity, ing.Unit)

			if idx, ok := seen[k]; ok {
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

func canonicalizeName(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	if canonical, ok := synonymMap[lower]; ok {
		return canonical
	}
	return lower
}

func formatSource(recipeName, qty, unit string) string {
	if qty == "" && unit == "" {
		return recipeName
	}
	if unit == "" {
		return fmt.Sprintf("%s (%s)", recipeName, qty)
	}
	return fmt.Sprintf("%s (%s %s)", recipeName, qty, unit)
}
