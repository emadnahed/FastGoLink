package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/emadnahed/FastGoLink/internal/models"
	"github.com/emadnahed/FastGoLink/internal/services"
)

// MockRedirectService is a mock implementation of services.RedirectService.
type MockRedirectService struct {
	mock.Mock
}

func (m *MockRedirectService) Redirect(ctx context.Context, shortCode string) (*services.RedirectResult, error) {
	args := m.Called(ctx, shortCode)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*services.RedirectResult), args.Error(1)
}

func TestRedirectHandler_Redirect(t *testing.T) {
	tests := []struct {
		name             string
		shortCode        string
		setupMock        func(*MockRedirectService)
		expectedStatus   int
		expectedLocation string
		checkResponse    func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:      "valid code redirects with 302",
			shortCode: "abc1234",
			setupMock: func(svc *MockRedirectService) {
				svc.On("Redirect", mock.Anything, "abc1234").Return(&services.RedirectResult{
					OriginalURL: "https://example.com/very/long/path",
					Permanent:   false,
				}, nil)
			},
			expectedStatus:   http.StatusFound,
			expectedLocation: "https://example.com/very/long/path",
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Equal(t, "https://example.com/very/long/path", rec.Header().Get("Location"))
			},
		},
		{
			name:      "permanent redirect uses 301",
			shortCode: "perm123",
			setupMock: func(svc *MockRedirectService) {
				svc.On("Redirect", mock.Anything, "perm123").Return(&services.RedirectResult{
					OriginalURL: "https://example.com/permanent",
					Permanent:   true,
				}, nil)
			},
			expectedStatus:   http.StatusMovedPermanently,
			expectedLocation: "https://example.com/permanent",
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Equal(t, "https://example.com/permanent", rec.Header().Get("Location"))
			},
		},
		{
			name:      "non-existent code returns 404",
			shortCode: "notfound",
			setupMock: func(svc *MockRedirectService) {
				svc.On("Redirect", mock.Anything, "notfound").Return(nil, models.ErrURLNotFound)
			},
			expectedStatus:   http.StatusNotFound,
			expectedLocation: "",
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Empty(t, rec.Header().Get("Location"))
			},
		},
		{
			name:      "expired code returns 410 Gone",
			shortCode: "expired",
			setupMock: func(svc *MockRedirectService) {
				svc.On("Redirect", mock.Anything, "expired").Return(nil, models.ErrURLExpired)
			},
			expectedStatus:   http.StatusGone,
			expectedLocation: "",
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Empty(t, rec.Header().Get("Location"))
			},
		},
		{
			name:      "service error returns 500",
			shortCode: "error",
			setupMock: func(svc *MockRedirectService) {
				svc.On("Redirect", mock.Anything, "error").Return(nil, errors.New("database error"))
			},
			expectedStatus:   http.StatusInternalServerError,
			expectedLocation: "",
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Empty(t, rec.Header().Get("Location"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSvc := new(MockRedirectService)
			tt.setupMock(mockSvc)

			handler := NewRedirectHandler(mockSvc)

			req := httptest.NewRequest(http.MethodGet, "/"+tt.shortCode, nil)
			rec := httptest.NewRecorder()

			handler.Redirect(rec, req, tt.shortCode)

			assert.Equal(t, tt.expectedStatus, rec.Code)
			if tt.expectedLocation != "" {
				assert.Equal(t, tt.expectedLocation, rec.Header().Get("Location"))
			}
			tt.checkResponse(t, rec)

			mockSvc.AssertExpectations(t)
		})
	}
}

func TestRedirectHandler_LatencyTracking(t *testing.T) {
	mockSvc := new(MockRedirectService)
	mockSvc.On("Redirect", mock.Anything, "fast123").Return(&services.RedirectResult{
		OriginalURL: "https://example.com/fast",
		Permanent:   false,
		CacheHit:    true,
	}, nil)

	handler := NewRedirectHandler(mockSvc)

	req := httptest.NewRequest(http.MethodGet, "/fast123", nil)
	rec := httptest.NewRecorder()

	start := time.Now()
	handler.Redirect(rec, req, "fast123")
	elapsed := time.Since(start)

	assert.Equal(t, http.StatusFound, rec.Code)
	// Handler overhead should be minimal (under 1ms typically, using 10ms as safe bound)
	assert.Less(t, elapsed, 10*time.Millisecond, "redirect handler should have minimal overhead")

	mockSvc.AssertExpectations(t)
}
