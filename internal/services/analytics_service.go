package services

import (
	"context"

	"github.com/gourl/gourl/internal/repository"
)

// URLStats represents click statistics for a URL.
type URLStats struct {
	ShortCode    string `json:"short_code"`
	ClickCount   int64  `json:"click_count"`
	PendingCount int64  `json:"pending_count,omitempty"`
}

// PendingStatsProvider provides access to pending (unflushed) click counts.
type PendingStatsProvider interface {
	GetPendingStats() map[string]int64
}

// AnalyticsService defines the interface for analytics operations.
type AnalyticsService interface {
	GetURLStats(ctx context.Context, shortCode string) (*URLStats, error)
}

// AnalyticsServiceImpl implements AnalyticsService.
type AnalyticsServiceImpl struct {
	repo            repository.URLRepository
	pendingProvider PendingStatsProvider
}

// NewAnalyticsService creates a new AnalyticsService.
func NewAnalyticsService(repo repository.URLRepository) *AnalyticsServiceImpl {
	return &AnalyticsServiceImpl{
		repo: repo,
	}
}

// NewAnalyticsServiceWithPendingStats creates an AnalyticsService with pending stats support.
func NewAnalyticsServiceWithPendingStats(repo repository.URLRepository, provider PendingStatsProvider) *AnalyticsServiceImpl {
	return &AnalyticsServiceImpl{
		repo:            repo,
		pendingProvider: provider,
	}
}

// GetURLStats retrieves click statistics for a URL.
func (s *AnalyticsServiceImpl) GetURLStats(ctx context.Context, shortCode string) (*URLStats, error) {
	url, err := s.repo.GetByShortCode(ctx, shortCode)
	if err != nil {
		return nil, err
	}

	stats := &URLStats{
		ShortCode:  url.ShortCode,
		ClickCount: url.ClickCount,
	}

	// Add pending (unflushed) clicks if available
	if s.pendingProvider != nil {
		pending := s.pendingProvider.GetPendingStats()
		if count, ok := pending[shortCode]; ok {
			stats.PendingCount = count
		}
	}

	return stats, nil
}
