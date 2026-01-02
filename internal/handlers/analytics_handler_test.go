package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/emadnahed/FastGoLink/internal/services"
)

// mockAnalyticsService implements services.AnalyticsService for testing.
type mockAnalyticsService struct {
	stats *services.URLStats
	err   error
}

func (m *mockAnalyticsService) GetURLStats(ctx context.Context, shortCode string) (*services.URLStats, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.stats, nil
}

func TestNewAnalyticsHandler(t *testing.T) {
	svc := &mockAnalyticsService{}
	handler := NewAnalyticsHandler(svc)

	require.NotNil(t, handler)
	assert.NotNil(t, handler.service)
}

func TestAnalyticsHandler_GetStats(t *testing.T) {
	t.Run("returns stats for valid short code", func(t *testing.T) {
		svc := &mockAnalyticsService{
			stats: &services.URLStats{
				ShortCode:    "abc123",
				ClickCount:   42,
				PendingCount: 5,
			},
		}
		handler := NewAnalyticsHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics/abc123", nil)
		rec := httptest.NewRecorder()

		handler.GetStats(rec, req, "abc123")

		assert.Equal(t, http.StatusOK, rec.Code)

		var stats services.URLStats
		err := json.NewDecoder(rec.Body).Decode(&stats)
		require.NoError(t, err)
		assert.Equal(t, "abc123", stats.ShortCode)
		assert.Equal(t, int64(42), stats.ClickCount)
		assert.Equal(t, int64(5), stats.PendingCount)
	})

	t.Run("returns 400 for empty short code", func(t *testing.T) {
		svc := &mockAnalyticsService{}
		handler := NewAnalyticsHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics/", nil)
		rec := httptest.NewRecorder()

		handler.GetStats(rec, req, "")

		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var errResp ErrorResponse
		err := json.NewDecoder(rec.Body).Decode(&errResp)
		require.NoError(t, err)
		assert.Equal(t, "INVALID_SHORT_CODE", errResp.Code)
	})

	t.Run("returns 404 for non-existent URL", func(t *testing.T) {
		svc := &mockAnalyticsService{
			err: errors.New("not found"),
		}
		handler := NewAnalyticsHandler(svc)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/analytics/nonexistent", nil)
		rec := httptest.NewRecorder()

		handler.GetStats(rec, req, "nonexistent")

		assert.Equal(t, http.StatusNotFound, rec.Code)

		var errResp ErrorResponse
		err := json.NewDecoder(rec.Body).Decode(&errResp)
		require.NoError(t, err)
		assert.Equal(t, "NOT_FOUND", errResp.Code)
	})
}
