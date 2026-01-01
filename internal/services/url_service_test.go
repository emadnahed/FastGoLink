package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/emadnahed/FastGoLink/internal/idgen"
	"github.com/emadnahed/FastGoLink/internal/models"
	"github.com/emadnahed/FastGoLink/internal/security"
)

// MockURLRepository is a mock implementation of repository.URLRepository.
type MockURLRepository struct {
	mock.Mock
}

func (m *MockURLRepository) Create(ctx context.Context, url *models.URLCreate) (*models.URL, error) {
	args := m.Called(ctx, url)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.URL), args.Error(1)
}

func (m *MockURLRepository) GetByShortCode(ctx context.Context, shortCode string) (*models.URL, error) {
	args := m.Called(ctx, shortCode)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.URL), args.Error(1)
}

func (m *MockURLRepository) GetByID(ctx context.Context, id int64) (*models.URL, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*models.URL), args.Error(1)
}

func (m *MockURLRepository) Delete(ctx context.Context, shortCode string) error {
	args := m.Called(ctx, shortCode)
	return args.Error(0)
}

func (m *MockURLRepository) IncrementClickCount(ctx context.Context, shortCode string) error {
	args := m.Called(ctx, shortCode)
	return args.Error(0)
}

func (m *MockURLRepository) BatchIncrementClickCounts(ctx context.Context, counts map[string]int64) error {
	args := m.Called(ctx, counts)
	return args.Error(0)
}

func (m *MockURLRepository) DeleteExpired(ctx context.Context) (int64, error) {
	args := m.Called(ctx)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockURLRepository) Exists(ctx context.Context, shortCode string) (bool, error) {
	args := m.Called(ctx, shortCode)
	return args.Bool(0), args.Error(1)
}

func (m *MockURLRepository) HealthCheck(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

// MockGenerator is a mock implementation of idgen.Generator.
type MockGenerator struct {
	mock.Mock
}

func (m *MockGenerator) Generate() (string, error) {
	args := m.Called()
	return args.String(0), args.Error(1)
}

func TestURLService_Create(t *testing.T) {
	ctx := context.Background()
	baseURL := "http://localhost:8080"

	tests := []struct {
		name          string
		request       CreateURLRequest
		setupMocks    func(*MockURLRepository, *MockGenerator)
		expectedCode  string
		expectedURL   string
		expectedError error
	}{
		{
			name: "valid URL creates short URL",
			request: CreateURLRequest{
				OriginalURL: "https://example.com/very/long/path",
			},
			setupMocks: func(repo *MockURLRepository, gen *MockGenerator) {
				gen.On("Generate").Return("abc1234", nil)
				repo.On("Create", ctx, mock.MatchedBy(func(u *models.URLCreate) bool {
					return u.OriginalURL == "https://example.com/very/long/path" &&
						u.ShortCode == "abc1234" &&
						u.ExpiresAt == nil
				})).Return(&models.URL{
					ID:          1,
					ShortCode:   "abc1234",
					OriginalURL: "https://example.com/very/long/path",
					CreatedAt:   time.Now(),
					ExpiresAt:   nil,
					ClickCount:  0,
				}, nil)
			},
			expectedCode: "abc1234",
			expectedURL:  "http://localhost:8080/abc1234",
		},
		{
			name: "URL with expiry sets expires_at",
			request: CreateURLRequest{
				OriginalURL: "https://example.com/path",
				ExpiresIn:   durationPtr(24 * time.Hour),
			},
			setupMocks: func(repo *MockURLRepository, gen *MockGenerator) {
				gen.On("Generate").Return("xyz9876", nil)
				repo.On("Create", ctx, mock.MatchedBy(func(u *models.URLCreate) bool {
					return u.OriginalURL == "https://example.com/path" &&
						u.ShortCode == "xyz9876" &&
						u.ExpiresAt != nil
				})).Return(&models.URL{
					ID:          2,
					ShortCode:   "xyz9876",
					OriginalURL: "https://example.com/path",
					CreatedAt:   time.Now(),
					ExpiresAt:   timePtr(time.Now().Add(24 * time.Hour)),
					ClickCount:  0,
				}, nil)
			},
			expectedCode: "xyz9876",
			expectedURL:  "http://localhost:8080/xyz9876",
		},
		{
			name: "empty URL returns error",
			request: CreateURLRequest{
				OriginalURL: "",
			},
			setupMocks:    func(repo *MockURLRepository, gen *MockGenerator) {},
			expectedError: models.ErrEmptyURL,
		},
		{
			name: "invalid URL format returns error",
			request: CreateURLRequest{
				OriginalURL: "not-a-valid-url",
			},
			setupMocks:    func(repo *MockURLRepository, gen *MockGenerator) {},
			expectedError: models.ErrInvalidURL,
		},
		{
			name: "URL without scheme returns error",
			request: CreateURLRequest{
				OriginalURL: "example.com/path",
			},
			setupMocks:    func(repo *MockURLRepository, gen *MockGenerator) {},
			expectedError: models.ErrInvalidURL,
		},
		{
			name: "generator failure returns error",
			request: CreateURLRequest{
				OriginalURL: "https://example.com/path",
			},
			setupMocks: func(repo *MockURLRepository, gen *MockGenerator) {
				gen.On("Generate").Return("", errors.New("generator error"))
			},
			expectedError: errors.New("generator error"),
		},
		{
			name: "repository failure returns error",
			request: CreateURLRequest{
				OriginalURL: "https://example.com/path",
			},
			setupMocks: func(repo *MockURLRepository, gen *MockGenerator) {
				gen.On("Generate").Return("abc1234", nil)
				repo.On("Create", ctx, mock.Anything).Return(nil, errors.New("database error"))
			},
			expectedError: errors.New("database error"),
		},
		{
			name: "max retries exceeded returns error",
			request: CreateURLRequest{
				OriginalURL: "https://example.com/path",
			},
			setupMocks: func(repo *MockURLRepository, gen *MockGenerator) {
				gen.On("Generate").Return("", idgen.ErrMaxRetriesExceeded)
			},
			expectedError: idgen.ErrMaxRetriesExceeded,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockURLRepository)
			mockGen := new(MockGenerator)

			tt.setupMocks(mockRepo, mockGen)

			svc := NewURLService(mockRepo, mockGen, baseURL)
			resp, err := svc.Create(ctx, tt.request)

			if tt.expectedError != nil {
				require.Error(t, err)
				if errors.Is(tt.expectedError, models.ErrEmptyURL) ||
					errors.Is(tt.expectedError, models.ErrInvalidURL) ||
					errors.Is(tt.expectedError, idgen.ErrMaxRetriesExceeded) {
					assert.ErrorIs(t, err, tt.expectedError)
				} else {
					assert.Contains(t, err.Error(), tt.expectedError.Error())
				}
				assert.Nil(t, resp)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)
				assert.Equal(t, tt.expectedCode, resp.ShortCode)
				assert.Equal(t, tt.expectedURL, resp.ShortURL)
				assert.Equal(t, tt.request.OriginalURL, resp.OriginalURL)
				assert.False(t, resp.CreatedAt.IsZero())
			}

			mockRepo.AssertExpectations(t)
			mockGen.AssertExpectations(t)
		})
	}
}

func TestURLService_Get(t *testing.T) {
	ctx := context.Background()
	baseURL := "http://localhost:8080"

	now := time.Now()
	expiredTime := now.Add(-1 * time.Hour)
	futureTime := now.Add(24 * time.Hour)

	tests := []struct {
		name          string
		shortCode     string
		setupMocks    func(*MockURLRepository, *MockGenerator)
		expectedURL   *models.URL
		expectedError error
	}{
		{
			name:      "existing code returns URL info",
			shortCode: "abc1234",
			setupMocks: func(repo *MockURLRepository, gen *MockGenerator) {
				repo.On("GetByShortCode", ctx, "abc1234").Return(&models.URL{
					ID:          1,
					ShortCode:   "abc1234",
					OriginalURL: "https://example.com/path",
					CreatedAt:   now,
					ExpiresAt:   nil,
					ClickCount:  42,
				}, nil)
			},
			expectedURL: &models.URL{
				ID:          1,
				ShortCode:   "abc1234",
				OriginalURL: "https://example.com/path",
				CreatedAt:   now,
				ExpiresAt:   nil,
				ClickCount:  42,
			},
		},
		{
			name:      "existing code with future expiry returns URL",
			shortCode: "xyz9876",
			setupMocks: func(repo *MockURLRepository, gen *MockGenerator) {
				repo.On("GetByShortCode", ctx, "xyz9876").Return(&models.URL{
					ID:          2,
					ShortCode:   "xyz9876",
					OriginalURL: "https://example.com/other",
					CreatedAt:   now,
					ExpiresAt:   &futureTime,
					ClickCount:  10,
				}, nil)
			},
			expectedURL: &models.URL{
				ID:          2,
				ShortCode:   "xyz9876",
				OriginalURL: "https://example.com/other",
				CreatedAt:   now,
				ExpiresAt:   &futureTime,
				ClickCount:  10,
			},
		},
		{
			name:      "non-existent code returns not found error",
			shortCode: "notfound",
			setupMocks: func(repo *MockURLRepository, gen *MockGenerator) {
				repo.On("GetByShortCode", ctx, "notfound").Return(nil, models.ErrURLNotFound)
			},
			expectedError: models.ErrURLNotFound,
		},
		{
			name:      "expired URL returns expired error",
			shortCode: "expired",
			setupMocks: func(repo *MockURLRepository, gen *MockGenerator) {
				repo.On("GetByShortCode", ctx, "expired").Return(&models.URL{
					ID:          3,
					ShortCode:   "expired",
					OriginalURL: "https://example.com/expired",
					CreatedAt:   now.Add(-48 * time.Hour),
					ExpiresAt:   &expiredTime,
					ClickCount:  5,
				}, nil)
			},
			expectedError: models.ErrURLExpired,
		},
		{
			name:      "repository error returns error",
			shortCode: "error",
			setupMocks: func(repo *MockURLRepository, gen *MockGenerator) {
				repo.On("GetByShortCode", ctx, "error").Return(nil, errors.New("database error"))
			},
			expectedError: errors.New("database error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockURLRepository)
			mockGen := new(MockGenerator)

			tt.setupMocks(mockRepo, mockGen)

			svc := NewURLService(mockRepo, mockGen, baseURL)
			url, err := svc.Get(ctx, tt.shortCode)

			if tt.expectedError != nil {
				require.Error(t, err)
				if errors.Is(tt.expectedError, models.ErrURLNotFound) ||
					errors.Is(tt.expectedError, models.ErrURLExpired) {
					assert.ErrorIs(t, err, tt.expectedError)
				} else {
					assert.Contains(t, err.Error(), tt.expectedError.Error())
				}
				assert.Nil(t, url)
			} else {
				require.NoError(t, err)
				require.NotNil(t, url)
				assert.Equal(t, tt.expectedURL.ShortCode, url.ShortCode)
				assert.Equal(t, tt.expectedURL.OriginalURL, url.OriginalURL)
				assert.Equal(t, tt.expectedURL.ClickCount, url.ClickCount)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestURLService_Delete(t *testing.T) {
	ctx := context.Background()
	baseURL := "http://localhost:8080"

	tests := []struct {
		name          string
		shortCode     string
		setupMocks    func(*MockURLRepository, *MockGenerator)
		expectedError error
	}{
		{
			name:      "existing code deletes successfully",
			shortCode: "abc1234",
			setupMocks: func(repo *MockURLRepository, gen *MockGenerator) {
				repo.On("Delete", ctx, "abc1234").Return(nil)
			},
			expectedError: nil,
		},
		{
			name:      "non-existent code returns not found error",
			shortCode: "notfound",
			setupMocks: func(repo *MockURLRepository, gen *MockGenerator) {
				repo.On("Delete", ctx, "notfound").Return(models.ErrURLNotFound)
			},
			expectedError: models.ErrURLNotFound,
		},
		{
			name:      "repository error returns error",
			shortCode: "error",
			setupMocks: func(repo *MockURLRepository, gen *MockGenerator) {
				repo.On("Delete", ctx, "error").Return(errors.New("database error"))
			},
			expectedError: errors.New("database error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockURLRepository)
			mockGen := new(MockGenerator)

			tt.setupMocks(mockRepo, mockGen)

			svc := NewURLService(mockRepo, mockGen, baseURL)
			err := svc.Delete(ctx, tt.shortCode)

			if tt.expectedError != nil {
				require.Error(t, err)
				if errors.Is(tt.expectedError, models.ErrURLNotFound) {
					assert.ErrorIs(t, err, tt.expectedError)
				} else {
					assert.Contains(t, err.Error(), tt.expectedError.Error())
				}
			} else {
				require.NoError(t, err)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

// Helper functions
func durationPtr(d time.Duration) *time.Duration {
	return &d
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func TestNewURLServiceWithSanitizer(t *testing.T) {
	mockRepo := new(MockURLRepository)
	mockGen := new(MockGenerator)
	sanitizer := security.NewSanitizer(security.DefaultConfig())

	svc := NewURLServiceWithSanitizer(mockRepo, mockGen, sanitizer, "http://localhost:8080")

	assert.NotNil(t, svc)
	assert.NotNil(t, svc.repo)
	assert.NotNil(t, svc.generator)
	assert.NotNil(t, svc.sanitizer)
	assert.Equal(t, "http://localhost:8080", svc.baseURL)
}

func TestURLService_Create_WithSanitizer(t *testing.T) {
	ctx := context.Background()
	baseURL := "http://localhost:8080"

	t.Run("blocks dangerous scheme", func(t *testing.T) {
		mockRepo := new(MockURLRepository)
		mockGen := new(MockGenerator)
		sanitizer := security.NewSanitizer(security.DefaultConfig())
		svc := NewURLServiceWithSanitizer(mockRepo, mockGen, sanitizer, baseURL)

		resp, err := svc.Create(ctx, CreateURLRequest{
			OriginalURL: "javascript:alert(1)",
		})

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, ErrDangerousURL)
	})

	t.Run("blocks private IP", func(t *testing.T) {
		mockRepo := new(MockURLRepository)
		mockGen := new(MockGenerator)
		sanitizer := security.NewSanitizer(security.Config{
			MaxURLLength:    2048,
			AllowPrivateIPs: false,
		})
		svc := NewURLServiceWithSanitizer(mockRepo, mockGen, sanitizer, baseURL)

		resp, err := svc.Create(ctx, CreateURLRequest{
			OriginalURL: "http://192.168.1.1/admin",
		})

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, ErrPrivateIPURL)
	})

	t.Run("blocks URL too long", func(t *testing.T) {
		mockRepo := new(MockURLRepository)
		mockGen := new(MockGenerator)
		sanitizer := security.NewSanitizer(security.Config{
			MaxURLLength:    50,
			AllowPrivateIPs: true,
		})
		svc := NewURLServiceWithSanitizer(mockRepo, mockGen, sanitizer, baseURL)

		longURL := "https://example.com/" + string(make([]byte, 100))
		resp, err := svc.Create(ctx, CreateURLRequest{
			OriginalURL: longURL,
		})

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, ErrURLTooLong)
	})

	t.Run("blocks blocked host", func(t *testing.T) {
		mockRepo := new(MockURLRepository)
		mockGen := new(MockGenerator)
		sanitizer := security.NewSanitizer(security.Config{
			MaxURLLength:    2048,
			AllowPrivateIPs: true,
			BlockedHosts:    []string{"evil.com"},
		})
		svc := NewURLServiceWithSanitizer(mockRepo, mockGen, sanitizer, baseURL)

		resp, err := svc.Create(ctx, CreateURLRequest{
			OriginalURL: "https://evil.com/malware",
		})

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, ErrBlockedHostURL)
	})
}

func TestMapSecurityError(t *testing.T) {
	tests := []struct {
		name     string
		input    error
		expected error
	}{
		{
			name:     "dangerous scheme",
			input:    security.ErrDangerousScheme,
			expected: ErrDangerousURL,
		},
		{
			name:     "private IP",
			input:    security.ErrPrivateIP,
			expected: ErrPrivateIPURL,
		},
		{
			name:     "blocked host",
			input:    security.ErrBlockedHost,
			expected: ErrBlockedHostURL,
		},
		{
			name:     "URL too long",
			input:    security.ErrURLTooLong,
			expected: ErrURLTooLong,
		},
		{
			name:     "unknown error",
			input:    errors.New("unknown error"),
			expected: models.ErrInvalidURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapSecurityError(tt.input)
			assert.ErrorIs(t, result, tt.expected)
		})
	}
}

func TestURLService_Create_WithNilSanitizer(t *testing.T) {
	ctx := context.Background()
	baseURL := "http://localhost:8080"

	t.Run("URLCreate validation fails when sanitizer is nil", func(t *testing.T) {
		mockRepo := new(MockURLRepository)
		mockGen := new(MockGenerator)

		// Create service with nil sanitizer by using NewURLServiceWithSanitizer
		svc := NewURLServiceWithSanitizer(mockRepo, mockGen, nil, baseURL)

		// This URL has no scheme, which passes the empty check but fails URLCreate.Validate()
		resp, err := svc.Create(ctx, CreateURLRequest{
			OriginalURL: "example.com/path",
		})

		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.ErrorIs(t, err, models.ErrInvalidURL)
	})
}
