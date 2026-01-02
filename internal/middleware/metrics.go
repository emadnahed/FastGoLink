package middleware

import (
	"net/http"
	"time"

	"github.com/emadnahed/FastGoLink/internal/metrics"
)

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func newResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{
		ResponseWriter: w,
		statusCode:     http.StatusOK,
	}
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Metrics returns a middleware that records Prometheus metrics.
func Metrics() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := newResponseWriter(w)

			metrics.ActiveConnections.Inc()
			defer metrics.ActiveConnections.Dec()

			next.ServeHTTP(rw, r)

			duration := time.Since(start)
			path := normalizePath(r.URL.Path)
			metrics.RecordRequest(r.Method, path, rw.statusCode, duration)
		})
	}
}

// normalizePath normalizes the URL path for metrics labels.
// This prevents high cardinality from dynamic path segments.
func normalizePath(path string) string {
	switch {
	case path == "/health" || path == "/ready" || path == "/metrics":
		return path
	case len(path) > 0 && path[0] == '/' && len(path) <= 10:
		// Short code redirects: /{code}
		return "/{code}"
	case len(path) > 13 && path[:13] == "/api/v1/urls/":
		return "/api/v1/urls/{code}"
	case len(path) > 18 && path[:18] == "/api/v1/analytics/":
		return "/api/v1/analytics/{code}"
	case path == "/api/v1/shorten":
		return path
	default:
		return "/other"
	}
}
