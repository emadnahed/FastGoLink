package repository

import (
	"context"
	"time"

	"github.com/emadnahed/FastGoLink/internal/cache"
	"github.com/emadnahed/FastGoLink/internal/models"
)

// CachedURLRepository wraps a URLRepository with caching.
// It implements write-through caching with fallback to database on cache miss.
type CachedURLRepository struct {
	repo     URLRepository
	cache    cache.URLCacher
	cacheTTL time.Duration
}

// NewCachedURLRepository creates a new cached URL repository.
func NewCachedURLRepository(repo URLRepository, urlCache cache.URLCacher, cacheTTL time.Duration) *CachedURLRepository {
	if cacheTTL == 0 {
		cacheTTL = 24 * time.Hour
	}
	return &CachedURLRepository{
		repo:     repo,
		cache:    urlCache,
		cacheTTL: cacheTTL,
	}
}

// Create stores a new URL in both database and cache (write-through).
func (c *CachedURLRepository) Create(ctx context.Context, create *models.URLCreate) (*models.URL, error) {
	// First create in database
	url, err := c.repo.Create(ctx, create)
	if err != nil {
		return nil, err
	}

	// Then cache it (ignore cache errors - they're not critical)
	_ = c.cacheURL(ctx, url)

	return url, nil
}

// GetByShortCode retrieves a URL, checking cache first then falling back to database.
func (c *CachedURLRepository) GetByShortCode(ctx context.Context, shortCode string) (*models.URL, error) {
	// Try cache first
	cached, err := c.cache.Get(ctx, shortCode)
	if err == nil {
		return c.cachedToURL(cached), nil
	}

	// Cache miss or error - fallback to database
	url, err := c.repo.GetByShortCode(ctx, shortCode)
	if err != nil {
		return nil, err
	}

	// Cache the result for next time
	_ = c.cacheURL(ctx, url)

	return url, nil
}

// GetByID retrieves a URL by ID from database (not cached by ID).
func (c *CachedURLRepository) GetByID(ctx context.Context, id int64) (*models.URL, error) {
	return c.repo.GetByID(ctx, id)
}

// Delete removes a URL from both cache and database.
func (c *CachedURLRepository) Delete(ctx context.Context, shortCode string) error {
	// Delete from cache first
	_ = c.cache.Delete(ctx, shortCode)

	// Then delete from database
	return c.repo.Delete(ctx, shortCode)
}

// IncrementClickCount increments the click count in the database
// and invalidates the cache to avoid serving stale data.
func (c *CachedURLRepository) IncrementClickCount(ctx context.Context, shortCode string) error {
	if err := c.repo.IncrementClickCount(ctx, shortCode); err != nil {
		return err
	}
	// Invalidate cache to avoid serving stale click counts
	_ = c.cache.Delete(ctx, shortCode)
	return nil
}

// BatchIncrementClickCounts increments click counts for multiple URLs
// and invalidates their cache entries.
func (c *CachedURLRepository) BatchIncrementClickCounts(ctx context.Context, counts map[string]int64) error {
	if err := c.repo.BatchIncrementClickCounts(ctx, counts); err != nil {
		return err
	}
	// Invalidate cache entries for all updated URLs
	for shortCode := range counts {
		_ = c.cache.Delete(ctx, shortCode)
	}
	return nil
}

// DeleteExpired removes expired URLs from database and doesn't touch cache
// (cache entries have their own TTL).
func (c *CachedURLRepository) DeleteExpired(ctx context.Context) (int64, error) {
	return c.repo.DeleteExpired(ctx)
}

// Exists checks if a URL exists, checking cache first.
func (c *CachedURLRepository) Exists(ctx context.Context, shortCode string) (bool, error) {
	// Try cache first
	exists, err := c.cache.Exists(ctx, shortCode)
	if err == nil && exists {
		return true, nil
	}

	// Fallback to database
	return c.repo.Exists(ctx, shortCode)
}

// HealthCheck checks both cache and database health.
func (c *CachedURLRepository) HealthCheck(ctx context.Context) error {
	// Check cache health
	if err := c.cache.Ping(ctx); err != nil {
		return err
	}

	// Check database health
	return c.repo.HealthCheck(ctx)
}

// cacheURL stores a URL in the cache with all fields.
func (c *CachedURLRepository) cacheURL(ctx context.Context, url *models.URL) error {
	cached := &cache.CachedURL{
		ID:          url.ID,
		ShortCode:   url.ShortCode,
		OriginalURL: url.OriginalURL,
		CreatedAt:   url.CreatedAt,
		ExpiresAt:   url.ExpiresAt,
		ClickCount:  url.ClickCount,
	}
	return c.cache.SetWithTTL(ctx, cached, c.cacheTTL)
}

// cachedToURL converts a CachedURL to a URL model.
// All fields are now fully populated from the cache.
func (c *CachedURLRepository) cachedToURL(cached *cache.CachedURL) *models.URL {
	return &models.URL{
		ID:          cached.ID,
		ShortCode:   cached.ShortCode,
		OriginalURL: cached.OriginalURL,
		CreatedAt:   cached.CreatedAt,
		ExpiresAt:   cached.ExpiresAt,
		ClickCount:  cached.ClickCount,
	}
}
