package handler

import (
	"encoding/json"
	"net/http"

	"recipe-to-reminders/internal/models"
	"recipe-to-reminders/internal/parser"
)

// Handler routes HTTP requests to the appropriate endpoint handler.
type Handler struct {
	mux       *http.ServeMux
	extractor *parser.Extractor
}

// New creates a Handler wired to a Fetcher for URL extraction.
func New(fetcher *parser.Fetcher, opts ...parser.ExtractorOption) *Handler {
	h := &Handler{
		mux:       http.NewServeMux(),
		extractor: parser.NewExtractor(fetcher, opts...),
	}
	h.mux.HandleFunc("POST /extract", h.handleExtract)
	// Return 405 for non-POST on /extract
	h.mux.HandleFunc("/extract", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for /extract")
	})
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
