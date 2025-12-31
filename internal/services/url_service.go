// Package services contains business logic.
package services

import (
	"context"
	"fmt"
	"time"

	"github.com/gourl/gourl/internal/idgen"
	"github.com/gourl/gourl/internal/models"
	"github.com/gourl/gourl/internal/repository"
)

// CreateURLRequest represents the input for creating a short URL.
type CreateURLRequest struct {
	OriginalURL string
	ExpiresIn   *time.Duration
}

// CreateURLResponse represents the result of creating a short URL.
type CreateURLResponse struct {
	ShortURL    string
	ShortCode   string
	OriginalURL string
	CreatedAt   time.Time
	ExpiresAt   *time.Time
}

// URLService defines the interface for URL shortening operations.
type URLService interface {
	Create(ctx context.Context, req CreateURLRequest) (*CreateURLResponse, error)
	Get(ctx context.Context, shortCode string) (*models.URL, error)
	Delete(ctx context.Context, shortCode string) error
}

// URLServiceImpl implements URLService.
type URLServiceImpl struct {
	repo      repository.URLRepository
	generator idgen.Generator
	baseURL   string
}

// NewURLService creates a new URLService instance.
func NewURLService(repo repository.URLRepository, gen idgen.Generator, baseURL string) *URLServiceImpl {
	return &URLServiceImpl{
		repo:      repo,
		generator: gen,
		baseURL:   baseURL,
	}
}

// Create creates a new short URL.
func (s *URLServiceImpl) Create(ctx context.Context, req CreateURLRequest) (*CreateURLResponse, error) {
	// Validate the original URL first
	if req.OriginalURL == "" {
		return nil, models.ErrEmptyURL
	}

	// Use URLCreate's validation for URL format
	urlCreate := &models.URLCreate{
		OriginalURL: req.OriginalURL,
	}
	if err := urlCreate.Validate(); err != nil {
		return nil, err
	}

	// Generate short code
	shortCode, err := s.generator.Generate()
	if err != nil {
		return nil, err
	}

	// Calculate expiry time if provided
	var expiresAt *time.Time
	if req.ExpiresIn != nil {
		exp := time.Now().Add(*req.ExpiresIn)
		expiresAt = &exp
	}

	// Create the URL in repository
	urlCreate.ShortCode = shortCode
	urlCreate.ExpiresAt = expiresAt

	url, err := s.repo.Create(ctx, urlCreate)
	if err != nil {
		return nil, err
	}

	return &CreateURLResponse{
		ShortURL:    fmt.Sprintf("%s/%s", s.baseURL, url.ShortCode),
		ShortCode:   url.ShortCode,
		OriginalURL: url.OriginalURL,
		CreatedAt:   url.CreatedAt,
		ExpiresAt:   url.ExpiresAt,
	}, nil
}

// Get retrieves a URL by its short code.
func (s *URLServiceImpl) Get(ctx context.Context, shortCode string) (*models.URL, error) {
	url, err := s.repo.GetByShortCode(ctx, shortCode)
	if err != nil {
		return nil, err
	}

	// Check if URL has expired
	if url.IsExpired() {
		return nil, models.ErrURLExpired
	}

	return url, nil
}

// Delete removes a URL by its short code.
func (s *URLServiceImpl) Delete(ctx context.Context, shortCode string) error {
	return s.repo.Delete(ctx, shortCode)
}
