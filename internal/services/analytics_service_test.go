package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gourl/gourl/internal/models"
)

// mockPendingStatsProvider implements PendingStatsProvider for testing.
type mockPendingStatsProvider struct {
	stats map[string]int64
}

func (m *mockPendingStatsProvider) GetPendingStats() map[string]int64 {
	return m.stats
}

func TestNewAnalyticsService(t *testing.T) {
	repo := &MockURLRepository{}
	svc := NewAnalyticsService(repo)

	require.NotNil(t, svc)
	assert.NotNil(t, svc.repo)
	assert.Nil(t, svc.pendingProvider)
}

func TestNewAnalyticsServiceWithPendingStats(t *testing.T) {
	repo := &MockURLRepository{}
	provider := &mockPendingStatsProvider{}
	svc := NewAnalyticsServiceWithPendingStats(repo, provider)

	require.NotNil(t, svc)
	assert.NotNil(t, svc.repo)
	assert.NotNil(t, svc.pendingProvider)
}

func TestAnalyticsServiceImpl_GetURLStats(t *testing.T) {
	t.Run("returns stats for existing URL", func(t *testing.T) {
		repo := &MockURLRepository{}
		svc := NewAnalyticsService(repo)

		url := &models.URL{
			ID:          1,
			ShortCode:   "abc123",
			OriginalURL: "https://example.com",
			ClickCount:  42,
			CreatedAt:   time.Now(),
		}
		repo.On("GetByShortCode", mock.Anything, "abc123").Return(url, nil)

		stats, err := svc.GetURLStats(context.Background(), "abc123")

		require.NoError(t, err)
		assert.Equal(t, "abc123", stats.ShortCode)
		assert.Equal(t, int64(42), stats.ClickCount)
		assert.Equal(t, int64(0), stats.PendingCount)
		repo.AssertExpectations(t)
	})

	t.Run("returns error for non-existent URL", func(t *testing.T) {
		repo := &MockURLRepository{}
		svc := NewAnalyticsService(repo)

		repo.On("GetByShortCode", mock.Anything, "nonexistent").Return(nil, errors.New("not found"))

		stats, err := svc.GetURLStats(context.Background(), "nonexistent")

		require.Error(t, err)
		assert.Nil(t, stats)
		repo.AssertExpectations(t)
	})

	t.Run("includes pending clicks when provider available", func(t *testing.T) {
		repo := &MockURLRepository{}
		provider := &mockPendingStatsProvider{
			stats: map[string]int64{
				"abc123": 5,
				"other":  10,
			},
		}
		svc := NewAnalyticsServiceWithPendingStats(repo, provider)

		url := &models.URL{
			ID:          1,
			ShortCode:   "abc123",
			OriginalURL: "https://example.com",
			ClickCount:  42,
			CreatedAt:   time.Now(),
		}
		repo.On("GetByShortCode", mock.Anything, "abc123").Return(url, nil)

		stats, err := svc.GetURLStats(context.Background(), "abc123")

		require.NoError(t, err)
		assert.Equal(t, "abc123", stats.ShortCode)
		assert.Equal(t, int64(42), stats.ClickCount)
		assert.Equal(t, int64(5), stats.PendingCount)
		repo.AssertExpectations(t)
	})

	t.Run("returns zero pending count when not in pending stats", func(t *testing.T) {
		repo := &MockURLRepository{}
		provider := &mockPendingStatsProvider{
			stats: map[string]int64{
				"other": 10,
			},
		}
		svc := NewAnalyticsServiceWithPendingStats(repo, provider)

		url := &models.URL{
			ID:          1,
			ShortCode:   "abc123",
			OriginalURL: "https://example.com",
			ClickCount:  42,
			CreatedAt:   time.Now(),
		}
		repo.On("GetByShortCode", mock.Anything, "abc123").Return(url, nil)

		stats, err := svc.GetURLStats(context.Background(), "abc123")

		require.NoError(t, err)
		assert.Equal(t, int64(0), stats.PendingCount)
		repo.AssertExpectations(t)
	})
}
