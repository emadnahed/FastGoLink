package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/emadnahed/FastGoLink/internal/config"
	"github.com/emadnahed/FastGoLink/internal/database"
	"github.com/emadnahed/FastGoLink/internal/models"
)

func skipIfNoPostgres(t *testing.T) {
	t.Helper()
	if os.Getenv("TEST_POSTGRES") != "true" {
		t.Skip("Skipping: TEST_POSTGRES not set. Run with docker-compose up -d")
	}
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func testDBConfig() *config.DatabaseConfig {
	return &config.DatabaseConfig{
		Host:            getEnvOrDefault("DB_HOST", "localhost"),
		Port:            5432,
		User:            getEnvOrDefault("DB_USER", "fastgolink"),
		Password:        getEnvOrDefault("DB_PASSWORD", "fastgolink_dev_password"),
		DBName:          getEnvOrDefault("DB_NAME", "fastgolink"),
		SSLMode:         "disable",
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
	}
}

func setupTestDB(t *testing.T) (*database.Pool, func()) {
	t.Helper()

	ctx := context.Background()
	cfg := testDBConfig()

	pool, err := database.NewPool(ctx, cfg)
	require.NoError(t, err)

	// Create urls table for tests
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

	cleanup := func() {
		_, _ = pool.Exec(ctx, "DELETE FROM urls")
		pool.Close()
	}

	return pool, cleanup
}

func TestPostgresURLRepository_Create(t *testing.T) {
	skipIfNoPostgres(t)

	pool, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewPostgresURLRepository(pool)
	ctx := context.Background()

	t.Run("create valid URL", func(t *testing.T) {
		create := &models.URLCreate{
			ShortCode:   "test123",
			OriginalURL: "https://example.com/test",
		}

		url, err := repo.Create(ctx, create)
		require.NoError(t, err)
		assert.NotZero(t, url.ID)
		assert.Equal(t, "test123", url.ShortCode)
		assert.Equal(t, "https://example.com/test", url.OriginalURL)
		assert.NotZero(t, url.CreatedAt)
		assert.Nil(t, url.ExpiresAt)
		assert.Zero(t, url.ClickCount)

		// Cleanup
		_ = repo.Delete(ctx, "test123")
	})

	t.Run("create with expiry", func(t *testing.T) {
		expiry := time.Now().Add(24 * time.Hour)
		create := &models.URLCreate{
			ShortCode:   "exp123",
			OriginalURL: "https://example.com/expiring",
			ExpiresAt:   &expiry,
		}

		url, err := repo.Create(ctx, create)
		require.NoError(t, err)
		assert.NotNil(t, url.ExpiresAt)

		// Cleanup
		_ = repo.Delete(ctx, "exp123")
	})

	t.Run("duplicate short code", func(t *testing.T) {
		create := &models.URLCreate{
			ShortCode:   "dup123",
			OriginalURL: "https://example.com/first",
		}

		_, err := repo.Create(ctx, create)
		require.NoError(t, err)

		// Try to create with same short code
		create2 := &models.URLCreate{
			ShortCode:   "dup123",
			OriginalURL: "https://example.com/second",
		}

		_, err = repo.Create(ctx, create2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")

		// Cleanup
		_ = repo.Delete(ctx, "dup123")
	})

	t.Run("invalid URL", func(t *testing.T) {
		create := &models.URLCreate{
			ShortCode:   "bad123",
			OriginalURL: "not-a-valid-url",
		}

		_, err := repo.Create(ctx, create)
		assert.ErrorIs(t, err, models.ErrInvalidURL)
	})
}

func TestPostgresURLRepository_GetByShortCode(t *testing.T) {
	skipIfNoPostgres(t)

	pool, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewPostgresURLRepository(pool)
	ctx := context.Background()

	t.Run("get existing URL", func(t *testing.T) {
		create := &models.URLCreate{
			ShortCode:   "get123",
			OriginalURL: "https://example.com/get",
		}
		_, err := repo.Create(ctx, create)
		require.NoError(t, err)

		url, err := repo.GetByShortCode(ctx, "get123")
		require.NoError(t, err)
		assert.Equal(t, "get123", url.ShortCode)
		assert.Equal(t, "https://example.com/get", url.OriginalURL)

		// Cleanup
		_ = repo.Delete(ctx, "get123")
	})

	t.Run("get non-existent URL", func(t *testing.T) {
		_, err := repo.GetByShortCode(ctx, "nonexistent")
		assert.ErrorIs(t, err, models.ErrURLNotFound)
	})
}

func TestPostgresURLRepository_Delete(t *testing.T) {
	skipIfNoPostgres(t)

	pool, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewPostgresURLRepository(pool)
	ctx := context.Background()

	t.Run("delete existing URL", func(t *testing.T) {
		create := &models.URLCreate{
			ShortCode:   "del123",
			OriginalURL: "https://example.com/delete",
		}
		_, err := repo.Create(ctx, create)
		require.NoError(t, err)

		err = repo.Delete(ctx, "del123")
		assert.NoError(t, err)

		// Verify it's gone
		_, err = repo.GetByShortCode(ctx, "del123")
		assert.ErrorIs(t, err, models.ErrURLNotFound)
	})

	t.Run("delete non-existent URL", func(t *testing.T) {
		err := repo.Delete(ctx, "nonexistent")
		assert.ErrorIs(t, err, models.ErrURLNotFound)
	})
}

func TestPostgresURLRepository_IncrementClickCount(t *testing.T) {
	skipIfNoPostgres(t)

	pool, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewPostgresURLRepository(pool)
	ctx := context.Background()

	t.Run("increment click count", func(t *testing.T) {
		create := &models.URLCreate{
			ShortCode:   "click1",
			OriginalURL: "https://example.com/click",
		}
		_, err := repo.Create(ctx, create)
		require.NoError(t, err)

		// Increment multiple times
		for i := 0; i < 5; i++ {
			err = repo.IncrementClickCount(ctx, "click1")
			require.NoError(t, err)
		}

		url, err := repo.GetByShortCode(ctx, "click1")
		require.NoError(t, err)
		assert.Equal(t, int64(5), url.ClickCount)

		// Cleanup
		_ = repo.Delete(ctx, "click1")
	})

	t.Run("increment non-existent URL", func(t *testing.T) {
		err := repo.IncrementClickCount(ctx, "nonexistent")
		assert.ErrorIs(t, err, models.ErrURLNotFound)
	})
}

func TestPostgresURLRepository_DeleteExpired(t *testing.T) {
	skipIfNoPostgres(t)

	pool, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewPostgresURLRepository(pool)
	ctx := context.Background()

	// Create expired URL
	expiredTime := time.Now().Add(-1 * time.Hour)
	expired := &models.URLCreate{
		ShortCode:   "expired1",
		OriginalURL: "https://example.com/expired",
		ExpiresAt:   &expiredTime,
	}
	_, err := repo.Create(ctx, expired)
	require.NoError(t, err)

	// Create non-expired URL
	futureTime := time.Now().Add(24 * time.Hour)
	notExpired := &models.URLCreate{
		ShortCode:   "future1",
		OriginalURL: "https://example.com/future",
		ExpiresAt:   &futureTime,
	}
	_, err = repo.Create(ctx, notExpired)
	require.NoError(t, err)

	// Create URL without expiry
	noExpiry := &models.URLCreate{
		ShortCode:   "noexp1",
		OriginalURL: "https://example.com/noexpiry",
	}
	_, err = repo.Create(ctx, noExpiry)
	require.NoError(t, err)

	// Delete expired
	count, err := repo.DeleteExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Verify expired is gone
	_, err = repo.GetByShortCode(ctx, "expired1")
	assert.ErrorIs(t, err, models.ErrURLNotFound)

	// Verify others still exist
	_, err = repo.GetByShortCode(ctx, "future1")
	assert.NoError(t, err)

	_, err = repo.GetByShortCode(ctx, "noexp1")
	assert.NoError(t, err)

	// Cleanup
	_ = repo.Delete(ctx, "future1")
	_ = repo.Delete(ctx, "noexp1")
}

func TestPostgresURLRepository_Exists(t *testing.T) {
	skipIfNoPostgres(t)

	pool, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewPostgresURLRepository(pool)
	ctx := context.Background()

	t.Run("exists returns true for existing", func(t *testing.T) {
		create := &models.URLCreate{
			ShortCode:   "exists1",
			OriginalURL: "https://example.com/exists",
		}
		_, err := repo.Create(ctx, create)
		require.NoError(t, err)

		exists, err := repo.Exists(ctx, "exists1")
		require.NoError(t, err)
		assert.True(t, exists)

		// Cleanup
		_ = repo.Delete(ctx, "exists1")
	})

	t.Run("exists returns false for non-existing", func(t *testing.T) {
		exists, err := repo.Exists(ctx, "nonexistent")
		require.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestPostgresURLRepository_GetByID(t *testing.T) {
	skipIfNoPostgres(t)

	pool, cleanup := setupTestDB(t)
	defer cleanup()

	repo := NewPostgresURLRepository(pool)
	ctx := context.Background()

	t.Run("get by ID", func(t *testing.T) {
		create := &models.URLCreate{
			ShortCode:   "byid1",
			OriginalURL: "https://example.com/byid",
		}
		created, err := repo.Create(ctx, create)
		require.NoError(t, err)

		url, err := repo.GetByID(ctx, created.ID)
		require.NoError(t, err)
		assert.Equal(t, created.ID, url.ID)
		assert.Equal(t, "byid1", url.ShortCode)

		// Cleanup
		_ = repo.Delete(ctx, "byid1")
	})

	t.Run("get by non-existent ID", func(t *testing.T) {
		_, err := repo.GetByID(ctx, 999999)
		assert.ErrorIs(t, err, models.ErrURLNotFound)
	})
}
