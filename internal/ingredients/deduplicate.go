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
