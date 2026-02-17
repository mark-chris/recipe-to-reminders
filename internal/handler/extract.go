package handler

import (
	"encoding/json"
	"net/http"

	"recipe-to-reminders/internal/models"
	"recipe-to-reminders/internal/parser"
)

const (
	maxRequestBody = 10 * 1024 * 1024 // 10MB (API Gateway limit)
	maxImageSizeMB = 7
	maxImageDim    = 3000
)

func (h *Handler) handleExtract(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)

	var req models.ExtractRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	// Reject if both URL and image_base64 are provided
	if req.URL != "" && req.ImageBase64 != "" {
		writeError(w, http.StatusBadRequest, "invalid_request",
			"Provide either a URL or image_base64, not both.")
		return
	}

	// Image path
	if req.ImageBase64 != "" {
		h.handleImageExtract(w, r, req.ImageBase64)
		return
	}

	// URL path
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "missing_input",
			"Provide a URL or image_base64 in the request body.")
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

func (h *Handler) handleImageExtract(w http.ResponseWriter, r *http.Request, b64 string) {
	imageBytes, _, err := parser.DecodeBase64Image(b64, maxImageSizeMB)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_image", err.Error())
		return
	}

	if err := parser.ValidateImageDimensions(imageBytes, maxImageDim); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_image", err.Error())
		return
	}

	result, err := h.extractor.ExtractImage(r.Context(), imageBytes)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "extraction_failed",
			"Could not extract ingredients from the provided image.")
		return
	}

	writeJSON(w, http.StatusOK, result)
}
