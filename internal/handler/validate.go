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
	if !utf8.ValidString(s) {
		s = strings.ToValidUTF8(s, "")
	}

	var b strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			b.WriteRune(r)
		}
	}
	s = strings.TrimSpace(b.String())

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
	return strings.TrimRight(result, "-")
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)[:n]
}
