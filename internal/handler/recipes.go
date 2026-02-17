package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"recipe-to-reminders/internal/ingredients"
	"recipe-to-reminders/internal/models"
	"recipe-to-reminders/internal/storage"
)

// requireStore returns true if the store is configured, or writes a 503 and
// returns false.
func (h *Handler) requireStore(w http.ResponseWriter) bool {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "storage_unavailable",
			"Recipe storage is not configured.")
		return false
	}
	return true
}

func (h *Handler) handleListRecipes(w http.ResponseWriter, r *http.Request) {
	if !h.requireStore(w) {
		return
	}

	col, _, err := h.store.ReadAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage_error", "Failed to read recipes.")
		return
	}

	summaries := make([]models.RecipeSummary, 0, len(col.Recipes))
	for _, recipe := range col.Recipes {
		summaries = append(summaries, models.RecipeSummary{
			ID:              recipe.ID,
			Name:            recipe.Name,
			Tags:            recipe.Tags,
			IngredientCount: len(recipe.Ingredients),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"recipes": summaries})
}

func (h *Handler) handleGetRecipe(w http.ResponseWriter, r *http.Request) {
	if !h.requireStore(w) {
		return
	}

	id := r.PathValue("id")
	if err := ValidateRecipeID(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	col, _, err := h.store.ReadAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage_error", "Failed to read recipes.")
		return
	}

	for _, recipe := range col.Recipes {
		if recipe.ID == id {
			writeJSON(w, http.StatusOK, recipe)
			return
		}
	}

	writeError(w, http.StatusNotFound, "not_found", "Recipe not found.")
}

func (h *Handler) handleSaveRecipe(w http.ResponseWriter, r *http.Request) {
	if !h.requireStore(w) {
		return
	}

	var req models.SaveRecipeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body.")
		return
	}

	req.Name = SanitizeString(req.Name, maxRecipeName)
	req.Source = SanitizeString(req.Source, maxRecipeName)
	req.SourceURL = SanitizeString(req.SourceURL, 2000)
	for i := range req.Tags {
		req.Tags[i] = SanitizeString(req.Tags[i], maxTagLen)
	}

	if err := ValidateSaveRecipeRequest(req.Name, req.Tags, req.Ingredients, 0); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}

	col, etag, err := h.store.ReadAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage_error", "Failed to read recipes.")
		return
	}

	if len(col.Recipes) >= maxRecipes {
		writeError(w, http.StatusConflict, "at_capacity",
			"Maximum number of saved recipes reached (200).")
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	recipe := models.SavedRecipe{
		ID:          GenerateRecipeID(req.Name),
		Name:        req.Name,
		Source:      req.Source,
		SourceURL:   req.SourceURL,
		Servings:    req.Servings,
		CreatedAt:   now,
		UpdatedAt:   now,
		Tags:        req.Tags,
		Ingredients: req.Ingredients,
	}

	col.Recipes = append(col.Recipes, recipe)
	col.UpdatedAt = now

	if err := h.store.WriteAll(r.Context(), col, etag); err != nil {
		if errors.Is(err, storage.ErrConflict) {
			writeError(w, http.StatusConflict, "conflict", "Concurrent modification — please retry.")
			return
		}
		writeError(w, http.StatusInternalServerError, "storage_error", "Failed to save recipe.")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"id":      recipe.ID,
		"message": "Recipe saved.",
	})
}

func (h *Handler) handleUpdateRecipe(w http.ResponseWriter, r *http.Request) {
	if !h.requireStore(w) {
		return
	}

	id := r.PathValue("id")
	if err := ValidateRecipeID(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	var updates map[string]json.RawMessage
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody)).Decode(&updates); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body.")
		return
	}

	col, etag, err := h.store.ReadAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage_error", "Failed to read recipes.")
		return
	}

	idx := -1
	for i, recipe := range col.Recipes {
		if recipe.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, "not_found", "Recipe not found.")
		return
	}

	recipe := &col.Recipes[idx]

	if raw, ok := updates["name"]; ok {
		var name string
		if err := json.Unmarshal(raw, &name); err == nil {
			recipe.Name = SanitizeString(name, maxRecipeName)
		}
	}
	if raw, ok := updates["source"]; ok {
		var source string
		if err := json.Unmarshal(raw, &source); err == nil {
			recipe.Source = SanitizeString(source, maxRecipeName)
		}
	}
	if raw, ok := updates["source_url"]; ok {
		var url string
		if err := json.Unmarshal(raw, &url); err == nil {
			recipe.SourceURL = SanitizeString(url, 2000)
		}
	}
	if raw, ok := updates["servings"]; ok {
		var servings string
		if err := json.Unmarshal(raw, &servings); err == nil {
			recipe.Servings = SanitizeString(servings, 50)
		}
	}
	if raw, ok := updates["tags"]; ok {
		var tags []string
		if err := json.Unmarshal(raw, &tags); err == nil {
			for i := range tags {
				tags[i] = SanitizeString(tags[i], maxTagLen)
			}
			recipe.Tags = tags
		}
	}
	if raw, ok := updates["ingredients"]; ok {
		var ings []models.Ingredient
		if err := json.Unmarshal(raw, &ings); err == nil {
			recipe.Ingredients = ings
		}
	}

	if err := ValidateSaveRecipeRequest(recipe.Name, recipe.Tags, recipe.Ingredients, 0); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	recipe.UpdatedAt = now
	col.UpdatedAt = now

	if err := h.store.WriteAll(r.Context(), col, etag); err != nil {
		if errors.Is(err, storage.ErrConflict) {
			writeError(w, http.StatusConflict, "conflict", "Concurrent modification — please retry.")
			return
		}
		writeError(w, http.StatusInternalServerError, "storage_error", "Failed to update recipe.")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"id":      id,
		"message": "Recipe updated.",
	})
}

func (h *Handler) handleDeleteRecipe(w http.ResponseWriter, r *http.Request) {
	if !h.requireStore(w) {
		return
	}

	id := r.PathValue("id")
	if err := ValidateRecipeID(id); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", err.Error())
		return
	}

	col, etag, err := h.store.ReadAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage_error", "Failed to read recipes.")
		return
	}

	idx := -1
	for i, recipe := range col.Recipes {
		if recipe.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeError(w, http.StatusNotFound, "not_found", "Recipe not found.")
		return
	}

	col.Recipes = append(col.Recipes[:idx], col.Recipes[idx+1:]...)
	col.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if err := h.store.WriteAll(r.Context(), col, etag); err != nil {
		if errors.Is(err, storage.ErrConflict) {
			writeError(w, http.StatusConflict, "conflict", "Concurrent modification — please retry.")
			return
		}
		writeError(w, http.StatusInternalServerError, "storage_error", "Failed to delete recipe.")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"id":      id,
		"message": "Recipe deleted.",
	})
}

func (h *Handler) handleShopRecipes(w http.ResponseWriter, r *http.Request) {
	if !h.requireStore(w) {
		return
	}

	var req models.ShopRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body.")
		return
	}

	if err := ValidateShopRequest(req.RecipeIDs); err != nil {
		writeError(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}

	col, _, err := h.store.ReadAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "storage_error", "Failed to read recipes.")
		return
	}

	recipeMap := make(map[string]*models.SavedRecipe, len(col.Recipes))
	for i := range col.Recipes {
		recipeMap[col.Recipes[i].ID] = &col.Recipes[i]
	}

	var recipeInputs []ingredients.RecipeIngredients
	var recipeNames []string

	for _, id := range req.RecipeIDs {
		recipe, ok := recipeMap[id]
		if !ok {
			writeError(w, http.StatusNotFound, "not_found", "Recipe "+id+" not found.")
			return
		}
		recipeInputs = append(recipeInputs, ingredients.RecipeIngredients{
			RecipeName:  recipe.Name,
			Ingredients: recipe.Ingredients,
		})
		recipeNames = append(recipeNames, recipe.Name)
	}

	if req.Deduplicate {
		merged := ingredients.MergeIngredients(recipeInputs)
		writeJSON(w, http.StatusOK, models.ShopResponse{
			RecipesIncluded: recipeNames,
			Ingredients:     merged,
		})
	} else {
		var all []models.MergedIngredient
		for _, ri := range recipeInputs {
			for _, ing := range ri.Ingredients {
				all = append(all, models.MergedIngredient{
					Name:     ing.Name,
					Quantity: ing.Quantity,
					Unit:     ing.Unit,
					Category: ing.Category,
					Sources:  []string{ri.RecipeName},
				})
			}
		}
		writeJSON(w, http.StatusOK, models.ShopResponse{
			RecipesIncluded: recipeNames,
			Ingredients:     all,
		})
	}
}
