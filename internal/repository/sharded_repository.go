package repository

import (
	"context"
	"fmt"

	"github.com/gourl/gourl/internal/database"
	"github.com/gourl/gourl/internal/models"
)

// ShardedURLRepository implements URLRepository with database sharding.
type ShardedURLRepository struct {
	router *database.ShardRouter
}

// NewShardedURLRepository creates a new sharded URL repository.
func NewShardedURLRepository(router *database.ShardRouter) *ShardedURLRepository {
	return &ShardedURLRepository{router: router}
}

// Create stores a new URL in the appropriate shard.
func (r *ShardedURLRepository) Create(ctx context.Context, create *models.URLCreate) (*models.URL, error) {
	if err := create.Validate(); err != nil {
		return nil, err
	}

	// Route to shard based on short code
	pool := r.router.GetShard(create.ShortCode)
	repo := NewPostgresURLRepository(pool)

	return repo.Create(ctx, create)
}

// GetByShortCode retrieves a URL from the appropriate shard.
func (r *ShardedURLRepository) GetByShortCode(ctx context.Context, shortCode string) (*models.URL, error) {
	pool := r.router.GetShard(shortCode)
	repo := NewPostgresURLRepository(pool)

	return repo.GetByShortCode(ctx, shortCode)
}

// GetByID retrieves a URL by ID. Since ID-based lookups can't be sharded
// without knowing the short code, this searches all shards.
func (r *ShardedURLRepository) GetByID(ctx context.Context, id int64) (*models.URL, error) {
	shards := r.router.GetAllShards()

	for _, pool := range shards {
		repo := NewPostgresURLRepository(pool)
		url, err := repo.GetByID(ctx, id)
		if err == nil {
			return url, nil
		}
		if err != models.ErrURLNotFound {
			return nil, err
		}
	}

	return nil, models.ErrURLNotFound
}

// Delete removes a URL from the appropriate shard.
func (r *ShardedURLRepository) Delete(ctx context.Context, shortCode string) error {
	pool := r.router.GetShard(shortCode)
	repo := NewPostgresURLRepository(pool)

	return repo.Delete(ctx, shortCode)
}

// IncrementClickCount increments the click counter in the appropriate shard.
func (r *ShardedURLRepository) IncrementClickCount(ctx context.Context, shortCode string) error {
	pool := r.router.GetShard(shortCode)
	repo := NewPostgresURLRepository(pool)

	return repo.IncrementClickCount(ctx, shortCode)
}

// DeleteExpired removes expired URLs from all shards.
func (r *ShardedURLRepository) DeleteExpired(ctx context.Context) (int64, error) {
	shards := r.router.GetAllShards()
	var totalDeleted int64

	for i, pool := range shards {
		repo := NewPostgresURLRepository(pool)
		deleted, err := repo.DeleteExpired(ctx)
		if err != nil {
			return totalDeleted, fmt.Errorf("failed to delete expired from shard %d: %w", i, err)
		}
		totalDeleted += deleted
	}

	return totalDeleted, nil
}

// Exists checks if a short code exists in the appropriate shard.
func (r *ShardedURLRepository) Exists(ctx context.Context, shortCode string) (bool, error) {
	pool := r.router.GetShard(shortCode)
	repo := NewPostgresURLRepository(pool)

	return repo.Exists(ctx, shortCode)
}

// HealthCheck checks the health of all shards.
func (r *ShardedURLRepository) HealthCheck(ctx context.Context) error {
	return r.router.HealthCheck(ctx)
}

// ShardCount returns the number of shards.
func (r *ShardedURLRepository) ShardCount() int {
	return r.router.ShardCount()
}
