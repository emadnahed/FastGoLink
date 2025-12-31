package cache

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gourl/gourl/internal/config"
)

func skipIfNoRedis(t *testing.T) {
	t.Helper()
	if os.Getenv("TEST_REDIS") != "true" {
		t.Skip("Skipping: TEST_REDIS not set. Run with docker-compose up -d")
	}
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func testRedisConfig() *config.RedisConfig {
	return &config.RedisConfig{
		Host:     getEnvOrDefault("REDIS_HOST", "localhost"),
		Port:     6379,
		Password: getEnvOrDefault("REDIS_PASSWORD", ""),
		DB:       0,
		PoolSize: 10,
	}
}

func setupTestRedis(t *testing.T) (*RedisCache, func()) {
	t.Helper()
	skipIfNoRedis(t)

	ctx := context.Background()
	cfg := testRedisConfig()

	cache, err := NewRedisCache(ctx, cfg)
	require.NoError(t, err)

	cleanup := func() {
		// Clean up test keys
		client := cache.Client()
		iter := client.Scan(ctx, 0, "test:*", 0).Iterator()
		for iter.Next(ctx) {
			_ = client.Del(ctx, iter.Val())
		}
		_ = cache.Close()
	}

	return cache, cleanup
}

func TestNewRedisCache(t *testing.T) {
	skipIfNoRedis(t)

	ctx := context.Background()
	cfg := testRedisConfig()

	cache, err := NewRedisCache(ctx, cfg)
	require.NoError(t, err)
	defer cache.Close()

	assert.NotNil(t, cache)
	assert.NotNil(t, cache.Client())
}

func TestNewRedisCache_InvalidHost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cfg := &config.RedisConfig{
		Host:     "invalid-host-that-does-not-exist",
		Port:     6379,
		Password: "",
		DB:       0,
		PoolSize: 1,
	}

	_, err := NewRedisCache(ctx, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect to Redis")
}

func TestRedisCache_SetAndGet(t *testing.T) {
	cache, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("set and get value", func(t *testing.T) {
		key := "test:setget1"
		value := []byte("hello world")

		err := cache.Set(ctx, key, value, time.Minute)
		require.NoError(t, err)

		got, err := cache.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, value, got)
	})

	t.Run("get non-existent key", func(t *testing.T) {
		_, err := cache.Get(ctx, "test:nonexistent")
		assert.ErrorIs(t, err, ErrCacheMiss)
	})

	t.Run("set with TTL expiry", func(t *testing.T) {
		key := "test:ttl1"
		value := []byte("expires soon")

		err := cache.Set(ctx, key, value, 100*time.Millisecond)
		require.NoError(t, err)

		// Should exist immediately
		got, err := cache.Get(ctx, key)
		require.NoError(t, err)
		assert.Equal(t, value, got)

		// Wait for expiry
		time.Sleep(150 * time.Millisecond)

		_, err = cache.Get(ctx, key)
		assert.ErrorIs(t, err, ErrCacheMiss)
	})
}

func TestRedisCache_Delete(t *testing.T) {
	cache, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("delete existing key", func(t *testing.T) {
		key := "test:del1"
		value := []byte("to be deleted")

		err := cache.Set(ctx, key, value, time.Minute)
		require.NoError(t, err)

		err = cache.Delete(ctx, key)
		require.NoError(t, err)

		_, err = cache.Get(ctx, key)
		assert.ErrorIs(t, err, ErrCacheMiss)
	})

	t.Run("delete non-existent key (no error)", func(t *testing.T) {
		err := cache.Delete(ctx, "test:nonexistent")
		assert.NoError(t, err)
	})
}

func TestRedisCache_Exists(t *testing.T) {
	cache, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("exists returns true for existing key", func(t *testing.T) {
		key := "test:exists1"
		err := cache.Set(ctx, key, []byte("value"), time.Minute)
		require.NoError(t, err)

		exists, err := cache.Exists(ctx, key)
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("exists returns false for non-existent key", func(t *testing.T) {
		exists, err := cache.Exists(ctx, "test:nonexistent")
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestRedisCache_Ping(t *testing.T) {
	cache, cleanup := setupTestRedis(t)
	defer cleanup()

	ctx := context.Background()

	err := cache.Ping(ctx)
	assert.NoError(t, err)
}

// URLCache tests

func TestNewURLCache(t *testing.T) {
	t.Run("with defaults", func(t *testing.T) {
		mockCache := &MockCache{}
		urlCache := NewURLCache(mockCache, "", 0)

		assert.Equal(t, "url:", urlCache.keyPrefix)
		assert.Equal(t, 24*time.Hour, urlCache.defaultTTL)
	})

	t.Run("with custom values", func(t *testing.T) {
		mockCache := &MockCache{}
		urlCache := NewURLCache(mockCache, "custom:", 1*time.Hour)

		assert.Equal(t, "custom:", urlCache.keyPrefix)
		assert.Equal(t, 1*time.Hour, urlCache.defaultTTL)
	})
}

func TestURLCache_SetAndGet(t *testing.T) {
	cache, cleanup := setupTestRedis(t)
	defer cleanup()

	urlCache := NewURLCache(cache, "test:url:", time.Minute)
	ctx := context.Background()

	t.Run("set and get URL", func(t *testing.T) {
		url := &CachedURL{
			ShortCode:   "abc123",
			OriginalURL: "https://example.com/test",
		}

		err := urlCache.Set(ctx, url)
		require.NoError(t, err)

		got, err := urlCache.Get(ctx, "abc123")
		require.NoError(t, err)
		assert.Equal(t, "abc123", got.ShortCode)
		assert.Equal(t, "https://example.com/test", got.OriginalURL)
		assert.Nil(t, got.ExpiresAt)
	})

	t.Run("set and get URL with expiry", func(t *testing.T) {
		expiry := time.Now().Add(24 * time.Hour)
		url := &CachedURL{
			ShortCode:   "exp123",
			OriginalURL: "https://example.com/expiring",
			ExpiresAt:   &expiry,
		}

		err := urlCache.Set(ctx, url)
		require.NoError(t, err)

		got, err := urlCache.Get(ctx, "exp123")
		require.NoError(t, err)
		assert.NotNil(t, got.ExpiresAt)
	})

	t.Run("get non-existent URL", func(t *testing.T) {
		_, err := urlCache.Get(ctx, "nonexistent")
		assert.ErrorIs(t, err, ErrCacheMiss)
	})

	t.Run("get expired URL returns error", func(t *testing.T) {
		// Create URL that's already expired
		expiry := time.Now().Add(-1 * time.Hour)
		url := &CachedURL{
			ShortCode:   "pastexp",
			OriginalURL: "https://example.com/past",
			ExpiresAt:   &expiry,
		}

		// Force set it directly (bypassing the expiry check in SetWithTTL)
		data, _ := json.Marshal(url)
		err := cache.Set(ctx, "test:url:pastexp", data, time.Minute)
		require.NoError(t, err)

		_, err = urlCache.Get(ctx, "pastexp")
		assert.ErrorIs(t, err, ErrCacheExpired)

		// Should also be deleted
		exists, _ := urlCache.Exists(ctx, "pastexp")
		assert.False(t, exists)
	})
}

func TestURLCache_SetWithTTL(t *testing.T) {
	cache, cleanup := setupTestRedis(t)
	defer cleanup()

	urlCache := NewURLCache(cache, "test:url:", time.Hour)
	ctx := context.Background()

	t.Run("set with custom TTL", func(t *testing.T) {
		url := &CachedURL{
			ShortCode:   "ttl123",
			OriginalURL: "https://example.com/ttl",
		}

		err := urlCache.SetWithTTL(ctx, url, 100*time.Millisecond)
		require.NoError(t, err)

		// Should exist immediately
		_, err = urlCache.Get(ctx, "ttl123")
		require.NoError(t, err)

		// Wait for expiry
		time.Sleep(150 * time.Millisecond)

		_, err = urlCache.Get(ctx, "ttl123")
		assert.ErrorIs(t, err, ErrCacheMiss)
	})

	t.Run("TTL capped to URL expiry", func(t *testing.T) {
		expiry := time.Now().Add(100 * time.Millisecond)
		url := &CachedURL{
			ShortCode:   "cap123",
			OriginalURL: "https://example.com/capped",
			ExpiresAt:   &expiry,
		}

		// Set with much longer TTL
		err := urlCache.SetWithTTL(ctx, url, time.Hour)
		require.NoError(t, err)

		// Wait for URL expiry
		time.Sleep(150 * time.Millisecond)

		// Should be expired in Redis
		_, err = urlCache.Get(ctx, "cap123")
		assert.Error(t, err)
	})

	t.Run("already expired URL not cached", func(t *testing.T) {
		expiry := time.Now().Add(-1 * time.Hour)
		url := &CachedURL{
			ShortCode:   "noset1",
			OriginalURL: "https://example.com/noset",
			ExpiresAt:   &expiry,
		}

		err := urlCache.SetWithTTL(ctx, url, time.Hour)
		require.NoError(t, err) // No error, just doesn't cache

		exists, err := urlCache.Exists(ctx, "noset1")
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestURLCache_Delete(t *testing.T) {
	cache, cleanup := setupTestRedis(t)
	defer cleanup()

	urlCache := NewURLCache(cache, "test:url:", time.Minute)
	ctx := context.Background()

	url := &CachedURL{
		ShortCode:   "del123",
		OriginalURL: "https://example.com/delete",
	}

	err := urlCache.Set(ctx, url)
	require.NoError(t, err)

	err = urlCache.Delete(ctx, "del123")
	require.NoError(t, err)

	_, err = urlCache.Get(ctx, "del123")
	assert.ErrorIs(t, err, ErrCacheMiss)
}

func TestURLCache_Exists(t *testing.T) {
	cache, cleanup := setupTestRedis(t)
	defer cleanup()

	urlCache := NewURLCache(cache, "test:url:", time.Minute)
	ctx := context.Background()

	t.Run("exists returns true for cached URL", func(t *testing.T) {
		url := &CachedURL{
			ShortCode:   "ex123",
			OriginalURL: "https://example.com/exists",
		}

		err := urlCache.Set(ctx, url)
		require.NoError(t, err)

		exists, err := urlCache.Exists(ctx, "ex123")
		require.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("exists returns false for non-cached URL", func(t *testing.T) {
		exists, err := urlCache.Exists(ctx, "nonexistent")
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestURLCache_Ping(t *testing.T) {
	cache, cleanup := setupTestRedis(t)
	defer cleanup()

	urlCache := NewURLCache(cache, "test:url:", time.Minute)
	ctx := context.Background()

	err := urlCache.Ping(ctx)
	assert.NoError(t, err)
}

// MockCache for testing URLCache with custom behaviors
type MockCache struct {
	data   map[string][]byte
	closed bool
}

func (m *MockCache) Get(_ context.Context, key string) ([]byte, error) {
	if m.data == nil {
		return nil, ErrCacheMiss
	}
	val, ok := m.data[key]
	if !ok {
		return nil, ErrCacheMiss
	}
	return val, nil
}

func (m *MockCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	if m.data == nil {
		m.data = make(map[string][]byte)
	}
	m.data[key] = value
	return nil
}

func (m *MockCache) Delete(_ context.Context, key string) error {
	delete(m.data, key)
	return nil
}

func (m *MockCache) Exists(_ context.Context, key string) (bool, error) {
	if m.data == nil {
		return false, nil
	}
	_, ok := m.data[key]
	return ok, nil
}

func (m *MockCache) Ping(_ context.Context) error {
	return nil
}

func (m *MockCache) Close() error {
	m.closed = true
	return nil
}
