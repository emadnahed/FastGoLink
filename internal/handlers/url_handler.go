package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gourl/gourl/internal/idgen"
	"github.com/gourl/gourl/internal/models"
	"github.com/gourl/gourl/internal/services"
)

// ShortenRequest represents the request body for creating a short URL.
type ShortenRequest struct {
	URL       string `json:"url"`
	ExpiresIn string `json:"expires_in,omitempty"`
}

// ShortenResponse represents the response for a successfully created short URL.
type ShortenResponse struct {
	ShortURL    string  `json:"short_url"`
	ShortCode   string  `json:"short_code"`
	OriginalURL string  `json:"original_url"`
	CreatedAt   string  `json:"created_at"`
	ExpiresAt   *string `json:"expires_at,omitempty"`
}

// URLInfoResponse represents the response for URL info retrieval.
type URLInfoResponse struct {
	ShortCode   string  `json:"short_code"`
	OriginalURL string  `json:"original_url"`
	CreatedAt   string  `json:"created_at"`
	ExpiresAt   *string `json:"expires_at,omitempty"`
	ClickCount  int64   `json:"click_count"`
}

// ErrorResponse represents an error response.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

// URLHandler handles URL shortening endpoints.
type URLHandler struct {
	service services.URLService
}

// NewURLHandler creates a new URLHandler.
func NewURLHandler(svc services.URLService) *URLHandler {
	return &URLHandler{service: svc}
}

// Shorten handles POST /api/v1/shorten requests.
func (h *URLHandler) Shorten(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var req ShortenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{
			Error: "invalid request body",
			Code:  "INVALID_REQUEST",
		})
		return
	}

	// Parse expires_in duration if provided
	var expiresIn *time.Duration
	if req.ExpiresIn != "" {
		d, err := time.ParseDuration(req.ExpiresIn)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{
				Error: "invalid expires_in duration format",
				Code:  "INVALID_EXPIRES_IN",
			})
			return
		}
		expiresIn = &d
	}

	// Call service
	createReq := services.CreateURLRequest{
		OriginalURL: req.URL,
		ExpiresIn:   expiresIn,
	}

	resp, err := h.service.Create(r.Context(), createReq)
	if err != nil {
		status, errResp := mapErrorToResponse(err)
		writeJSON(w, status, errResp)
		return
	}

	// Build response
	shortenResp := ShortenResponse{
		ShortURL:    resp.ShortURL,
		ShortCode:   resp.ShortCode,
		OriginalURL: resp.OriginalURL,
		CreatedAt:   resp.CreatedAt.Format(time.RFC3339),
	}
	if resp.ExpiresAt != nil {
		expiresAtStr := resp.ExpiresAt.Format(time.RFC3339)
		shortenResp.ExpiresAt = &expiresAtStr
	}

	writeJSON(w, http.StatusCreated, shortenResp)
}

// GetURL handles GET /api/v1/urls/:code requests.
func (h *URLHandler) GetURL(w http.ResponseWriter, r *http.Request, shortCode string) {
	url, err := h.service.Get(r.Context(), shortCode)
	if err != nil {
		status, errResp := mapErrorToResponse(err)
		writeJSON(w, status, errResp)
		return
	}

	// Build response
	infoResp := URLInfoResponse{
		ShortCode:   url.ShortCode,
		OriginalURL: url.OriginalURL,
		CreatedAt:   url.CreatedAt.Format(time.RFC3339),
		ClickCount:  url.ClickCount,
	}
	if url.ExpiresAt != nil {
		expiresAtStr := url.ExpiresAt.Format(time.RFC3339)
		infoResp.ExpiresAt = &expiresAtStr
	}

	writeJSON(w, http.StatusOK, infoResp)
}

// DeleteURL handles DELETE /api/v1/urls/:code requests.
func (h *URLHandler) DeleteURL(w http.ResponseWriter, r *http.Request, shortCode string) {
	err := h.service.Delete(r.Context(), shortCode)
	if err != nil {
		status, errResp := mapErrorToResponse(err)
		writeJSON(w, status, errResp)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// mapErrorToResponse maps service errors to HTTP status codes and error responses.
func mapErrorToResponse(err error) (int, ErrorResponse) {
	switch {
	case errors.Is(err, models.ErrEmptyURL):
		return http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "EMPTY_URL",
		}
	case errors.Is(err, models.ErrInvalidURL):
		return http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "INVALID_URL",
		}
	case errors.Is(err, models.ErrURLNotFound):
		return http.StatusNotFound, ErrorResponse{
			Error: err.Error(),
			Code:  "NOT_FOUND",
		}
	case errors.Is(err, models.ErrURLExpired):
		return http.StatusGone, ErrorResponse{
			Error: err.Error(),
			Code:  "EXPIRED",
		}
	case errors.Is(err, idgen.ErrMaxRetriesExceeded):
		return http.StatusServiceUnavailable, ErrorResponse{
			Error: "service temporarily unavailable",
			Code:  "RETRY_EXCEEDED",
		}
	case errors.Is(err, services.ErrDangerousURL):
		return http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "DANGEROUS_URL",
		}
	case errors.Is(err, services.ErrPrivateIPURL):
		return http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "PRIVATE_IP_BLOCKED",
		}
	case errors.Is(err, services.ErrBlockedHostURL):
		return http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "BLOCKED_HOST",
		}
	case errors.Is(err, services.ErrURLTooLong):
		return http.StatusBadRequest, ErrorResponse{
			Error: err.Error(),
			Code:  "URL_TOO_LONG",
		}
	default:
		return http.StatusInternalServerError, ErrorResponse{
			Error: "internal server error",
			Code:  "INTERNAL_ERROR",
		}
	}
}
