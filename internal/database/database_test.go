package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/emadnahed/FastGoLink/internal/config"
)

func skipIfNoPostgres(t *testing.T) {
	t.Helper()
	if os.Getenv("TEST_POSTGRES") != "true" {
		t.Skip("Skipping: TEST_POSTGRES not set. Run with docker-compose up -d")
	}
}

func testDBConfig() *config.DatabaseConfig {
	return &config.DatabaseConfig{
		Host:            getEnvOrDefault("DB_HOST", "localhost"),
		Port:            5432,
		User:            getEnvOrDefault("DB_USER", "fastgolink"),
		Password:        getEnvOrDefault("DB_PASSWORD", "fastgolink_dev_password"),
		DBName:          getEnvOrDefault("DB_NAME", "fastgolink"),
		SSLMode:         "disable",
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
	}
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func TestNewPool(t *testing.T) {
	skipIfNoPostgres(t)

	cfg := testDBConfig()
	ctx := context.Background()

	pool, err := NewPool(ctx, cfg)
	require.NoError(t, err)
	assert.NotNil(t, pool)

	defer pool.Close()

	// Verify we can ping
	err = pool.Ping(ctx)
	assert.NoError(t, err)
}

func TestPool_Ping(t *testing.T) {
	skipIfNoPostgres(t)

	cfg := testDBConfig()
	ctx := context.Background()

	pool, err := NewPool(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()

	err = pool.Ping(ctx)
	assert.NoError(t, err)
}

func TestPool_Stats(t *testing.T) {
	skipIfNoPostgres(t)

	cfg := testDBConfig()
	ctx := context.Background()

	pool, err := NewPool(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()

	stats := pool.Stats()
	assert.NotNil(t, stats)
	assert.GreaterOrEqual(t, stats.MaxConns, int32(1))
}

func TestPool_Close(t *testing.T) {
	skipIfNoPostgres(t)

	cfg := testDBConfig()
	ctx := context.Background()

	pool, err := NewPool(ctx, cfg)
	require.NoError(t, err)

	pool.Close()

	// After close, ping should fail
	err = pool.Ping(ctx)
	assert.Error(t, err)
}

func TestNewPool_InvalidConfig(t *testing.T) {
	cfg := &config.DatabaseConfig{
		Host:     "invalid-host-that-does-not-exist",
		Port:     5432,
		User:     "invalid",
		Password: "invalid",
		DBName:   "invalid",
		SSLMode:  "disable",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := NewPool(ctx, cfg)
	assert.Error(t, err)
}

func TestBuildDSN(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.DatabaseConfig
		expected string
	}{
		{
			name: "basic config",
			cfg: &config.DatabaseConfig{
				Host:     "localhost",
				Port:     5432,
				User:     "user",
				Password: "pass",
				DBName:   "testdb",
				SSLMode:  "disable",
			},
			expected: "postgres://user:pass@localhost:5432/testdb?sslmode=disable",
		},
		{
			name: "with pool settings",
			cfg: &config.DatabaseConfig{
				Host:            "db.example.com",
				Port:            5433,
				User:            "admin",
				Password:        "secret",
				DBName:          "production",
				SSLMode:         "require",
				MaxOpenConns:    25,
				MaxIdleConns:    10,
				ConnMaxLifetime: 10 * time.Minute,
			},
			expected: "postgres://admin:secret@db.example.com:5433/production?sslmode=require",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsn := BuildDSN(tt.cfg)
			assert.Equal(t, tt.expected, dsn)
		})
	}
}
