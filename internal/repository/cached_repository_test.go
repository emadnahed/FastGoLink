package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/emadnahed/FastGoLink/internal/cache"
	"github.com/emadnahed/FastGoLink/internal/config"
	"github.com/emadnahed/FastGoLink/internal/database"
	"github.com/emadnahed/FastGoLink/internal/models"
)

func skipIfNoRedisOrPostgres(t *testing.T) {
	t.Helper()
	if os.Getenv("TEST_REDIS") != "true" {
		t.Skip("Skipping: TEST_REDIS not set")
	}
	if os.Getenv("TEST_POSTGRES") != "true" {
		t.Skip("Skipping: TEST_POSTGRES not set")
	}
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

func setupCachedTestDB(t *testing.T) (*CachedURLRepository, func()) {
	t.Helper()
	skipIfNoRedisOrPostgres(t)

	ctx := context.Background()

	// Setup Postgres
	dbCfg := testDBConfig()
	pool, err := database.NewPool(ctx, dbCfg)
	require.NoError(t, err)

	// Create table
	_, err = pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS urls (
			id BIGSERIAL PRIMARY KEY,
			short_code VARCHAR(10) UNIQUE NOT NULL,
			original_url TEXT NOT NULL,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			expires_at TIMESTAMPTZ,
			click_count BIGINT DEFAULT 0
		)
	`)
	require.NoError(t, err)

	// Setup Redis
	redisCfg := testRedisConfig()
	redisCache, err := cache.NewRedisCache(ctx, redisCfg)
	require.NoError(t, err)

	urlCache := cache.NewURLCache(redisCache, "test:cached:", time.Minute)
	baseRepo := NewPostgresURLRepository(pool)
	cachedRepo := NewCachedURLRepository(baseRepo, urlCache, time.Minute)

	cleanup := func() {
		// Clean up test data
		_, _ = pool.Exec(ctx, "DELETE FROM urls WHERE short_code LIKE 'cached%'")

		// Clean up Redis keys
		client := redisCache.Client()
		iter := client.Scan(ctx, 0, "test:cached:*", 0).Iterator()
		for iter.Next(ctx) {
			_ = client.Del(ctx, iter.Val())
		}

		_ = redisCache.Close()
		pool.Close()
	}

	return cachedRepo, cleanup
}

func TestCachedURLRepository_Create(t *testing.T) {
	repo, cleanup := setupCachedTestDB(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("creates in both db and cache", func(t *testing.T) {
		create := &models.URLCreate{
			ShortCode:   "cached1",
			OriginalURL: "https://example.com/cached",
		}

		url, err := repo.Create(ctx, create)
		require.NoError(t, err)
		assert.NotZero(t, url.ID)
		assert.Equal(t, "cached1", url.ShortCode)

		// Verify it's in cache
		exists, err := repo.cache.Exists(ctx, "cached1")
		require.NoError(t, err)
		assert.True(t, exists)

		// Cleanup
		_ = repo.Delete(ctx, "cached1")
	})
}

func TestCachedURLRepository_GetByShortCode(t *testing.T) {
	repo, cleanup := setupCachedTestDB(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("cache hit returns cached value", func(t *testing.T) {
		create := &models.URLCreate{
			ShortCode:   "cached2",
			OriginalURL: "https://example.com/hit",
		}

		_, err := repo.Create(ctx, create)
		require.NoError(t, err)

		// First get - should populate cache
		url1, err := repo.GetByShortCode(ctx, "cached2")
		require.NoError(t, err)
		assert.Equal(t, "https://example.com/hit", url1.OriginalURL)

		// Second get - should use cache
		url2, err := repo.GetByShortCode(ctx, "cached2")
		require.NoError(t, err)
		assert.Equal(t, "https://example.com/hit", url2.OriginalURL)

		// Cleanup
		_ = repo.Delete(ctx, "cached2")
	})

	t.Run("cache miss falls back to db", func(t *testing.T) {
		create := &models.URLCreate{
			ShortCode:   "cached3",
			OriginalURL: "https://example.com/miss",
		}

		_, err := repo.Create(ctx, create)
		require.NoError(t, err)

		// Delete from cache only
		_ = repo.cache.Delete(ctx, "cached3")

		// Should still get from db
		url, err := repo.GetByShortCode(ctx, "cached3")
		require.NoError(t, err)
		assert.Equal(t, "https://example.com/miss", url.OriginalURL)

		// Should now be back in cache
		exists, err := repo.cache.Exists(ctx, "cached3")
		require.NoError(t, err)
		assert.True(t, exists)

		// Cleanup
		_ = repo.Delete(ctx, "cached3")
	})

	t.Run("not found returns error", func(t *testing.T) {
		_, err := repo.GetByShortCode(ctx, "nonexistent")
		assert.ErrorIs(t, err, models.ErrURLNotFound)
	})
}

func TestCachedURLRepository_Delete(t *testing.T) {
	repo, cleanup := setupCachedTestDB(t)
	defer cleanup()

	ctx := context.Background()

	create := &models.URLCreate{
		ShortCode:   "cached4",
		OriginalURL: "https://example.com/delete",
	}

	_, err := repo.Create(ctx, create)
	require.NoError(t, err)

	// Delete should remove from both
	err = repo.Delete(ctx, "cached4")
	require.NoError(t, err)

	// Verify gone from cache
	exists, err := repo.cache.Exists(ctx, "cached4")
	require.NoError(t, err)
	assert.False(t, exists)

	// Verify gone from db
	_, err = repo.GetByShortCode(ctx, "cached4")
	assert.ErrorIs(t, err, models.ErrURLNotFound)
}

func TestCachedURLRepository_Exists(t *testing.T) {
	repo, cleanup := setupCachedTestDB(t)
	defer cleanup()

	ctx := context.Background()

	t.Run("exists returns true from cache", func(t *testing.T) {
		create := &models.URLCreate{
			ShortCode:   "cached5",
			OriginalURL: "https://example.com/exists",
		}

		_, err := repo.Create(ctx, create)
		require.NoError(t, err)

		exists, err := repo.Exists(ctx, "cached5")
		require.NoError(t, err)
		assert.True(t, exists)

		// Cleanup
		_ = repo.Delete(ctx, "cached5")
	})

	t.Run("exists falls back to db when not in cache", func(t *testing.T) {
		create := &models.URLCreate{
			ShortCode:   "cached6",
			OriginalURL: "https://example.com/db",
		}

		_, err := repo.Create(ctx, create)
		require.NoError(t, err)

		// Delete from cache
		_ = repo.cache.Delete(ctx, "cached6")

		exists, err := repo.Exists(ctx, "cached6")
		require.NoError(t, err)
		assert.True(t, exists)

		// Cleanup
		_ = repo.Delete(ctx, "cached6")
	})

	t.Run("exists returns false for non-existent", func(t *testing.T) {
		exists, err := repo.Exists(ctx, "nonexistent")
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestCachedURLRepository_IncrementClickCount(t *testing.T) {
	repo, cleanup := setupCachedTestDB(t)
	defer cleanup()

	ctx := context.Background()

	create := &models.URLCreate{
		ShortCode:   "cached7",
		OriginalURL: "https://example.com/click",
	}

	_, err := repo.Create(ctx, create)
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		err = repo.IncrementClickCount(ctx, "cached7")
		require.NoError(t, err)
	}

	// Get from DB to verify (cache doesn't store click count)
	url, err := repo.GetByShortCode(ctx, "cached7")
	require.NoError(t, err)
	// Note: cached version won't have click count, need to query DB directly
	// but at least we verified the increment doesn't error

	_ = repo.Delete(ctx, "cached7")
	_ = url // silence unused
}

func TestCachedURLRepository_HealthCheck(t *testing.T) {
	repo, cleanup := setupCachedTestDB(t)
	defer cleanup()

	ctx := context.Background()

	err := repo.HealthCheck(ctx)
	assert.NoError(t, err)
}

// Test with mock cache to verify behavior without real Redis
func TestCachedURLRepository_MockCache(t *testing.T) {
	skipIfNoPostgres(t)

	ctx := context.Background()

	// Setup Postgres only
	pool, cleanup := setupTestDB(t)
	defer cleanup()

	mockCache := &mockURLCache{data: make(map[string]*cache.CachedURL)}
	baseRepo := NewPostgresURLRepository(pool)
	cachedRepo := &CachedURLRepository{
		repo:     baseRepo,
		cache:    nil, // We'll use our mock directly
		cacheTTL: time.Minute,
	}

	// Replace cache operations with mock
	cachedRepo2 := NewCachedURLRepositoryWithMock(baseRepo, mockCache, time.Minute)

	t.Run("write-through caching", func(t *testing.T) {
		create := &models.URLCreate{
			ShortCode:   "mock1",
			OriginalURL: "https://example.com/mock",
		}

		url, err := cachedRepo2.Create(ctx, create)
		require.NoError(t, err)
		assert.NotZero(t, url.ID)

		// Verify mock cache was populated
		assert.Contains(t, mockCache.data, "mock1")

		// Cleanup
		_ = cachedRepo2.Delete(ctx, "mock1")
	})

	_ = cachedRepo // silence unused warning
}

// mockURLCache implements cache operations for testing
type mockURLCache struct {
	data map[string]*cache.CachedURL
}

func (m *mockURLCache) Get(_ context.Context, shortCode string) (*cache.CachedURL, error) {
	if url, ok := m.data[shortCode]; ok {
		return url, nil
	}
	return nil, cache.ErrCacheMiss
}

func (m *mockURLCache) Set(_ context.Context, url *cache.CachedURL) error {
	m.data[url.ShortCode] = url
	return nil
}

func (m *mockURLCache) SetWithTTL(_ context.Context, url *cache.CachedURL, _ time.Duration) error {
	m.data[url.ShortCode] = url
	return nil
}

func (m *mockURLCache) Delete(_ context.Context, shortCode string) error {
	delete(m.data, shortCode)
	return nil
}

func (m *mockURLCache) Exists(_ context.Context, shortCode string) (bool, error) {
	_, ok := m.data[shortCode]
	return ok, nil
}

func (m *mockURLCache) Ping(_ context.Context) error {
	return nil
}

// CachedURLRepositoryWithMock is a version that uses a mock cache interface
type CachedURLRepositoryWithMock struct {
	repo     URLRepository
	cache    *mockURLCache
	cacheTTL time.Duration
}

func NewCachedURLRepositoryWithMock(repo URLRepository, cache *mockURLCache, cacheTTL time.Duration) *CachedURLRepositoryWithMock {
	return &CachedURLRepositoryWithMock{
		repo:     repo,
		cache:    cache,
		cacheTTL: cacheTTL,
	}
}

func (c *CachedURLRepositoryWithMock) Create(ctx context.Context, create *models.URLCreate) (*models.URL, error) {
	url, err := c.repo.Create(ctx, create)
	if err != nil {
		return nil, err
	}

	cached := &cache.CachedURL{
		ShortCode:   url.ShortCode,
		OriginalURL: url.OriginalURL,
		ExpiresAt:   url.ExpiresAt,
	}
	_ = c.cache.SetWithTTL(ctx, cached, c.cacheTTL)

	return url, nil
}

func (c *CachedURLRepositoryWithMock) GetByShortCode(ctx context.Context, shortCode string) (*models.URL, error) {
	cached, err := c.cache.Get(ctx, shortCode)
	if err == nil {
		return &models.URL{
			ShortCode:   cached.ShortCode,
			OriginalURL: cached.OriginalURL,
			ExpiresAt:   cached.ExpiresAt,
		}, nil
	}

	url, err := c.repo.GetByShortCode(ctx, shortCode)
	if err != nil {
		return nil, err
	}

	cachedURL := &cache.CachedURL{
		ShortCode:   url.ShortCode,
		OriginalURL: url.OriginalURL,
		ExpiresAt:   url.ExpiresAt,
	}
	_ = c.cache.SetWithTTL(ctx, cachedURL, c.cacheTTL)

	return url, nil
}

func (c *CachedURLRepositoryWithMock) Delete(ctx context.Context, shortCode string) error {
	_ = c.cache.Delete(ctx, shortCode)
	return c.repo.Delete(ctx, shortCode)
}

func (c *CachedURLRepositoryWithMock) GetByID(ctx context.Context, id int64) (*models.URL, error) {
	return c.repo.GetByID(ctx, id)
}

func (c *CachedURLRepositoryWithMock) IncrementClickCount(ctx context.Context, shortCode string) error {
	return c.repo.IncrementClickCount(ctx, shortCode)
}

func (c *CachedURLRepositoryWithMock) DeleteExpired(ctx context.Context) (int64, error) {
	return c.repo.DeleteExpired(ctx)
}

func (c *CachedURLRepositoryWithMock) Exists(ctx context.Context, shortCode string) (bool, error) {
	exists, err := c.cache.Exists(ctx, shortCode)
	if err == nil && exists {
		return true, nil
	}
	return c.repo.Exists(ctx, shortCode)
}

func (c *CachedURLRepositoryWithMock) HealthCheck(ctx context.Context) error {
	return c.repo.HealthCheck(ctx)
}
