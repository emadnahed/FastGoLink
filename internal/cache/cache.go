// Package cache handles Redis caching operations.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/emadnahed/FastGoLink/internal/config"
)

// Common errors
var (
	ErrCacheMiss    = errors.New("cache miss")
	ErrCacheExpired = errors.New("cache entry expired")
)

// Cache defines the interface for caching operations.
type Cache interface {
	// Get retrieves a value from the cache.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores a value in the cache with a TTL.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Delete removes a value from the cache.
	Delete(ctx context.Context, key string) error

	// Exists checks if a key exists in the cache.
	Exists(ctx context.Context, key string) (bool, error)

	// Ping checks if the cache is healthy.
	Ping(ctx context.Context) error

	// Close closes the cache connection.
	Close() error
}

// RedisCache implements Cache using Redis.
type RedisCache struct {
	client *redis.Client
}

// NewRedisCache creates a new Redis cache client.
func NewRedisCache(ctx context.Context, cfg *config.RedisConfig) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Password: cfg.Password,
		DB:       cfg.DB,
		PoolSize: cfg.PoolSize,
	})

	// Verify connectivity
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &RedisCache{client: client}, nil
}

// Get retrieves a value from the cache.
func (c *RedisCache) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrCacheMiss
		}
		return nil, fmt.Errorf("cache get failed: %w", err)
	}
	return val, nil
}

// Set stores a value in the cache with a TTL.
func (c *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	err := c.client.Set(ctx, key, value, ttl).Err()
	if err != nil {
		return fmt.Errorf("cache set failed: %w", err)
	}
	return nil
}

// Delete removes a value from the cache.
func (c *RedisCache) Delete(ctx context.Context, key string) error {
	err := c.client.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("cache delete failed: %w", err)
	}
	return nil
}

// Exists checks if a key exists in the cache.
func (c *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	n, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("cache exists check failed: %w", err)
	}
	return n > 0, nil
}

// Ping checks if the cache is healthy.
func (c *RedisCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

// Close closes the cache connection.
func (c *RedisCache) Close() error {
	return c.client.Close()
}

// Client returns the underlying Redis client for advanced operations.
func (c *RedisCache) Client() *redis.Client {
	return c.client
}

// URLCacher defines the interface for URL caching operations.
// This interface enables easy mocking in tests.
type URLCacher interface {
	Get(ctx context.Context, shortCode string) (*CachedURL, error)
	Set(ctx context.Context, url *CachedURL) error
	SetWithTTL(ctx context.Context, url *CachedURL, ttl time.Duration) error
	Delete(ctx context.Context, shortCode string) error
	Exists(ctx context.Context, shortCode string) (bool, error)
	Ping(ctx context.Context) error
}

// Ensure URLCache implements URLCacher
var _ URLCacher = (*URLCache)(nil)

// URLCache provides URL-specific caching operations.
type URLCache struct {
	cache      Cache
	keyPrefix  string
	defaultTTL time.Duration
}

// NewURLCache creates a new URL-specific cache.
func NewURLCache(cache Cache, keyPrefix string, defaultTTL time.Duration) *URLCache {
	if keyPrefix == "" {
		keyPrefix = "url:"
	}
	if defaultTTL == 0 {
		defaultTTL = 24 * time.Hour
	}
	return &URLCache{
		cache:      cache,
		keyPrefix:  keyPrefix,
		defaultTTL: defaultTTL,
	}
}

// CachedURL represents a URL stored in cache.
// Contains all fields from models.URL for complete data on cache hit.
type CachedURL struct {
	ID          int64      `json:"id"`
	ShortCode   string     `json:"short_code"`
	OriginalURL string     `json:"original_url"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	ClickCount  int64      `json:"click_count"`
}

// Get retrieves a URL from cache by short code.
func (c *URLCache) Get(ctx context.Context, shortCode string) (*CachedURL, error) {
	key := c.key(shortCode)
	data, err := c.cache.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	var url CachedURL
	if err := json.Unmarshal(data, &url); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cached URL: %w", err)
	}

	// Check if URL has expired
	if url.ExpiresAt != nil && time.Now().After(*url.ExpiresAt) {
		// Delete expired entry
		_ = c.cache.Delete(ctx, key)
		return nil, ErrCacheExpired
	}

	return &url, nil
}

// Set stores a URL in cache.
func (c *URLCache) Set(ctx context.Context, url *CachedURL) error {
	return c.SetWithTTL(ctx, url, c.defaultTTL)
}

// SetWithTTL stores a URL in cache with a specific TTL.
func (c *URLCache) SetWithTTL(ctx context.Context, url *CachedURL, ttl time.Duration) error {
	key := c.key(url.ShortCode)

	data, err := json.Marshal(url)
	if err != nil {
		return fmt.Errorf("failed to marshal URL: %w", err)
	}

	// If URL has an expiry, use the minimum of TTL and time until expiry
	if url.ExpiresAt != nil {
		timeUntilExpiry := time.Until(*url.ExpiresAt)
		if timeUntilExpiry <= 0 {
			// Already expired, don't cache
			return nil
		}
		if timeUntilExpiry < ttl {
			ttl = timeUntilExpiry
		}
	}

	return c.cache.Set(ctx, key, data, ttl)
}

// Delete removes a URL from cache.
func (c *URLCache) Delete(ctx context.Context, shortCode string) error {
	return c.cache.Delete(ctx, c.key(shortCode))
}

// Exists checks if a URL exists in cache.
func (c *URLCache) Exists(ctx context.Context, shortCode string) (bool, error) {
	return c.cache.Exists(ctx, c.key(shortCode))
}

// key generates the cache key for a short code.
func (c *URLCache) key(shortCode string) string {
	return c.keyPrefix + shortCode
}

// Ping checks if the cache is healthy.
func (c *URLCache) Ping(ctx context.Context) error {
	return c.cache.Ping(ctx)
}
