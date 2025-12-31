package ratelimit

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMemoryLimiter_Allow(t *testing.T) {
	t.Run("allows requests under limit", func(t *testing.T) {
		cfg := Config{
			Requests: 5,
			Window:   time.Minute,
		}
		limiter := NewMemoryLimiter(cfg)
		defer limiter.Close()

		ctx := context.Background()
		identifier := "192.168.1.1"

		for i := 0; i < 5; i++ {
			result, err := limiter.Allow(ctx, identifier)
			require.NoError(t, err)
			assert.True(t, result.Allowed, "request %d should be allowed", i+1)
			assert.Equal(t, 5-i-1, result.Remaining)
		}
	})

	t.Run("blocks requests over limit", func(t *testing.T) {
		cfg := Config{
			Requests: 2,
			Window:   time.Minute,
		}
		limiter := NewMemoryLimiter(cfg)
		defer limiter.Close()

		ctx := context.Background()
		identifier := "192.168.1.1"

		// First two should be allowed
		for i := 0; i < 2; i++ {
			result, err := limiter.Allow(ctx, identifier)
			require.NoError(t, err)
			assert.True(t, result.Allowed)
		}

		// Third and fourth should be blocked
		for i := 0; i < 2; i++ {
			result, err := limiter.Allow(ctx, identifier)
			require.NoError(t, err)
			assert.False(t, result.Allowed, "request should be blocked")
			assert.Equal(t, 0, result.Remaining)
			assert.True(t, result.RetryAfter > 0, "should have retry-after duration")
		}
	})

	t.Run("different identifiers have separate limits", func(t *testing.T) {
		cfg := Config{
			Requests: 1,
			Window:   time.Minute,
		}
		limiter := NewMemoryLimiter(cfg)
		defer limiter.Close()

		ctx := context.Background()

		// IP1 uses its limit
		result1, err := limiter.Allow(ctx, "192.168.1.1")
		require.NoError(t, err)
		assert.True(t, result1.Allowed)

		// IP2 should still be allowed (separate limit)
		result2, err := limiter.Allow(ctx, "192.168.1.2")
		require.NoError(t, err)
		assert.True(t, result2.Allowed)

		// IP1 should now be blocked
		result3, err := limiter.Allow(ctx, "192.168.1.1")
		require.NoError(t, err)
		assert.False(t, result3.Allowed)
	})

	t.Run("old requests expire from window", func(t *testing.T) {
		cfg := Config{
			Requests: 2,
			Window:   100 * time.Millisecond,
		}
		limiter := NewMemoryLimiter(cfg)
		defer limiter.Close()

		ctx := context.Background()
		identifier := "192.168.1.1"

		// Use up the limit
		for i := 0; i < 2; i++ {
			result, err := limiter.Allow(ctx, identifier)
			require.NoError(t, err)
			assert.True(t, result.Allowed)
		}

		// Should be blocked now
		result, err := limiter.Allow(ctx, identifier)
		require.NoError(t, err)
		assert.False(t, result.Allowed)

		// Wait for window to expire
		time.Sleep(150 * time.Millisecond)

		// Should be allowed again
		result, err = limiter.Allow(ctx, identifier)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "request should be allowed after window expires")
	})

	t.Run("returns correct reset time", func(t *testing.T) {
		cfg := Config{
			Requests: 1,
			Window:   time.Second,
		}
		limiter := NewMemoryLimiter(cfg)
		defer limiter.Close()

		ctx := context.Background()
		identifier := "192.168.1.1"

		// Use up limit
		_, err := limiter.Allow(ctx, identifier)
		require.NoError(t, err)

		// Check reset time
		result, err := limiter.Allow(ctx, identifier)
		require.NoError(t, err)
		assert.False(t, result.Allowed)
		assert.True(t, result.ResetAfter > 0)
		assert.True(t, result.ResetAfter <= time.Second)
	})
}

func TestMemoryLimiter_Reset(t *testing.T) {
	t.Run("resets limit for identifier", func(t *testing.T) {
		cfg := Config{
			Requests: 1,
			Window:   time.Minute,
		}
		limiter := NewMemoryLimiter(cfg)
		defer limiter.Close()

		ctx := context.Background()
		identifier := "192.168.1.1"

		// Use up limit
		result, err := limiter.Allow(ctx, identifier)
		require.NoError(t, err)
		assert.True(t, result.Allowed)

		// Should be blocked
		result, err = limiter.Allow(ctx, identifier)
		require.NoError(t, err)
		assert.False(t, result.Allowed)

		// Reset
		err = limiter.Reset(ctx, identifier)
		require.NoError(t, err)

		// Should be allowed again
		result, err = limiter.Allow(ctx, identifier)
		require.NoError(t, err)
		assert.True(t, result.Allowed, "should be allowed after reset")
	})
}

func TestMemoryLimiter_Concurrency(t *testing.T) {
	t.Run("handles concurrent requests safely", func(t *testing.T) {
		cfg := Config{
			Requests: 100,
			Window:   time.Minute,
		}
		limiter := NewMemoryLimiter(cfg)
		defer limiter.Close()

		ctx := context.Background()
		identifier := "192.168.1.1"

		var wg sync.WaitGroup
		var allowed int64

		// Launch 200 concurrent requests
		for i := 0; i < 200; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				result, err := limiter.Allow(ctx, identifier)
				if err == nil && result.Allowed {
					atomic.AddInt64(&allowed, 1)
				}
			}()
		}

		wg.Wait()

		// Exactly 100 should be allowed
		assert.Equal(t, int64(100), allowed, "exactly limit requests should be allowed")
	})

	t.Run("handles concurrent requests for different identifiers", func(t *testing.T) {
		cfg := Config{
			Requests: 10,
			Window:   time.Minute,
		}
		limiter := NewMemoryLimiter(cfg)
		defer limiter.Close()

		ctx := context.Background()

		var wg sync.WaitGroup
		var totalAllowed int64

		// Launch concurrent requests for 10 different identifiers
		for id := 0; id < 10; id++ {
			identifier := string(rune('A' + id))
			for i := 0; i < 20; i++ {
				wg.Add(1)
				go func(id string) {
					defer wg.Done()
					result, err := limiter.Allow(ctx, id)
					if err == nil && result.Allowed {
						atomic.AddInt64(&totalAllowed, 1)
					}
				}(identifier)
			}
		}

		wg.Wait()

		// Each identifier should allow 10, so total = 100
		assert.Equal(t, int64(100), totalAllowed)
	})
}

func TestMemoryLimiter_ContextCancellation(t *testing.T) {
	t.Run("respects context cancellation", func(t *testing.T) {
		cfg := Config{
			Requests: 10,
			Window:   time.Minute,
		}
		limiter := NewMemoryLimiter(cfg)
		defer limiter.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		_, err := limiter.Allow(ctx, "test")
		assert.ErrorIs(t, err, context.Canceled)
	})
}
