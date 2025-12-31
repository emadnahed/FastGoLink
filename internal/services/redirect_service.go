// Package services contains business logic.
package services

import (
	"context"

	"github.com/gourl/gourl/internal/models"
	"github.com/gourl/gourl/internal/repository"
)

// RedirectResult represents the result of a redirect lookup.
type RedirectResult struct {
	OriginalURL string
	Permanent   bool
	CacheHit    bool
}

// RedirectService defines the interface for URL redirect operations.
type RedirectService interface {
	Redirect(ctx context.Context, shortCode string) (*RedirectResult, error)
}

// RedirectServiceImpl implements RedirectService.
type RedirectServiceImpl struct {
	repo repository.URLRepository
}

// NewRedirectService creates a new RedirectService instance.
func NewRedirectService(repo repository.URLRepository) *RedirectServiceImpl {
	return &RedirectServiceImpl{
		repo: repo,
	}
}

// Redirect looks up a URL by short code and returns the original URL for redirecting.
// It increments the click count asynchronously (failures are silently ignored to not impact redirect latency).
func (s *RedirectServiceImpl) Redirect(ctx context.Context, shortCode string) (*RedirectResult, error) {
	// Look up URL (cache-first via CachedURLRepository)
	url, err := s.repo.GetByShortCode(ctx, shortCode)
	if err != nil {
		return nil, err
	}

	// Check if URL has expired
	if url.IsExpired() {
		return nil, models.ErrURLExpired
	}

	// Increment click count (non-blocking, failures are ignored to not impact redirect latency)
	// The increment is done in-line but errors are swallowed
	_ = s.repo.IncrementClickCount(ctx, shortCode)

	return &RedirectResult{
		OriginalURL: url.OriginalURL,
		Permanent:   false, // Use 302 for temporary redirects (allows analytics updates)
		CacheHit:    false, // This would be set by the cache layer if we had access to that info
	}, nil
}
