package handlers

import (
	"errors"
	"net/http"

	"github.com/gourl/gourl/internal/models"
	"github.com/gourl/gourl/internal/services"
)

// RedirectHandler handles URL redirect requests.
type RedirectHandler struct {
	service services.RedirectService
}

// NewRedirectHandler creates a new RedirectHandler.
func NewRedirectHandler(svc services.RedirectService) *RedirectHandler {
	return &RedirectHandler{service: svc}
}

// Redirect handles GET /:code requests and redirects to the original URL.
// This is optimized for minimal latency - cache hits should return in < 5ms.
func (h *RedirectHandler) Redirect(w http.ResponseWriter, r *http.Request, shortCode string) {
	result, err := h.service.Redirect(r.Context(), shortCode)
	if err != nil {
		h.handleError(w, err)
		return
	}

	// Choose redirect status code
	statusCode := http.StatusFound // 302 Temporary Redirect
	if result.Permanent {
		statusCode = http.StatusMovedPermanently // 301 Permanent Redirect
	}

	// Set Location header and send redirect response
	http.Redirect(w, r, result.OriginalURL, statusCode)
}

// handleError maps service errors to HTTP responses for redirect endpoints.
func (h *RedirectHandler) handleError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, models.ErrURLNotFound):
		http.Error(w, "URL not found", http.StatusNotFound)
	case errors.Is(err, models.ErrURLExpired):
		http.Error(w, "URL has expired", http.StatusGone)
	default:
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}
