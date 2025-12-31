// Package repository handles data persistence.
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/gourl/gourl/internal/database"
	"github.com/gourl/gourl/internal/models"
)

// URLRepository defines the interface for URL persistence operations.
type URLRepository interface {
	// Create stores a new URL and returns the created entity.
	Create(ctx context.Context, url *models.URLCreate) (*models.URL, error)

	// GetByShortCode retrieves a URL by its short code.
	GetByShortCode(ctx context.Context, shortCode string) (*models.URL, error)

	// GetByID retrieves a URL by its ID.
	GetByID(ctx context.Context, id int64) (*models.URL, error)

	// Delete removes a URL by its short code.
	Delete(ctx context.Context, shortCode string) error

	// IncrementClickCount increments the click counter for a URL.
	IncrementClickCount(ctx context.Context, shortCode string) error

	// DeleteExpired removes all expired URLs and returns the count.
	DeleteExpired(ctx context.Context) (int64, error)

	// Exists checks if a short code already exists.
	Exists(ctx context.Context, shortCode string) (bool, error)

	// HealthCheck verifies the repository is healthy.
	HealthCheck(ctx context.Context) error
}

// PostgresURLRepository implements URLRepository using PostgreSQL.
type PostgresURLRepository struct {
	pool *database.Pool
}

// NewPostgresURLRepository creates a new PostgreSQL-backed URL repository.
func NewPostgresURLRepository(pool *database.Pool) *PostgresURLRepository {
	return &PostgresURLRepository{pool: pool}
}

// Create stores a new URL.
func (r *PostgresURLRepository) Create(ctx context.Context, create *models.URLCreate) (*models.URL, error) {
	if err := create.Validate(); err != nil {
		return nil, err
	}

	query := `
		INSERT INTO urls (short_code, original_url, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, short_code, original_url, created_at, expires_at, click_count
	`

	var url models.URL
	err := r.pool.QueryRow(ctx, query, create.ShortCode, create.OriginalURL, create.ExpiresAt).Scan(
		&url.ID,
		&url.ShortCode,
		&url.OriginalURL,
		&url.CreatedAt,
		&url.ExpiresAt,
		&url.ClickCount,
	)
	if err != nil {
		if isDuplicateKeyError(err) {
			return nil, fmt.Errorf("short code already exists: %s", create.ShortCode)
		}
		return nil, fmt.Errorf("failed to create URL: %w", err)
	}

	return &url, nil
}

// GetByShortCode retrieves a URL by its short code.
func (r *PostgresURLRepository) GetByShortCode(ctx context.Context, shortCode string) (*models.URL, error) {
	query := `
		SELECT id, short_code, original_url, created_at, expires_at, click_count
		FROM urls
		WHERE short_code = $1
	`

	var url models.URL
	err := r.pool.QueryRow(ctx, query, shortCode).Scan(
		&url.ID,
		&url.ShortCode,
		&url.OriginalURL,
		&url.CreatedAt,
		&url.ExpiresAt,
		&url.ClickCount,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrURLNotFound
		}
		return nil, fmt.Errorf("failed to get URL: %w", err)
	}

	return &url, nil
}

// GetByID retrieves a URL by its ID.
func (r *PostgresURLRepository) GetByID(ctx context.Context, id int64) (*models.URL, error) {
	query := `
		SELECT id, short_code, original_url, created_at, expires_at, click_count
		FROM urls
		WHERE id = $1
	`

	var url models.URL
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&url.ID,
		&url.ShortCode,
		&url.OriginalURL,
		&url.CreatedAt,
		&url.ExpiresAt,
		&url.ClickCount,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, models.ErrURLNotFound
		}
		return nil, fmt.Errorf("failed to get URL: %w", err)
	}

	return &url, nil
}

// Delete removes a URL by its short code.
func (r *PostgresURLRepository) Delete(ctx context.Context, shortCode string) error {
	query := `DELETE FROM urls WHERE short_code = $1`

	result, err := r.pool.Exec(ctx, query, shortCode)
	if err != nil {
		return fmt.Errorf("failed to delete URL: %w", err)
	}

	if result.RowsAffected() == 0 {
		return models.ErrURLNotFound
	}

	return nil
}

// IncrementClickCount increments the click counter for a URL.
func (r *PostgresURLRepository) IncrementClickCount(ctx context.Context, shortCode string) error {
	query := `UPDATE urls SET click_count = click_count + 1 WHERE short_code = $1`

	result, err := r.pool.Exec(ctx, query, shortCode)
	if err != nil {
		return fmt.Errorf("failed to increment click count: %w", err)
	}

	if result.RowsAffected() == 0 {
		return models.ErrURLNotFound
	}

	return nil
}

// DeleteExpired removes all expired URLs and returns the count.
func (r *PostgresURLRepository) DeleteExpired(ctx context.Context) (int64, error) {
	query := `DELETE FROM urls WHERE expires_at IS NOT NULL AND expires_at < $1`

	result, err := r.pool.Exec(ctx, query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired URLs: %w", err)
	}

	return result.RowsAffected(), nil
}

// Exists checks if a short code already exists.
func (r *PostgresURLRepository) Exists(ctx context.Context, shortCode string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM urls WHERE short_code = $1)`

	var exists bool
	err := r.pool.QueryRow(ctx, query, shortCode).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check existence: %w", err)
	}

	return exists, nil
}

// HealthCheck verifies the database connection is healthy.
func (r *PostgresURLRepository) HealthCheck(ctx context.Context) error {
	return r.pool.HealthCheck(ctx)
}

// isDuplicateKeyError checks if the error is a duplicate key violation.
func isDuplicateKeyError(err error) bool {
	// PostgreSQL error code for unique violation is 23505
	return err != nil && (contains(err.Error(), "23505") || contains(err.Error(), "duplicate key"))
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
