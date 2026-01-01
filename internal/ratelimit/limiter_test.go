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
	t.Run("respects context cancellation in Allow", func(t *testing.T) {
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

	t.Run("respects context cancellation in Reset", func(t *testing.T) {
		cfg := Config{
			Requests: 10,
			Window:   time.Minute,
		}
		limiter := NewMemoryLimiter(cfg)
		defer limiter.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := limiter.Reset(ctx, "test")
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.Equal(t, 100, cfg.Requests)
	assert.Equal(t, time.Minute, cfg.Window)
}

func TestMemoryLimiter_Cleanup(t *testing.T) {
	t.Run("cleanup removes expired entries", func(t *testing.T) {
		cfg := Config{
			Requests: 10,
			Window:   50 * time.Millisecond,
		}
		limiter := NewMemoryLimiter(cfg)
		defer limiter.Close()

		ctx := context.Background()

		// Make some requests
		_, _ = limiter.Allow(ctx, "user1")
		_, _ = limiter.Allow(ctx, "user2")

		// Wait for window to pass and cleanup to run
		time.Sleep(150 * time.Millisecond)

		// Make new request - old entries should be cleaned up
		result, err := limiter.Allow(ctx, "user1")
		require.NoError(t, err)
		assert.True(t, result.Allowed)
		// Should have full remaining since old entry was cleaned
		assert.Equal(t, 9, result.Remaining)
	})
}

func TestMemoryLimiter_Close(t *testing.T) {
	t.Run("close stops cleanup goroutine", func(t *testing.T) {
		cfg := Config{
			Requests: 10,
			Window:   time.Millisecond,
		}
		limiter := NewMemoryLimiter(cfg)

		// Use the limiter
		ctx := context.Background()
		_, _ = limiter.Allow(ctx, "test")

		// Close should return without error
		err := limiter.Close()
		assert.NoError(t, err)
	})
}

func TestMemoryLimiter_ResetAfterZero(t *testing.T) {
	t.Run("handles expired timestamps with zero reset time", func(t *testing.T) {
		cfg := Config{
			Requests: 5,
			Window:   10 * time.Millisecond,
		}
		limiter := NewMemoryLimiter(cfg)
		defer limiter.Close()

		ctx := context.Background()
		identifier := "test-user"

		// Make a request
		result, err := limiter.Allow(ctx, identifier)
		require.NoError(t, err)
		assert.True(t, result.Allowed)

		// Wait for the timestamp to expire
		time.Sleep(20 * time.Millisecond)

		// Make another request - old timestamp should be expired
		// and resetAfter calculation should handle negative value
		result, err = limiter.Allow(ctx, identifier)
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	})
}

func TestMemoryLimiter_CleanupKeepsValidEntries(t *testing.T) {
	t.Run("cleanup keeps entries with valid timestamps", func(t *testing.T) {
		cfg := Config{
			Requests: 10,
			Window:   100 * time.Millisecond,
		}
		limiter := NewMemoryLimiter(cfg)
		defer limiter.Close()

		ctx := context.Background()

		// Make requests to two users
		_, _ = limiter.Allow(ctx, "user1")
		_, _ = limiter.Allow(ctx, "user2")

		// Wait a bit but not enough for window to fully expire
		time.Sleep(30 * time.Millisecond)

		// Make more requests to keep user1 active
		_, _ = limiter.Allow(ctx, "user1")

		// Wait for cleanup to run (window duration)
		time.Sleep(120 * time.Millisecond)

		// user1 should still have an entry (recent request)
		// user2's first request might be cleaned but their entry might be removed entirely
		result, err := limiter.Allow(ctx, "user1")
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	})

	t.Run("cleanup retains partially expired entries", func(t *testing.T) {
		cfg := Config{
			Requests: 10,
			Window:   50 * time.Millisecond,
		}
		limiter := NewMemoryLimiter(cfg)
		defer limiter.Close()

		ctx := context.Background()

		// Make multiple requests
		for i := 0; i < 3; i++ {
			_, _ = limiter.Allow(ctx, "user")
			time.Sleep(10 * time.Millisecond)
		}

		// First request should be about to expire, last is recent
		// Trigger cleanup by waiting for window duration
		time.Sleep(60 * time.Millisecond)

		// Make new request - should still work and keep valid timestamps
		result, err := limiter.Allow(ctx, "user")
		require.NoError(t, err)
		assert.True(t, result.Allowed)
	})
}
