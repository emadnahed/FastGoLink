package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/gourl/gourl/internal/idgen"
	"github.com/gourl/gourl/internal/models"
	"github.com/gourl/gourl/internal/services"
)

// MockURLService is a mock implementation of services.URLService.
type MockURLService struct {
	mock.Mock
}

func (m *MockURLService) Create(ctx context.Context, req services.CreateURLRequest) (*services.CreateURLResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*services.CreateURLResponse), args.Error(1)
}

func (m *MockURLService) Get(ctx context.Context, shortCode string) (*models.URL, error) {
	args := m.Called(ctx, shortCode)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.URL), args.Error(1)
}

func (m *MockURLService) Delete(ctx context.Context, shortCode string) error {
	args := m.Called(ctx, shortCode)
	return args.Error(0)
}

func TestURLHandler_Shorten(t *testing.T) {
	now := time.Now()
	futureTime := now.Add(24 * time.Hour)

	tests := []struct {
		name           string
		method         string
		body           interface{}
		setupMock      func(*MockURLService)
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:   "POST with valid URL returns 201 and short URL",
			method: http.MethodPost,
			body: ShortenRequest{
				URL: "https://example.com/very/long/path",
			},
			setupMock: func(svc *MockURLService) {
				svc.On("Create", mock.Anything, mock.MatchedBy(func(req services.CreateURLRequest) bool {
					return req.OriginalURL == "https://example.com/very/long/path" && req.ExpiresIn == nil
				})).Return(&services.CreateURLResponse{
					ShortURL:    "http://localhost:8080/abc1234",
					ShortCode:   "abc1234",
					OriginalURL: "https://example.com/very/long/path",
					CreatedAt:   now,
					ExpiresAt:   nil,
				}, nil)
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp ShortenResponse
				err := json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Equal(t, "http://localhost:8080/abc1234", resp.ShortURL)
				assert.Equal(t, "abc1234", resp.ShortCode)
				assert.Equal(t, "https://example.com/very/long/path", resp.OriginalURL)
				assert.NotEmpty(t, resp.CreatedAt)
				assert.Nil(t, resp.ExpiresAt)
			},
		},
		{
			name:   "POST with expires_in creates expiring URL",
			method: http.MethodPost,
			body: ShortenRequest{
				URL:       "https://example.com/path",
				ExpiresIn: "24h",
			},
			setupMock: func(svc *MockURLService) {
				svc.On("Create", mock.Anything, mock.MatchedBy(func(req services.CreateURLRequest) bool {
					return req.OriginalURL == "https://example.com/path" &&
						req.ExpiresIn != nil &&
						*req.ExpiresIn == 24*time.Hour
				})).Return(&services.CreateURLResponse{
					ShortURL:    "http://localhost:8080/xyz9876",
					ShortCode:   "xyz9876",
					OriginalURL: "https://example.com/path",
					CreatedAt:   now,
					ExpiresAt:   &futureTime,
				}, nil)
			},
			expectedStatus: http.StatusCreated,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp ShortenResponse
				err := json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Equal(t, "xyz9876", resp.ShortCode)
				assert.NotNil(t, resp.ExpiresAt)
			},
		},
		{
			name:           "POST with empty body returns 400",
			method:         http.MethodPost,
			body:           nil,
			setupMock:      func(svc *MockURLService) {},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp ErrorResponse
				err := json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.NotEmpty(t, resp.Error)
			},
		},
		{
			name:           "POST with invalid JSON returns 400",
			method:         http.MethodPost,
			body:           "not valid json{",
			setupMock:      func(svc *MockURLService) {},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp ErrorResponse
				err := json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Contains(t, resp.Error, "invalid")
			},
		},
		{
			name:   "POST with empty URL returns 400",
			method: http.MethodPost,
			body: ShortenRequest{
				URL: "",
			},
			setupMock: func(svc *MockURLService) {
				svc.On("Create", mock.Anything, mock.Anything).Return(nil, models.ErrEmptyURL)
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp ErrorResponse
				err := json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Equal(t, "EMPTY_URL", resp.Code)
			},
		},
		{
			name:   "POST with invalid URL returns 400",
			method: http.MethodPost,
			body: ShortenRequest{
				URL: "not-a-valid-url",
			},
			setupMock: func(svc *MockURLService) {
				svc.On("Create", mock.Anything, mock.Anything).Return(nil, models.ErrInvalidURL)
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp ErrorResponse
				err := json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Equal(t, "INVALID_URL", resp.Code)
			},
		},
		{
			name:   "POST with invalid expires_in returns 400",
			method: http.MethodPost,
			body: ShortenRequest{
				URL:       "https://example.com/path",
				ExpiresIn: "not-a-duration",
			},
			setupMock:      func(svc *MockURLService) {},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp ErrorResponse
				err := json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Equal(t, "INVALID_EXPIRES_IN", resp.Code)
			},
		},
		{
			name:   "service error returns 500",
			method: http.MethodPost,
			body: ShortenRequest{
				URL: "https://example.com/path",
			},
			setupMock: func(svc *MockURLService) {
				svc.On("Create", mock.Anything, mock.Anything).Return(nil, errors.New("database error"))
			},
			expectedStatus: http.StatusInternalServerError,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp ErrorResponse
				err := json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Equal(t, "INTERNAL_ERROR", resp.Code)
			},
		},
		{
			name:   "max retries exceeded returns 503",
			method: http.MethodPost,
			body: ShortenRequest{
				URL: "https://example.com/path",
			},
			setupMock: func(svc *MockURLService) {
				svc.On("Create", mock.Anything, mock.Anything).Return(nil, idgen.ErrMaxRetriesExceeded)
			},
			expectedStatus: http.StatusServiceUnavailable,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp ErrorResponse
				err := json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Equal(t, "RETRY_EXCEEDED", resp.Code)
			},
		},
		{
			name:   "dangerous URL returns 400",
			method: http.MethodPost,
			body: ShortenRequest{
				URL: "javascript:alert(1)",
			},
			setupMock: func(svc *MockURLService) {
				svc.On("Create", mock.Anything, mock.Anything).Return(nil, services.ErrDangerousURL)
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp ErrorResponse
				err := json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Equal(t, "DANGEROUS_URL", resp.Code)
			},
		},
		{
			name:   "private IP returns 400",
			method: http.MethodPost,
			body: ShortenRequest{
				URL: "http://192.168.1.1/admin",
			},
			setupMock: func(svc *MockURLService) {
				svc.On("Create", mock.Anything, mock.Anything).Return(nil, services.ErrPrivateIPURL)
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp ErrorResponse
				err := json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Equal(t, "PRIVATE_IP_BLOCKED", resp.Code)
			},
		},
		{
			name:   "blocked host returns 400",
			method: http.MethodPost,
			body: ShortenRequest{
				URL: "https://blocked.com/path",
			},
			setupMock: func(svc *MockURLService) {
				svc.On("Create", mock.Anything, mock.Anything).Return(nil, services.ErrBlockedHostURL)
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp ErrorResponse
				err := json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Equal(t, "BLOCKED_HOST", resp.Code)
			},
		},
		{
			name:   "URL too long returns 400",
			method: http.MethodPost,
			body: ShortenRequest{
				URL: "https://example.com/very-long-path",
			},
			setupMock: func(svc *MockURLService) {
				svc.On("Create", mock.Anything, mock.Anything).Return(nil, services.ErrURLTooLong)
			},
			expectedStatus: http.StatusBadRequest,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp ErrorResponse
				err := json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Equal(t, "URL_TOO_LONG", resp.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSvc := new(MockURLService)
			tt.setupMock(mockSvc)

			handler := NewURLHandler(mockSvc)

			var body []byte
			var err error
			if tt.body != nil {
				switch v := tt.body.(type) {
				case string:
					body = []byte(v)
				default:
					body, err = json.Marshal(tt.body)
					require.NoError(t, err)
				}
			}

			req := httptest.NewRequest(tt.method, "/api/v1/shorten", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()

			handler.Shorten(rec, req)

			assert.Equal(t, tt.expectedStatus, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			tt.checkResponse(t, rec)

			mockSvc.AssertExpectations(t)
		})
	}
}

func TestURLHandler_GetURL(t *testing.T) {
	now := time.Now()
	futureTime := now.Add(24 * time.Hour)

	tests := []struct {
		name           string
		shortCode      string
		setupMock      func(*MockURLService)
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:      "GET existing code returns 200 with URL info",
			shortCode: "abc1234",
			setupMock: func(svc *MockURLService) {
				svc.On("Get", mock.Anything, "abc1234").Return(&models.URL{
					ID:          1,
					ShortCode:   "abc1234",
					OriginalURL: "https://example.com/path",
					CreatedAt:   now,
					ExpiresAt:   &futureTime,
					ClickCount:  42,
				}, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp URLInfoResponse
				err := json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Equal(t, "abc1234", resp.ShortCode)
				assert.Equal(t, "https://example.com/path", resp.OriginalURL)
				assert.Equal(t, int64(42), resp.ClickCount)
				assert.NotNil(t, resp.ExpiresAt)
			},
		},
		{
			name:      "GET non-existent code returns 404",
			shortCode: "notfound",
			setupMock: func(svc *MockURLService) {
				svc.On("Get", mock.Anything, "notfound").Return(nil, models.ErrURLNotFound)
			},
			expectedStatus: http.StatusNotFound,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp ErrorResponse
				err := json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Equal(t, "NOT_FOUND", resp.Code)
			},
		},
		{
			name:      "GET expired code returns 410 Gone",
			shortCode: "expired",
			setupMock: func(svc *MockURLService) {
				svc.On("Get", mock.Anything, "expired").Return(nil, models.ErrURLExpired)
			},
			expectedStatus: http.StatusGone,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp ErrorResponse
				err := json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Equal(t, "EXPIRED", resp.Code)
			},
		},
		{
			name:      "service error returns 500",
			shortCode: "error",
			setupMock: func(svc *MockURLService) {
				svc.On("Get", mock.Anything, "error").Return(nil, errors.New("database error"))
			},
			expectedStatus: http.StatusInternalServerError,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp ErrorResponse
				err := json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Equal(t, "INTERNAL_ERROR", resp.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSvc := new(MockURLService)
			tt.setupMock(mockSvc)

			handler := NewURLHandler(mockSvc)

			req := httptest.NewRequest(http.MethodGet, "/api/v1/urls/"+tt.shortCode, nil)
			rec := httptest.NewRecorder()

			handler.GetURL(rec, req, tt.shortCode)

			assert.Equal(t, tt.expectedStatus, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			tt.checkResponse(t, rec)

			mockSvc.AssertExpectations(t)
		})
	}
}

func TestURLHandler_DeleteURL(t *testing.T) {
	tests := []struct {
		name           string
		shortCode      string
		setupMock      func(*MockURLService)
		expectedStatus int
		checkResponse  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:      "DELETE existing code returns 204 No Content",
			shortCode: "abc1234",
			setupMock: func(svc *MockURLService) {
				svc.On("Delete", mock.Anything, "abc1234").Return(nil)
			},
			expectedStatus: http.StatusNoContent,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				assert.Empty(t, rec.Body.Bytes())
			},
		},
		{
			name:      "DELETE non-existent code returns 404",
			shortCode: "notfound",
			setupMock: func(svc *MockURLService) {
				svc.On("Delete", mock.Anything, "notfound").Return(models.ErrURLNotFound)
			},
			expectedStatus: http.StatusNotFound,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp ErrorResponse
				err := json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Equal(t, "NOT_FOUND", resp.Code)
			},
		},
		{
			name:      "service error returns 500",
			shortCode: "error",
			setupMock: func(svc *MockURLService) {
				svc.On("Delete", mock.Anything, "error").Return(errors.New("database error"))
			},
			expectedStatus: http.StatusInternalServerError,
			checkResponse: func(t *testing.T, rec *httptest.ResponseRecorder) {
				var resp ErrorResponse
				err := json.Unmarshal(rec.Body.Bytes(), &resp)
				require.NoError(t, err)
				assert.Equal(t, "INTERNAL_ERROR", resp.Code)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockSvc := new(MockURLService)
			tt.setupMock(mockSvc)

			handler := NewURLHandler(mockSvc)

			req := httptest.NewRequest(http.MethodDelete, "/api/v1/urls/"+tt.shortCode, nil)
			rec := httptest.NewRecorder()

			handler.DeleteURL(rec, req, tt.shortCode)

			assert.Equal(t, tt.expectedStatus, rec.Code)
			tt.checkResponse(t, rec)

			mockSvc.AssertExpectations(t)
		})
	}
}
