package middleware

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

// uuidRegex matches UUID v4 format.
var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestRequestID(t *testing.T) {
	t.Run("generates ID when none provided", func(t *testing.T) {
		mw := RequestID()
		var capturedID string

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedID = GetRequestID(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		// Should set response header
		responseID := rec.Header().Get("X-Request-ID")
		assert.NotEmpty(t, responseID)
		assert.True(t, uuidRegex.MatchString(responseID), "expected UUID format, got: %s", responseID)

		// Context should have the ID
		assert.Equal(t, responseID, capturedID)
	})

	t.Run("uses provided valid ID", func(t *testing.T) {
		mw := RequestID()
		incomingID := "550e8400-e29b-41d4-a716-446655440000"
		var capturedID string

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedID = GetRequestID(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Request-ID", incomingID)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		// Should echo back the incoming ID
		assert.Equal(t, incomingID, rec.Header().Get("X-Request-ID"))
		assert.Equal(t, incomingID, capturedID)
	})

	t.Run("generates new ID for invalid format", func(t *testing.T) {
		mw := RequestID()
		invalidID := "invalid<script>alert('xss')</script>"
		var capturedID string

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedID = GetRequestID(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Request-ID", invalidID)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		responseID := rec.Header().Get("X-Request-ID")
		assert.NotEqual(t, invalidID, responseID)
		assert.True(t, uuidRegex.MatchString(responseID), "expected UUID format, got: %s", responseID)
		assert.Equal(t, responseID, capturedID)
	})

	t.Run("generates new ID for empty header value", func(t *testing.T) {
		mw := RequestID()

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Request-ID", "")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		responseID := rec.Header().Get("X-Request-ID")
		assert.NotEmpty(t, responseID)
		assert.True(t, uuidRegex.MatchString(responseID))
	})

	t.Run("generates new ID for too long value", func(t *testing.T) {
		mw := RequestID()
		longID := "a" + "0123456789" // repeated to make it very long
		for i := 0; i < 10; i++ {
			longID += longID
		}

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Request-ID", longID)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		responseID := rec.Header().Get("X-Request-ID")
		assert.NotEqual(t, longID, responseID)
		assert.True(t, uuidRegex.MatchString(responseID))
	})

	t.Run("accepts custom request ID format", func(t *testing.T) {
		mw := RequestID()
		// A valid custom format that's alphanumeric with dashes
		customID := "my-trace-id-12345"
		var capturedID string

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedID = GetRequestID(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Request-ID", customID)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		// Should accept the custom format
		assert.Equal(t, customID, rec.Header().Get("X-Request-ID"))
		assert.Equal(t, customID, capturedID)
	})
}

func TestClientIP(t *testing.T) {
	t.Run("extracts IP from RemoteAddr", func(t *testing.T) {
		mw := ClientIP(false, nil)
		var capturedIP string

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedIP = GetClientIP(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, "192.168.1.1", capturedIP)
	})

	t.Run("extracts IP without port", func(t *testing.T) {
		mw := ClientIP(false, nil)
		var capturedIP string

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedIP = GetClientIP(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, "192.168.1.1", capturedIP)
	})

	t.Run("ignores X-Forwarded-For when trust is false", func(t *testing.T) {
		mw := ClientIP(false, nil)
		var capturedIP string

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedIP = GetClientIP(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		req.Header.Set("X-Forwarded-For", "203.0.113.195")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, "192.168.1.1", capturedIP)
	})

	t.Run("uses X-Forwarded-For when trusted", func(t *testing.T) {
		mw := ClientIP(true, nil)
		var capturedIP string

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedIP = GetClientIP(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "10.0.0.1:80"
		req.Header.Set("X-Forwarded-For", "203.0.113.195, 70.41.3.18")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		// Should use the first (leftmost) IP from X-Forwarded-For
		assert.Equal(t, "203.0.113.195", capturedIP)
	})

	t.Run("uses X-Real-IP when trusted and X-Forwarded-For not present", func(t *testing.T) {
		mw := ClientIP(true, nil)
		var capturedIP string

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedIP = GetClientIP(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "10.0.0.1:80"
		req.Header.Set("X-Real-IP", "203.0.113.195")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, "203.0.113.195", capturedIP)
	})

	t.Run("handles IPv6 addresses", func(t *testing.T) {
		mw := ClientIP(false, nil)
		var capturedIP string

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedIP = GetClientIP(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "[2001:db8::1]:12345"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, "2001:db8::1", capturedIP)
	})

	t.Run("only trusts specific proxy IPs", func(t *testing.T) {
		trustedProxies := []string{"10.0.0.1", "10.0.0.2"}
		mw := ClientIP(true, trustedProxies)
		var capturedIP string

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedIP = GetClientIP(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

		// Request from trusted proxy
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "10.0.0.1:80"
		req.Header.Set("X-Forwarded-For", "203.0.113.195")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)
		assert.Equal(t, "203.0.113.195", capturedIP)

		// Request from untrusted proxy
		req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
		req2.RemoteAddr = "192.168.1.1:80"
		req2.Header.Set("X-Forwarded-For", "spoofed.ip")
		rec2 := httptest.NewRecorder()

		handler.ServeHTTP(rec2, req2)
		// capturedIP is now from the second request
		assert.Equal(t, "192.168.1.1", capturedIP)
	})

	t.Run("falls back to X-Real-IP when X-Forwarded-For is empty", func(t *testing.T) {
		mw := ClientIP(true, nil)
		var capturedIP string

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedIP = GetClientIP(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "10.0.0.1:80"
		req.Header.Set("X-Forwarded-For", "  ") // Empty after trimming
		req.Header.Set("X-Real-IP", "203.0.113.100")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		// Should fall back to X-Real-IP
		assert.Equal(t, "203.0.113.100", capturedIP)
	})

	t.Run("falls back to RemoteAddr when both headers empty", func(t *testing.T) {
		mw := ClientIP(true, nil)
		var capturedIP string

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedIP = GetClientIP(r.Context())
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "10.0.0.1:80"
		req.Header.Set("X-Forwarded-For", "  ") // Empty after trimming
		// No X-Real-IP header
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		// Should fall back to RemoteAddr
		assert.Equal(t, "10.0.0.1", capturedIP)
	})
}
