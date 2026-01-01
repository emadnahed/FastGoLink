// Package database provides PostgreSQL database connectivity.
package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/emadnahed/FastGoLink/internal/config"
)

// Pool wraps pgxpool.Pool with additional functionality.
type Pool struct {
	*pgxpool.Pool
}

// Stats represents pool statistics.
type Stats struct {
	MaxConns          int32
	TotalConns        int32
	IdleConns         int32
	AcquiredConns     int32
	AcquireCount      int64
	AcquireDuration   int64
	EmptyAcquireCount int64
}

// NewPool creates a new database connection pool.
func NewPool(ctx context.Context, cfg *config.DatabaseConfig) (*Pool, error) {
	dsn := BuildDSN(cfg)

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	// Configure pool settings
	if cfg.MaxOpenConns > 0 && cfg.MaxOpenConns <= 1000 {
		poolConfig.MaxConns = int32(cfg.MaxOpenConns)
	} else {
		poolConfig.MaxConns = 10
	}
	if cfg.MaxIdleConns > 0 && cfg.MaxIdleConns <= 1000 {
		poolConfig.MinConns = int32(cfg.MaxIdleConns)
	}
	poolConfig.MaxConnLifetime = cfg.ConnMaxLifetime

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Verify connectivity
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &Pool{Pool: pool}, nil
}

// BuildDSN constructs a PostgreSQL connection string.
func BuildDSN(cfg *config.DatabaseConfig) string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.DBName,
		cfg.SSLMode,
	)
}

// Stats returns pool statistics.
func (p *Pool) Stats() *Stats {
	s := p.Pool.Stat()
	return &Stats{
		MaxConns:          s.MaxConns(),
		TotalConns:        s.TotalConns(),
		IdleConns:         s.IdleConns(),
		AcquiredConns:     s.AcquiredConns(),
		AcquireCount:      s.AcquireCount(),
		AcquireDuration:   int64(s.AcquireDuration()),
		EmptyAcquireCount: s.EmptyAcquireCount(),
	}
}

// HealthCheck performs a database health check.
func (p *Pool) HealthCheck(ctx context.Context) error {
	return p.Ping(ctx)
}
