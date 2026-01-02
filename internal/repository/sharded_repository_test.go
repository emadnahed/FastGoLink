package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/emadnahed/FastGoLink/internal/database"
	"github.com/emadnahed/FastGoLink/internal/models"
)

func setupShardedTestDB(t *testing.T) (*database.ShardRouter, func()) {
	t.Helper()
	skipIfNoPostgres(t)

	ctx := context.Background()
	cfg := testDBConfig()

	router, err := database.SingleShardRouter(ctx, cfg)
	require.NoError(t, err)

	// Create urls table for tests
	pool := router.GetShard("any")
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
		router.Close()
	}

	return router, cleanup
}

func TestShardedURLRepository_Create(t *testing.T) {
	router, cleanup := setupShardedTestDB(t)
	defer cleanup()

	repo := NewShardedURLRepository(router)
	ctx := context.Background()

	t.Run("create valid URL", func(t *testing.T) {
		create := &models.URLCreate{
			ShortCode:   "shard1",
			OriginalURL: "https://example.com/sharded",
		}

		url, err := repo.Create(ctx, create)
		require.NoError(t, err)
		assert.NotZero(t, url.ID)
		assert.Equal(t, "shard1", url.ShortCode)

		// Cleanup
		_ = repo.Delete(ctx, "shard1")
	})
}

func TestShardedURLRepository_GetByShortCode(t *testing.T) {
	router, cleanup := setupShardedTestDB(t)
	defer cleanup()

	repo := NewShardedURLRepository(router)
	ctx := context.Background()

	t.Run("get existing URL", func(t *testing.T) {
		create := &models.URLCreate{
			ShortCode:   "shget1",
			OriginalURL: "https://example.com/shget",
		}
		_, err := repo.Create(ctx, create)
		require.NoError(t, err)

		url, err := repo.GetByShortCode(ctx, "shget1")
		require.NoError(t, err)
		assert.Equal(t, "shget1", url.ShortCode)

		// Cleanup
		_ = repo.Delete(ctx, "shget1")
	})

	t.Run("get non-existent URL", func(t *testing.T) {
		_, err := repo.GetByShortCode(ctx, "nonexistent")
		assert.ErrorIs(t, err, models.ErrURLNotFound)
	})
}

func TestShardedURLRepository_Delete(t *testing.T) {
	router, cleanup := setupShardedTestDB(t)
	defer cleanup()

	repo := NewShardedURLRepository(router)
	ctx := context.Background()

	create := &models.URLCreate{
		ShortCode:   "shdel1",
		OriginalURL: "https://example.com/delete",
	}
	_, err := repo.Create(ctx, create)
	require.NoError(t, err)

	err = repo.Delete(ctx, "shdel1")
	assert.NoError(t, err)

	_, err = repo.GetByShortCode(ctx, "shdel1")
	assert.ErrorIs(t, err, models.ErrURLNotFound)
}

func TestShardedURLRepository_IncrementClickCount(t *testing.T) {
	router, cleanup := setupShardedTestDB(t)
	defer cleanup()

	repo := NewShardedURLRepository(router)
	ctx := context.Background()

	create := &models.URLCreate{
		ShortCode:   "shclk1",
		OriginalURL: "https://example.com/click",
	}
	_, err := repo.Create(ctx, create)
	require.NoError(t, err)

	for i := 0; i < 3; i++ {
		err = repo.IncrementClickCount(ctx, "shclk1")
		require.NoError(t, err)
	}

	url, err := repo.GetByShortCode(ctx, "shclk1")
	require.NoError(t, err)
	assert.Equal(t, int64(3), url.ClickCount)

	// Cleanup
	_ = repo.Delete(ctx, "shclk1")
}

func TestShardedURLRepository_Exists(t *testing.T) {
	router, cleanup := setupShardedTestDB(t)
	defer cleanup()

	repo := NewShardedURLRepository(router)
	ctx := context.Background()

	create := &models.URLCreate{
		ShortCode:   "shex1",
		OriginalURL: "https://example.com/exists",
	}
	_, err := repo.Create(ctx, create)
	require.NoError(t, err)

	exists, err := repo.Exists(ctx, "shex1")
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = repo.Exists(ctx, "nonexistent")
	require.NoError(t, err)
	assert.False(t, exists)

	// Cleanup
	_ = repo.Delete(ctx, "shex1")
}

func TestShardedURLRepository_DeleteExpired(t *testing.T) {
	router, cleanup := setupShardedTestDB(t)
	defer cleanup()

	repo := NewShardedURLRepository(router)
	ctx := context.Background()

	// Create expired URL
	expiredTime := time.Now().Add(-1 * time.Hour)
	expired := &models.URLCreate{
		ShortCode:   "shexp1",
		OriginalURL: "https://example.com/expired",
		ExpiresAt:   &expiredTime,
	}
	_, err := repo.Create(ctx, expired)
	require.NoError(t, err)

	// Create non-expired URL
	futureTime := time.Now().Add(24 * time.Hour)
	notExpired := &models.URLCreate{
		ShortCode:   "shfut1",
		OriginalURL: "https://example.com/future",
		ExpiresAt:   &futureTime,
	}
	_, err = repo.Create(ctx, notExpired)
	require.NoError(t, err)

	count, err := repo.DeleteExpired(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Verify expired is gone
	_, err = repo.GetByShortCode(ctx, "shexp1")
	assert.ErrorIs(t, err, models.ErrURLNotFound)

	// Verify future still exists
	_, err = repo.GetByShortCode(ctx, "shfut1")
	assert.NoError(t, err)

	// Cleanup
	_ = repo.Delete(ctx, "shfut1")
}

func TestShardedURLRepository_HealthCheck(t *testing.T) {
	router, cleanup := setupShardedTestDB(t)
	defer cleanup()

	repo := NewShardedURLRepository(router)
	ctx := context.Background()

	err := repo.HealthCheck(ctx)
	assert.NoError(t, err)
}

func TestShardedURLRepository_ShardCount(t *testing.T) {
	router, cleanup := setupShardedTestDB(t)
	defer cleanup()

	repo := NewShardedURLRepository(router)

	assert.Equal(t, 1, repo.ShardCount())
}

func TestShardedURLRepository_GetByID(t *testing.T) {
	router, cleanup := setupShardedTestDB(t)
	defer cleanup()

	repo := NewShardedURLRepository(router)
	ctx := context.Background()

	create := &models.URLCreate{
		ShortCode:   "shid1",
		OriginalURL: "https://example.com/byid",
	}
	created, err := repo.Create(ctx, create)
	require.NoError(t, err)

	url, err := repo.GetByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, url.ID)

	// Cleanup
	_ = repo.Delete(ctx, "shid1")
}
