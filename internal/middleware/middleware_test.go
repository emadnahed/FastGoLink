package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetRequestID(t *testing.T) {
	t.Run("returns request ID from context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), RequestIDKey, "test-123")
		assert.Equal(t, "test-123", GetRequestID(ctx))
	})

	t.Run("returns empty string when no request ID in context", func(t *testing.T) {
		ctx := context.Background()
		assert.Equal(t, "", GetRequestID(ctx))
	})

	t.Run("returns empty string when value is wrong type", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), RequestIDKey, 12345)
		assert.Equal(t, "", GetRequestID(ctx))
	})
}

func TestGetClientIP(t *testing.T) {
	t.Run("returns client IP from context", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ClientIPKey, "192.168.1.1")
		assert.Equal(t, "192.168.1.1", GetClientIP(ctx))
	})

	t.Run("returns empty string when no client IP in context", func(t *testing.T) {
		ctx := context.Background()
		assert.Equal(t, "", GetClientIP(ctx))
	})

	t.Run("returns empty string when value is wrong type", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), ClientIPKey, []byte("ip"))
		assert.Equal(t, "", GetClientIP(ctx))
	})
}

func TestChain_Then(t *testing.T) {
	t.Run("nil handler uses DefaultServeMux", func(t *testing.T) {
		chain := New()
		handler := chain.Then(nil)

		// Should not panic and should return DefaultServeMux
		assert.NotNil(t, handler)
	})

	t.Run("empty chain passes through to handler", func(t *testing.T) {
		chain := New()
		called := false

		handler := chain.Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.True(t, called)
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("single middleware wraps handler", func(t *testing.T) {
		order := []string{}

		mw := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "middleware")
				next.ServeHTTP(w, r)
			})
		}

		chain := New(mw)
		handler := chain.Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "handler")
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, []string{"middleware", "handler"}, order)
	})

	t.Run("multiple middlewares execute in order", func(t *testing.T) {
		order := []string{}

		mw1 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "mw1-before")
				next.ServeHTTP(w, r)
				order = append(order, "mw1-after")
			})
		}

		mw2 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "mw2-before")
				next.ServeHTTP(w, r)
				order = append(order, "mw2-after")
			})
		}

		chain := New(mw1, mw2)
		handler := chain.Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "handler")
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		expected := []string{"mw1-before", "mw2-before", "handler", "mw2-after", "mw1-after"}
		assert.Equal(t, expected, order)
	})

	t.Run("middleware can short-circuit", func(t *testing.T) {
		handlerCalled := false

		blockingMW := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusForbidden)
				// Does not call next.ServeHTTP
			})
		}

		chain := New(blockingMW)
		handler := chain.Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.False(t, handlerCalled)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})
}

func TestChain_ThenFunc(t *testing.T) {
	t.Run("wraps handler function", func(t *testing.T) {
		called := false

		chain := New()
		handler := chain.ThenFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.True(t, called)
	})
}

func TestChain_Append(t *testing.T) {
	t.Run("appends middleware to chain", func(t *testing.T) {
		order := []string{}

		mw1 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "mw1")
				next.ServeHTTP(w, r)
			})
		}

		mw2 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "mw2")
				next.ServeHTTP(w, r)
			})
		}

		chain := New(mw1)
		chain = chain.Append(mw2)

		handler := chain.Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "handler")
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, []string{"mw1", "mw2", "handler"}, order)
	})

	t.Run("does not modify original chain", func(t *testing.T) {
		mw1Called := false
		mw2Called := false

		mw1 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mw1Called = true
				next.ServeHTTP(w, r)
			})
		}

		mw2 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mw2Called = true
				next.ServeHTTP(w, r)
			})
		}

		original := New(mw1)
		_ = original.Append(mw2) // Create new chain but don't use it

		handler := original.Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.True(t, mw1Called)
		assert.False(t, mw2Called) // mw2 should NOT be called on original
	})
}

func TestChain_Extend(t *testing.T) {
	t.Run("extends chain with multiple middlewares", func(t *testing.T) {
		order := []string{}

		mw1 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "mw1")
				next.ServeHTTP(w, r)
			})
		}

		mw2 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "mw2")
				next.ServeHTTP(w, r)
			})
		}

		mw3 := func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, "mw3")
				next.ServeHTTP(w, r)
			})
		}

		chain := New(mw1)
		chain = chain.Extend(mw2, mw3)

		handler := chain.Then(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "handler")
		}))

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, []string{"mw1", "mw2", "mw3", "handler"}, order)
	})
}
