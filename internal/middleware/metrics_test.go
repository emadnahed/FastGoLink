package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResponseWriter(t *testing.T) {
	t.Run("defaults to 200 OK", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rw := newResponseWriter(rec)

		assert.Equal(t, http.StatusOK, rw.statusCode)
	})

	t.Run("captures written status code", func(t *testing.T) {
		rec := httptest.NewRecorder()
		rw := newResponseWriter(rec)

		rw.WriteHeader(http.StatusNotFound)

		assert.Equal(t, http.StatusNotFound, rw.statusCode)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})
}

func TestMetrics(t *testing.T) {
	t.Run("wraps handler and records metrics", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		})

		middleware := Metrics()
		wrapped := middleware(handler)

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "OK", rec.Body.String())
	})

	t.Run("records correct status code for errors", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})

		middleware := Metrics()
		wrapped := middleware(handler)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/shorten", nil)
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
	})
}

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "health endpoint",
			path:     "/health",
			expected: "/health",
		},
		{
			name:     "ready endpoint",
			path:     "/ready",
			expected: "/ready",
		},
		{
			name:     "metrics endpoint",
			path:     "/metrics",
			expected: "/metrics",
		},
		{
			name:     "shorten endpoint",
			path:     "/api/v1/shorten",
			expected: "/api/v1/shorten",
		},
		{
			name:     "short code redirect",
			path:     "/abc123",
			expected: "/{code}",
		},
		{
			name:     "URL info endpoint",
			path:     "/api/v1/urls/abc123",
			expected: "/api/v1/urls/{code}",
		},
		{
			name:     "analytics endpoint",
			path:     "/api/v1/analytics/abc123",
			expected: "/api/v1/analytics/{code}",
		},
		{
			name:     "unknown path",
			path:     "/some/random/path",
			expected: "/other",
		},
		{
			name:     "long unknown path",
			path:     "/verylongpath123456789",
			expected: "/other",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizePath(tt.path)
			assert.Equal(t, tt.expected, result)
		})
	}
}
