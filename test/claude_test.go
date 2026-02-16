package test

import (
	"context"
	"testing"

	"recipe-to-reminders/internal/parser"
)

// MockImageExtractor for use by other tests.
type MockImageExtractor struct {
	Result *parser.ClaudeResponse
	Err    error
}

func (m *MockImageExtractor) Extract(_ context.Context, _ []byte) (*parser.ClaudeResponse, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Result, nil
}

func TestValidateClaudeResponse_Valid(t *testing.T) {
	resp := &parser.ClaudeResponse{
		Title: "Beef Stew",
		Ingredients: []parser.ClaudeIngredient{
			{Raw: "2 cups flour", Name: "flour", Quantity: "2", Unit: "cups"},
			{Raw: "1 tsp salt", Name: "salt", Quantity: "1", Unit: "tsp"},
		},
	}
	err := parser.ValidateClaudeResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateClaudeResponse_TooManyIngredients(t *testing.T) {
	resp := &parser.ClaudeResponse{Title: "Big Recipe"}
	for i := 0; i < 101; i++ {
		resp.Ingredients = append(resp.Ingredients, parser.ClaudeIngredient{
			Raw: "1 cup item", Name: "item",
		})
	}
	err := parser.ValidateClaudeResponse(resp)
	if err == nil {
		t.Fatal("expected error for >100 ingredients")
	}
}

func TestValidateClaudeResponse_OversizedField(t *testing.T) {
	long := ""
	for len(long) < 600 {
		long += "x"
	}
	resp := &parser.ClaudeResponse{
		Title: "Test",
		Ingredients: []parser.ClaudeIngredient{
			{Raw: long, Name: "item"},
		},
	}
	err := parser.ValidateClaudeResponse(resp)
	if err == nil {
		t.Fatal("expected error for oversized raw field")
	}
}

func TestValidateClaudeResponse_Empty(t *testing.T) {
	resp := &parser.ClaudeResponse{}
	err := parser.ValidateClaudeResponse(resp)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateClaudeJSON_UnexpectedFields(t *testing.T) {
	raw := []byte(`{"title":"Stew","ingredients":[],"instructions":"Mix all together","secret":"injected"}`)
	err := parser.ValidateClaudeJSON(raw)
	if err == nil {
		t.Fatal("expected error for unexpected fields in Claude response")
	}
}

func TestValidateClaudeJSON_ValidFields(t *testing.T) {
	raw := []byte(`{"title":"Stew","ingredients":[{"raw":"1 cup flour","name":"flour","quantity":"1","unit":"cups"}]}`)
	err := parser.ValidateClaudeJSON(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewRateLimiter(t *testing.T) {
	rl := parser.NewRateLimiter(2)
	if !rl.Allow() {
		t.Error("first call should be allowed")
	}
	if !rl.Allow() {
		t.Error("second call should be allowed")
	}
	if rl.Allow() {
		t.Error("third call should be rate limited")
	}
}
