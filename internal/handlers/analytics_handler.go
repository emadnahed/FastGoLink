package handlers

import (
	"net/http"

	"github.com/gourl/gourl/internal/services"
)

// AnalyticsHandler handles analytics-related HTTP requests.
type AnalyticsHandler struct {
	service services.AnalyticsService
}

// NewAnalyticsHandler creates a new AnalyticsHandler.
func NewAnalyticsHandler(svc services.AnalyticsService) *AnalyticsHandler {
	return &AnalyticsHandler{service: svc}
}

// GetStats handles GET /api/v1/analytics/:code requests.
func (h *AnalyticsHandler) GetStats(w http.ResponseWriter, r *http.Request, shortCode string) {
	if shortCode == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error: "short code is required",
			Code:  "INVALID_SHORT_CODE",
		})
		return
	}

	stats, err := h.service.GetURLStats(r.Context(), shortCode)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{
			Error: "URL not found",
			Code:  "NOT_FOUND",
		})
		return
	}

	writeJSON(w, http.StatusOK, stats)
}
