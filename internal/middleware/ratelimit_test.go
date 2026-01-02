package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/emadnahed/FastGoLink/internal/ratelimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockLimiter implements ratelimit.Limiter for testing.
type mockLimiter struct {
	result *ratelimit.Result
	err    error
	calls  []string
}

func (m *mockLimiter) Allow(ctx context.Context, identifier string) (*ratelimit.Result, error) {
	m.calls = append(m.calls, identifier)
	return m.result, m.err
}

func (m *mockLimiter) Reset(ctx context.Context, identifier string) error {
	return nil
}

func (m *mockLimiter) Close() error {
	return nil
}

func TestRateLimit(t *testing.T) {
	t.Run("allows request when under limit", func(t *testing.T) {
		limiter := &mockLimiter{
			result: &ratelimit.Result{
				Allowed:   true,
				Remaining: 9,
				Limit:     10,
			},
		}

		mw := RateLimit(limiter, RateLimitConfig{})
		handlerCalled := false

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.True(t, handlerCalled)
		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "10", rec.Header().Get("X-RateLimit-Limit"))
		assert.Equal(t, "9", rec.Header().Get("X-RateLimit-Remaining"))
	})

	t.Run("returns 429 when over limit", func(t *testing.T) {
		limiter := &mockLimiter{
			result: &ratelimit.Result{
				Allowed:    false,
				Remaining:  0,
				RetryAfter: 30 * time.Second,
				Limit:      10,
			},
		}

		mw := RateLimit(limiter, RateLimitConfig{})
		handlerCalled := false

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.False(t, handlerCalled, "handler should not be called when rate limited")
		assert.Equal(t, http.StatusTooManyRequests, rec.Code)
		assert.Equal(t, "30", rec.Header().Get("Retry-After"))
		assert.Equal(t, "0", rec.Header().Get("X-RateLimit-Remaining"))

		// Check response body
		var resp map[string]interface{}
		err := json.NewDecoder(rec.Body).Decode(&resp)
		require.NoError(t, err)
		assert.Equal(t, "rate limit exceeded", resp["error"])
		assert.Equal(t, "RATE_LIMIT_EXCEEDED", resp["code"])
	})

	t.Run("uses IP from context when available", func(t *testing.T) {
		limiter := &mockLimiter{
			result: &ratelimit.Result{
				Allowed:   true,
				Remaining: 9,
				Limit:     10,
			},
		}

		// First add client IP middleware, then rate limit middleware
		chain := New(
			ClientIP(false, nil),
			RateLimit(limiter, RateLimitConfig{}),
		)

		handler := chain.Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		// Should have used the IP from context (with ip: prefix)
		require.Len(t, limiter.calls, 1)
		assert.Equal(t, "ip:192.168.1.1", limiter.calls[0])
	})

	t.Run("uses API key when provided", func(t *testing.T) {
		limiter := &mockLimiter{
			result: &ratelimit.Result{
				Allowed:   true,
				Remaining: 9,
				Limit:     10,
			},
		}

		mw := RateLimit(limiter, RateLimitConfig{
			APIKeyHeader: "X-API-Key",
		})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		req.Header.Set("X-API-Key", "my-api-key-123")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		// Should use API key as identifier
		require.Len(t, limiter.calls, 1)
		assert.Equal(t, "api:my-api-key-123", limiter.calls[0])
	})

	t.Run("falls back to IP when API key not provided", func(t *testing.T) {
		limiter := &mockLimiter{
			result: &ratelimit.Result{
				Allowed:   true,
				Remaining: 9,
				Limit:     10,
			},
		}

		mw := RateLimit(limiter, RateLimitConfig{
			APIKeyHeader: "X-API-Key",
		})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		// No API key header
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		// Should fall back to IP
		require.Len(t, limiter.calls, 1)
		assert.Equal(t, "ip:192.168.1.1", limiter.calls[0])
	})

	t.Run("uses X-Forwarded-For when trusted", func(t *testing.T) {
		limiter := &mockLimiter{
			result: &ratelimit.Result{
				Allowed:   true,
				Remaining: 9,
				Limit:     10,
			},
		}

		mw := RateLimit(limiter, RateLimitConfig{
			TrustProxy: true,
		})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "10.0.0.1:80"
		req.Header.Set("X-Forwarded-For", "203.0.113.195, 70.41.3.18")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Len(t, limiter.calls, 1)
		assert.Equal(t, "ip:203.0.113.195", limiter.calls[0])
	})

	t.Run("ignores X-Forwarded-For when not trusted", func(t *testing.T) {
		limiter := &mockLimiter{
			result: &ratelimit.Result{
				Allowed:   true,
				Remaining: 9,
				Limit:     10,
			},
		}

		mw := RateLimit(limiter, RateLimitConfig{
			TrustProxy: false,
		})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		req.Header.Set("X-Forwarded-For", "203.0.113.195")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Len(t, limiter.calls, 1)
		assert.Equal(t, "ip:192.168.1.1", limiter.calls[0])
	})

	t.Run("handles limiter error", func(t *testing.T) {
		limiter := &mockLimiter{
			err: context.DeadlineExceeded,
		}

		mw := RateLimit(limiter, RateLimitConfig{})
		handlerCalled := false

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		// On error, should fail open (allow the request)
		assert.True(t, handlerCalled, "should fail open on limiter error")
	})

	t.Run("sets correct headers on rate limited response", func(t *testing.T) {
		limiter := &mockLimiter{
			result: &ratelimit.Result{
				Allowed:    false,
				Remaining:  0,
				RetryAfter: 45 * time.Second,
				ResetAfter: 45 * time.Second,
				Limit:      100,
			},
		}

		mw := RateLimit(limiter, RateLimitConfig{})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, "45", rec.Header().Get("Retry-After"))
		assert.Equal(t, "100", rec.Header().Get("X-RateLimit-Limit"))
		assert.Equal(t, "0", rec.Header().Get("X-RateLimit-Remaining"))

		// Verify reset header is set (unix timestamp)
		resetHeader := rec.Header().Get("X-RateLimit-Reset")
		assert.NotEmpty(t, resetHeader)
		resetTime, err := strconv.ParseInt(resetHeader, 10, 64)
		require.NoError(t, err)
		assert.True(t, resetTime > time.Now().Unix())
	})

	t.Run("uses X-Real-IP when X-Forwarded-For is not set", func(t *testing.T) {
		limiter := &mockLimiter{
			result: &ratelimit.Result{
				Allowed:   true,
				Remaining: 9,
				Limit:     10,
			},
		}

		mw := RateLimit(limiter, RateLimitConfig{
			TrustProxy: true,
		})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "10.0.0.1:80"
		req.Header.Set("X-Real-IP", "203.0.113.100")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Len(t, limiter.calls, 1)
		assert.Equal(t, "ip:203.0.113.100", limiter.calls[0])
	})

	t.Run("handles RemoteAddr without port", func(t *testing.T) {
		limiter := &mockLimiter{
			result: &ratelimit.Result{
				Allowed:   true,
				Remaining: 9,
				Limit:     10,
			},
		}

		mw := RateLimit(limiter, RateLimitConfig{})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1" // No port
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Len(t, limiter.calls, 1)
		assert.Equal(t, "ip:192.168.1.1", limiter.calls[0])
	})

	t.Run("ignores X-Forwarded-For when not from trusted proxy", func(t *testing.T) {
		limiter := &mockLimiter{
			result: &ratelimit.Result{
				Allowed:   true,
				Remaining: 9,
				Limit:     10,
			},
		}

		mw := RateLimit(limiter, RateLimitConfig{
			TrustProxy:     true,
			TrustedProxies: []string{"10.0.0.1"},
		})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345" // Not in trusted proxies
		req.Header.Set("X-Forwarded-For", "203.0.113.195")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		// Should use RemoteAddr since not from trusted proxy
		require.Len(t, limiter.calls, 1)
		assert.Equal(t, "ip:192.168.1.1", limiter.calls[0])
	})

	t.Run("handles empty X-Forwarded-For value", func(t *testing.T) {
		limiter := &mockLimiter{
			result: &ratelimit.Result{
				Allowed:   true,
				Remaining: 9,
				Limit:     10,
			},
		}

		mw := RateLimit(limiter, RateLimitConfig{
			TrustProxy: true,
		})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "10.0.0.1:80"
		req.Header.Set("X-Forwarded-For", "  ,  ") // Empty values
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		// Should fall back to RemoteAddr
		require.Len(t, limiter.calls, 1)
		assert.Equal(t, "ip:10.0.0.1", limiter.calls[0])
	})

	t.Run("sets headers without ResetAfter", func(t *testing.T) {
		limiter := &mockLimiter{
			result: &ratelimit.Result{
				Allowed:    true,
				Remaining:  9,
				Limit:      10,
				ResetAfter: 0, // No reset time
			},
		}

		mw := RateLimit(limiter, RateLimitConfig{})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, "10", rec.Header().Get("X-RateLimit-Limit"))
		assert.Equal(t, "9", rec.Header().Get("X-RateLimit-Remaining"))
		assert.Empty(t, rec.Header().Get("X-RateLimit-Reset")) // Should not be set
	})

	t.Run("sets minimum RetryAfter of 1 second when less than 1s", func(t *testing.T) {
		limiter := &mockLimiter{
			result: &ratelimit.Result{
				Allowed:    false,
				Remaining:  0,
				RetryAfter: 500 * time.Millisecond, // Less than 1 second
				Limit:      10,
			},
		}

		mw := RateLimit(limiter, RateLimitConfig{})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusTooManyRequests, rec.Code)
		// Should be minimum of 1 second
		assert.Equal(t, "1", rec.Header().Get("Retry-After"))
	})

	t.Run("does not set RetryAfter header when allowed", func(t *testing.T) {
		limiter := &mockLimiter{
			result: &ratelimit.Result{
				Allowed:    true,
				Remaining:  5,
				RetryAfter: 0,
				Limit:      10,
			},
		}

		mw := RateLimit(limiter, RateLimitConfig{})

		handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Empty(t, rec.Header().Get("Retry-After"))
	})
}
