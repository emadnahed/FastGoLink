package repository

import (
	"context"
	"time"

	"github.com/gourl/gourl/internal/cache"
	"github.com/gourl/gourl/internal/models"
)

// CachedURLRepository wraps a URLRepository with caching.
// It implements write-through caching with fallback to database on cache miss.
type CachedURLRepository struct {
	repo     URLRepository
	cache    *cache.URLCache
	cacheTTL time.Duration
}

// NewCachedURLRepository creates a new cached URL repository.
func NewCachedURLRepository(repo URLRepository, urlCache *cache.URLCache, cacheTTL time.Duration) *CachedURLRepository {
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

// IncrementClickCount increments the click count in the database.
// We don't cache click counts as they change frequently.
func (c *CachedURLRepository) IncrementClickCount(ctx context.Context, shortCode string) error {
	return c.repo.IncrementClickCount(ctx, shortCode)
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

// cacheURL stores a URL in the cache.
func (c *CachedURLRepository) cacheURL(ctx context.Context, url *models.URL) error {
	cached := &cache.CachedURL{
		ShortCode:   url.ShortCode,
		OriginalURL: url.OriginalURL,
		ExpiresAt:   url.ExpiresAt,
	}
	return c.cache.SetWithTTL(ctx, cached, c.cacheTTL)
}

// cachedToURL converts a CachedURL to a URL model.
// Note: Some fields like ID, CreatedAt, ClickCount are not cached.
func (c *CachedURLRepository) cachedToURL(cached *cache.CachedURL) *models.URL {
	return &models.URL{
		ShortCode:   cached.ShortCode,
		OriginalURL: cached.OriginalURL,
		ExpiresAt:   cached.ExpiresAt,
	}
}
