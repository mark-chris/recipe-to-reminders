package handler

import (
	"encoding/json"
	"net/http"

	"recipe-to-reminders/internal/models"
)

const maxRequestBody = 10 * 1024 * 1024 // 10MB (API Gateway limit)

func (h *Handler) handleExtract(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)

	var req models.ExtractRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	// image_base64 not yet supported
	if req.ImageBase64 != "" {
		writeError(w, http.StatusBadRequest, "image_not_supported",
			"Image extraction is not yet supported. Please provide a URL.")
		return
	}

	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "missing_input",
			"Provide a URL in the request body.")
		return
	}

	result, err := h.extractor.Extract(r.Context(), req.URL)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "no_recipe_found",
			"Could not extract recipe data from the provided URL.")
		return
	}

	writeJSON(w, http.StatusOK, result)
}
