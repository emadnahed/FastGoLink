package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/emadnahed/FastGoLink/internal/models"
)

// Note: MockURLRepository is defined in url_service_test.go

func TestRedirectService_Redirect_CacheHit(t *testing.T) {
	mockRepo := new(MockURLRepository)
	service := NewRedirectService(mockRepo)

	futureTime := time.Now().Add(24 * time.Hour)
	mockRepo.On("GetByShortCode", mock.Anything, "abc1234").Return(&models.URL{
		ID:          1,
		ShortCode:   "abc1234",
		OriginalURL: "https://example.com/path",
		CreatedAt:   time.Now(),
		ExpiresAt:   &futureTime,
		ClickCount:  10,
	}, nil)
	mockRepo.On("IncrementClickCount", mock.Anything, "abc1234").Return(nil)

	result, err := service.Redirect(context.Background(), "abc1234")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "https://example.com/path", result.OriginalURL)
	assert.False(t, result.Permanent)

	mockRepo.AssertExpectations(t)
}

func TestRedirectService_Redirect_NotFound(t *testing.T) {
	mockRepo := new(MockURLRepository)
	service := NewRedirectService(mockRepo)

	mockRepo.On("GetByShortCode", mock.Anything, "notfound").Return(nil, models.ErrURLNotFound)

	result, err := service.Redirect(context.Background(), "notfound")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, models.ErrURLNotFound)

	mockRepo.AssertExpectations(t)
}

func TestRedirectService_Redirect_Expired(t *testing.T) {
	mockRepo := new(MockURLRepository)
	service := NewRedirectService(mockRepo)

	pastTime := time.Now().Add(-24 * time.Hour)
	mockRepo.On("GetByShortCode", mock.Anything, "expired").Return(&models.URL{
		ID:          2,
		ShortCode:   "expired",
		OriginalURL: "https://example.com/expired",
		CreatedAt:   time.Now().Add(-48 * time.Hour),
		ExpiresAt:   &pastTime,
		ClickCount:  5,
	}, nil)

	result, err := service.Redirect(context.Background(), "expired")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, models.ErrURLExpired)

	// IncrementClickCount should NOT be called for expired URLs
	mockRepo.AssertNotCalled(t, "IncrementClickCount", mock.Anything, mock.Anything)
	mockRepo.AssertExpectations(t)
}

func TestRedirectService_Redirect_NoExpiry(t *testing.T) {
	mockRepo := new(MockURLRepository)
	service := NewRedirectService(mockRepo)

	mockRepo.On("GetByShortCode", mock.Anything, "noexpiry").Return(&models.URL{
		ID:          3,
		ShortCode:   "noexpiry",
		OriginalURL: "https://example.com/permanent",
		CreatedAt:   time.Now(),
		ExpiresAt:   nil,
		ClickCount:  100,
	}, nil)
	mockRepo.On("IncrementClickCount", mock.Anything, "noexpiry").Return(nil)

	result, err := service.Redirect(context.Background(), "noexpiry")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "https://example.com/permanent", result.OriginalURL)

	mockRepo.AssertExpectations(t)
}

func TestRedirectService_Redirect_IncrementFailure(t *testing.T) {
	// Click count increment failures should not fail the redirect
	mockRepo := new(MockURLRepository)
	service := NewRedirectService(mockRepo)

	mockRepo.On("GetByShortCode", mock.Anything, "abc1234").Return(&models.URL{
		ID:          1,
		ShortCode:   "abc1234",
		OriginalURL: "https://example.com/path",
		CreatedAt:   time.Now(),
		ExpiresAt:   nil,
		ClickCount:  10,
	}, nil)
	mockRepo.On("IncrementClickCount", mock.Anything, "abc1234").Return(errors.New("db error"))

	result, err := service.Redirect(context.Background(), "abc1234")

	// Redirect should still succeed even if increment fails
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "https://example.com/path", result.OriginalURL)

	mockRepo.AssertExpectations(t)
}

func TestRedirectService_Redirect_DatabaseError(t *testing.T) {
	mockRepo := new(MockURLRepository)
	service := NewRedirectService(mockRepo)

	mockRepo.On("GetByShortCode", mock.Anything, "error").Return(nil, errors.New("database connection error"))

	result, err := service.Redirect(context.Background(), "error")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "database connection error")

	mockRepo.AssertExpectations(t)
}

// mockClickRecorder implements ClickRecorder for testing.
type mockClickRecorder struct {
	recordedCodes []string
}

func (m *mockClickRecorder) RecordClick(shortCode string) {
	m.recordedCodes = append(m.recordedCodes, shortCode)
}

func TestNewRedirectServiceWithAnalytics(t *testing.T) {
	mockRepo := new(MockURLRepository)
	recorder := &mockClickRecorder{}

	service := NewRedirectServiceWithAnalytics(mockRepo, recorder)

	assert.NotNil(t, service)
	assert.NotNil(t, service.repo)
	assert.NotNil(t, service.clickRecorder)
}

func TestRedirectService_Redirect_WithClickRecorder(t *testing.T) {
	mockRepo := new(MockURLRepository)
	recorder := &mockClickRecorder{}
	service := NewRedirectServiceWithAnalytics(mockRepo, recorder)

	mockRepo.On("GetByShortCode", mock.Anything, "abc1234").Return(&models.URL{
		ID:          1,
		ShortCode:   "abc1234",
		OriginalURL: "https://example.com/path",
		CreatedAt:   time.Now(),
		ExpiresAt:   nil,
		ClickCount:  10,
	}, nil)

	result, err := service.Redirect(context.Background(), "abc1234")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "https://example.com/path", result.OriginalURL)

	// Verify click was recorded via the click recorder, not directly to repo
	assert.Contains(t, recorder.recordedCodes, "abc1234")
	mockRepo.AssertNotCalled(t, "IncrementClickCount", mock.Anything, mock.Anything)
	mockRepo.AssertExpectations(t)
}
