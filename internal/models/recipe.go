package models

// Ingredient is a single parsed ingredient with structured fields.
type Ingredient struct {
	Name     string `json:"name"`
	Quantity string `json:"quantity"`
	Unit     string `json:"unit"`
	Category string `json:"category"`
	Raw      string `json:"raw"`
}

// RawRecipe holds the output of an HTML parser before ingredient normalization.
type RawRecipe struct {
	Title       string
	Source      string
	Servings    string
	Method      string
	Confidence  float64
	Ingredients []string // raw ingredient strings as extracted from the page
}

// ExtractRequest is the JSON body for POST /extract.
type ExtractRequest struct {
	URL         string `json:"url"`
	ImageBase64 string `json:"image_base64"`
	ListName    string `json:"list_name"`
}

// ExtractResponse is the JSON body returned by POST /extract.
type ExtractResponse struct {
	Title       string       `json:"title"`
	Source      string       `json:"source"`
	Servings    string       `json:"servings"`
	Method      string       `json:"method"`
	Confidence  float64      `json:"confidence"`
	Ingredients []Ingredient `json:"ingredients"`
}

// ErrorResponse is returned on failure.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// SavedRecipe is a recipe stored in S3 for reuse.
type SavedRecipe struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Source      string       `json:"source"`
	SourceURL   string       `json:"source_url"`
	Servings    string       `json:"servings"`
	CreatedAt   string       `json:"created_at"`
	UpdatedAt   string       `json:"updated_at"`
	Tags        []string     `json:"tags"`
	Ingredients []Ingredient `json:"ingredients"`
}

// RecipeCollection is the top-level schema for the S3 recipes.json file.
type RecipeCollection struct {
	Version   int           `json:"version"`
	UpdatedAt string        `json:"updated_at"`
	Recipes   []SavedRecipe `json:"recipes"`
}

// RecipeSummary is a lightweight recipe for list responses (no ingredients).
type RecipeSummary struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Tags            []string `json:"tags"`
	IngredientCount int      `json:"ingredient_count"`
}

// SaveRecipeRequest is the JSON body for POST /recipes.
type SaveRecipeRequest struct {
	Name        string       `json:"name"`
	Source      string       `json:"source"`
	SourceURL   string       `json:"source_url"`
	Servings    string       `json:"servings"`
	Tags        []string     `json:"tags"`
	Ingredients []Ingredient `json:"ingredients"`
}

// ShopRequest is the JSON body for POST /recipes/shop.
type ShopRequest struct {
	RecipeIDs   []string `json:"recipe_ids"`
	Deduplicate bool     `json:"deduplicate"`
}

// MergedIngredient is an ingredient with source tracking for multi-recipe shopping.
type MergedIngredient struct {
	Name     string   `json:"name"`
	Quantity string   `json:"quantity"`
	Unit     string   `json:"unit"`
	Category string   `json:"category"`
	Sources  []string `json:"sources"`
}

// ShopResponse is the JSON body returned by POST /recipes/shop.
type ShopResponse struct {
	RecipesIncluded []string           `json:"recipes_included"`
	Ingredients     []MergedIngredient `json:"ingredients"`
}
