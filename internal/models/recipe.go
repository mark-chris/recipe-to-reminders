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
