package test

import (
	"strings"
	"testing"

	"recipe-to-reminders/internal/handler"
	"recipe-to-reminders/internal/models"
)

func TestGenerateRecipeID(t *testing.T) {
	id := handler.GenerateRecipeID("Classic Beef Stew")
	if len(id) < 8 {
		t.Errorf("id %q too short", id)
	}
	parts := splitLast(id, "-")
	if len(parts[1]) != 6 {
		t.Errorf("hex suffix %q should be 6 chars", parts[1])
	}
	if parts[0] != "classic-beef-stew" {
		t.Errorf("slug = %q, want classic-beef-stew", parts[0])
	}
}

func TestGenerateRecipeID_SpecialChars(t *testing.T) {
	id := handler.GenerateRecipeID("Mom's Best Mac & Cheese!!!")
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			t.Errorf("invalid char %q in ID %q", string(c), id)
		}
	}
}

func TestGenerateRecipeID_EmptyName(t *testing.T) {
	id := handler.GenerateRecipeID("")
	if len(id) < 7 {
		t.Errorf("id %q too short for empty name", id)
	}
}

func TestGenerateRecipeID_Uniqueness(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		id := handler.GenerateRecipeID("Test Recipe")
		if seen[id] {
			t.Errorf("duplicate ID generated: %q", id)
		}
		seen[id] = true
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
		"abc",
		"-abc123",
		"ABC-abc123",
		"../../etc-abc123",
		"test/path-abc123",
		"test\\path-abc123",
		"a-ABCDEF",
		"a-abcde",
		"a-abcdefg",
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
		{"tabs\tok", 100, "tabs\tok"},
		{"newlines\nok", 100, "newlines\nok"},
		{"toolong", 4, "tool"},
	}
	for _, tt := range tests {
		got := handler.SanitizeString(tt.input, tt.max)
		if got != tt.want {
			t.Errorf("SanitizeString(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
		}
	}
}

func TestSanitizeString_InvalidUTF8(t *testing.T) {
	// Invalid UTF-8 sequence should be stripped
	input := "hello\xff\xfeworld"
	got := handler.SanitizeString(input, 100)
	if strings.Contains(got, "\xff") || strings.Contains(got, "\xfe") {
		t.Errorf("SanitizeString should remove invalid UTF-8, got %q", got)
	}
	if !strings.Contains(got, "hello") || !strings.Contains(got, "world") {
		t.Errorf("SanitizeString should preserve valid parts, got %q", got)
	}
}

func TestValidateSaveRecipeRequest(t *testing.T) {
	err := handler.ValidateSaveRecipeRequest("Recipe", nil, nil, 0)
	if err != nil {
		t.Errorf("valid request got error: %v", err)
	}

	err = handler.ValidateSaveRecipeRequest("", nil, nil, 0)
	if err == nil {
		t.Error("empty name should be rejected")
	}
}

func TestValidateSaveRecipeRequest_NameTooLong(t *testing.T) {
	longName := strings.Repeat("a", 201)
	err := handler.ValidateSaveRecipeRequest(longName, nil, nil, 0)
	if err == nil {
		t.Error("name exceeding 200 chars should be rejected")
	}
}

func TestValidateSaveRecipeRequest_TooManyTags(t *testing.T) {
	tags := make([]string, 21)
	for i := range tags {
		tags[i] = "tag"
	}
	err := handler.ValidateSaveRecipeRequest("Recipe", tags, nil, 0)
	if err == nil {
		t.Error("more than 20 tags should be rejected")
	}
}

func TestValidateSaveRecipeRequest_TagTooLong(t *testing.T) {
	tags := []string{strings.Repeat("a", 51)}
	err := handler.ValidateSaveRecipeRequest("Recipe", tags, nil, 0)
	if err == nil {
		t.Error("tag exceeding 50 chars should be rejected")
	}
}

func TestValidateSaveRecipeRequest_TooManyIngredients(t *testing.T) {
	ings := make([]models.Ingredient, 101)
	err := handler.ValidateSaveRecipeRequest("Recipe", nil, ings, 0)
	if err == nil {
		t.Error("more than 100 ingredients should be rejected")
	}
}

func TestValidateSaveRecipeRequest_IngredientRawTooLong(t *testing.T) {
	ings := []models.Ingredient{{Raw: strings.Repeat("x", 501)}}
	err := handler.ValidateSaveRecipeRequest("Recipe", nil, ings, 0)
	if err == nil {
		t.Error("ingredient raw field exceeding 500 chars should be rejected")
	}
}

func TestValidateSaveRecipeRequest_IngredientNameTooLong(t *testing.T) {
	ings := []models.Ingredient{{Name: strings.Repeat("x", 201)}}
	err := handler.ValidateSaveRecipeRequest("Recipe", nil, ings, 0)
	if err == nil {
		t.Error("ingredient name field exceeding 200 chars should be rejected")
	}
}

func TestValidateShopRequest(t *testing.T) {
	err := handler.ValidateShopRequest([]string{"beef-stew-abc123"})
	if err != nil {
		t.Errorf("valid shop request got error: %v", err)
	}
}

func TestValidateShopRequest_Empty(t *testing.T) {
	err := handler.ValidateShopRequest(nil)
	if err == nil {
		t.Error("empty recipe_ids should be rejected")
	}
	err = handler.ValidateShopRequest([]string{})
	if err == nil {
		t.Error("empty recipe_ids slice should be rejected")
	}
}

func TestValidateShopRequest_TooMany(t *testing.T) {
	ids := make([]string, 21)
	for i := range ids {
		hex := strings.Repeat("a", 5) + string(rune('0'+i%10))
		ids[i] = "recipe-" + hex
	}
	err := handler.ValidateShopRequest(ids)
	if err == nil {
		t.Error("more than 20 recipe_ids should be rejected")
	}
}

func TestValidateShopRequest_InvalidID(t *testing.T) {
	err := handler.ValidateShopRequest([]string{"not-valid"})
	if err == nil {
		t.Error("invalid recipe ID in shop request should be rejected")
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
