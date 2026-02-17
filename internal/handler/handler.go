package handler

import (
	"encoding/json"
	"net/http"

	"recipe-to-reminders/internal/models"
	"recipe-to-reminders/internal/parser"
	"recipe-to-reminders/internal/storage"
)

// Handler routes HTTP requests to the appropriate endpoint handler.
type Handler struct {
	mux       *http.ServeMux
	extractor *parser.Extractor
	store     storage.Store
}

// HandlerOption configures the Handler.
type HandlerOption func(*Handler)

// WithStore sets the recipe storage backend.
func WithStore(s storage.Store) HandlerOption {
	return func(h *Handler) {
		h.store = s
	}
}

// New creates a Handler. HandlerOptions configure the handler itself (like WithStore).
// ExtractorOptions configure the underlying parser.
func New(fetcher *parser.Fetcher, handlerOpts []HandlerOption, extractorOpts ...parser.ExtractorOption) *Handler {
	h := &Handler{
		mux:       http.NewServeMux(),
		extractor: parser.NewExtractor(fetcher, extractorOpts...),
	}

	for _, opt := range handlerOpts {
		if opt != nil {
			opt(h)
		}
	}

	h.mux.HandleFunc("POST /extract", h.handleExtract)
	// Return 405 for non-POST on /extract
	h.mux.HandleFunc("/extract", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for /extract")
	})

	// Recipe CRUD routes
	// IMPORTANT: POST /recipes/shop MUST be registered BEFORE /recipes/{id} patterns
	// otherwise the Go 1.22+ ServeMux will match {id} = "shop" first.
	h.mux.HandleFunc("GET /recipes", h.handleListRecipes)
	h.mux.HandleFunc("POST /recipes", h.handleSaveRecipe)
	h.mux.HandleFunc("POST /recipes/shop", h.handleShopRecipes)
	h.mux.HandleFunc("GET /recipes/{id}", h.handleGetRecipe)
	h.mux.HandleFunc("PUT /recipes/{id}", h.handleUpdateRecipe)
	h.mux.HandleFunc("DELETE /recipes/{id}", h.handleDeleteRecipe)

	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, models.ErrorResponse{Error: code, Message: message})
}
