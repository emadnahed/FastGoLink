package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setEnv sets an environment variable for the duration of a test.
func setEnv(t *testing.T, key, value string) {
	t.Helper()
	old, existed := os.LookupEnv(key)
	require.NoError(t, os.Setenv(key, value))
	t.Cleanup(func() {
		if existed {
			os.Setenv(key, old)
		} else {
			os.Unsetenv(key)
		}
	})
}

// clearEnv clears an environment variable for the duration of a test.
func clearEnv(t *testing.T, key string) {
	t.Helper()
	old, existed := os.LookupEnv(key)
	os.Unsetenv(key)
	t.Cleanup(func() {
		if existed {
			os.Setenv(key, old)
		}
	})
}

func TestLoad_Defaults(t *testing.T) {
	// Clear all relevant env vars to test defaults
	envVars := []string{
		"SERVER_HOST", "SERVER_PORT", "SERVER_READ_TIMEOUT",
		"SERVER_WRITE_TIMEOUT", "SERVER_SHUTDOWN_TIMEOUT",
		"APP_ENV", "LOG_LEVEL",
	}
	for _, v := range envVars {
		clearEnv(t, v)
	}

	cfg, err := Load()
	require.NoError(t, err)

	// Server defaults
	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, 5*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, 10*time.Second, cfg.Server.WriteTimeout)
	assert.Equal(t, 30*time.Second, cfg.Server.ShutdownTimeout)

	// App defaults
	assert.Equal(t, "development", cfg.App.Env)
	assert.Equal(t, "info", cfg.App.LogLevel)
}

func TestLoad_ServerConfig(t *testing.T) {
	setEnv(t, "SERVER_HOST", "127.0.0.1")
	setEnv(t, "SERVER_PORT", "3000")
	setEnv(t, "SERVER_READ_TIMEOUT", "10s")
	setEnv(t, "SERVER_WRITE_TIMEOUT", "20s")
	setEnv(t, "SERVER_SHUTDOWN_TIMEOUT", "60s")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1", cfg.Server.Host)
	assert.Equal(t, 3000, cfg.Server.Port)
	assert.Equal(t, 10*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, 20*time.Second, cfg.Server.WriteTimeout)
	assert.Equal(t, 60*time.Second, cfg.Server.ShutdownTimeout)
}

func TestLoad_AppConfig(t *testing.T) {
	setEnv(t, "APP_ENV", "production")
	setEnv(t, "LOG_LEVEL", "error")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "production", cfg.App.Env)
	assert.Equal(t, "error", cfg.App.LogLevel)
}

func TestLoad_InvalidPort(t *testing.T) {
	setEnv(t, "SERVER_PORT", "not-a-number")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SERVER_PORT")
}

func TestLoad_InvalidTimeout(t *testing.T) {
	setEnv(t, "SERVER_READ_TIMEOUT", "invalid")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SERVER_READ_TIMEOUT")
}

func TestConfig_Address(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{
			Host: "localhost",
			Port: 8080,
		},
	}

	assert.Equal(t, "localhost:8080", cfg.Server.Address())
}

func TestConfig_IsDevelopment(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		expected bool
	}{
		{"development", "development", true},
		{"dev", "dev", true},
		{"production", "production", false},
		{"staging", "staging", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{App: AppConfig{Env: tt.env}}
			assert.Equal(t, tt.expected, cfg.App.IsDevelopment())
		})
	}
}

func TestConfig_IsProduction(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		expected bool
	}{
		{"production", "production", true},
		{"prod", "prod", true},
		{"development", "development", false},
		{"staging", "staging", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{App: AppConfig{Env: tt.env}}
			assert.Equal(t, tt.expected, cfg.App.IsProduction())
		})
	}
}

func TestSecurityConfig_BlockedHostsList(t *testing.T) {
	tests := []struct {
		name     string
		hosts    string
		expected []string
	}{
		{
			name:     "empty string returns nil",
			hosts:    "",
			expected: nil,
		},
		{
			name:     "single host",
			hosts:    "evil.com",
			expected: []string{"evil.com"},
		},
		{
			name:     "multiple hosts",
			hosts:    "evil.com,bad.com,malware.org",
			expected: []string{"evil.com", "bad.com", "malware.org"},
		},
		{
			name:     "hosts with spaces",
			hosts:    " evil.com , bad.com , malware.org ",
			expected: []string{"evil.com", "bad.com", "malware.org"},
		},
		{
			name:     "hosts with empty entries",
			hosts:    "evil.com,,bad.com,",
			expected: []string{"evil.com", "bad.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := SecurityConfig{BlockedHosts: tt.hosts}
			result := cfg.BlockedHostsList()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestConfig_DatabaseEnabled(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		password string
		expected bool
	}{
		{
			name:     "both host and password set",
			host:     "localhost",
			password: "secret",
			expected: true,
		},
		{
			name:     "only host set",
			host:     "localhost",
			password: "",
			expected: false,
		},
		{
			name:     "only password set",
			host:     "",
			password: "secret",
			expected: false,
		},
		{
			name:     "neither set",
			host:     "",
			password: "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Database: DatabaseConfig{
					Host:     tt.host,
					Password: tt.password,
				},
			}
			assert.Equal(t, tt.expected, cfg.DatabaseEnabled())
		})
	}
}

func TestConfig_RedisEnabled(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected bool
	}{
		{
			name:     "host set",
			host:     "localhost",
			expected: true,
		},
		{
			name:     "host not set",
			host:     "",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Redis: RedisConfig{Host: tt.host},
			}
			assert.Equal(t, tt.expected, cfg.RedisEnabled())
		})
	}
}

func TestLoad_DatabaseConfig(t *testing.T) {
	// Clear any existing DB env vars first
	clearEnv(t, "DB_HOST")
	clearEnv(t, "DB_PORT")
	clearEnv(t, "DB_USER")
	clearEnv(t, "DB_PASSWORD")
	clearEnv(t, "DB_NAME")
	clearEnv(t, "DB_SSLMODE")

	setEnv(t, "DB_HOST", "db.example.com")
	setEnv(t, "DB_PORT", "5433")
	setEnv(t, "DB_USER", "testuser")
	setEnv(t, "DB_PASSWORD", "testpass")
	setEnv(t, "DB_NAME", "testdb")
	setEnv(t, "DB_SSLMODE", "require")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "db.example.com", cfg.Database.Host)
	assert.Equal(t, 5433, cfg.Database.Port)
	assert.Equal(t, "testuser", cfg.Database.User)
	assert.Equal(t, "testpass", cfg.Database.Password)
	assert.Equal(t, "testdb", cfg.Database.DBName)
	assert.Equal(t, "require", cfg.Database.SSLMode)
	assert.True(t, cfg.DatabaseEnabled())
}

func TestLoad_RedisConfig(t *testing.T) {
	clearEnv(t, "REDIS_HOST")
	clearEnv(t, "REDIS_PORT")
	clearEnv(t, "REDIS_PASSWORD")

	setEnv(t, "REDIS_HOST", "redis.example.com")
	setEnv(t, "REDIS_PORT", "6380")
	setEnv(t, "REDIS_PASSWORD", "redispass")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, "redis.example.com", cfg.Redis.Host)
	assert.Equal(t, 6380, cfg.Redis.Port)
	assert.Equal(t, "redispass", cfg.Redis.Password)
	assert.True(t, cfg.RedisEnabled())
}

func TestLoad_SecurityConfig(t *testing.T) {
	setEnv(t, "SECURITY_MAX_URL_LENGTH", "4096")
	setEnv(t, "SECURITY_ALLOW_PRIVATE_IPS", "true")
	setEnv(t, "SECURITY_BLOCKED_HOSTS", "evil.com,bad.com")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, 4096, cfg.Security.MaxURLLength)
	assert.True(t, cfg.Security.AllowPrivateIPs)
	assert.Equal(t, "evil.com,bad.com", cfg.Security.BlockedHosts)
	assert.Equal(t, []string{"evil.com", "bad.com"}, cfg.Security.BlockedHostsList())
}

func TestLoad_RateLimitConfig(t *testing.T) {
	setEnv(t, "RATE_LIMIT_ENABLED", "true")
	setEnv(t, "RATE_LIMIT_REQUESTS", "50")
	setEnv(t, "RATE_LIMIT_WINDOW", "30s")

	cfg, err := Load()
	require.NoError(t, err)

	assert.True(t, cfg.Rate.Enabled)
	assert.Equal(t, 50, cfg.Rate.Requests)
}

func TestLoad_InvalidDatabasePort(t *testing.T) {
	setEnv(t, "DB_PORT", "invalid")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "DB_PORT")
}

func TestLoad_InvalidRedisPort(t *testing.T) {
	setEnv(t, "REDIS_PORT", "invalid")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "REDIS_PORT")
}

func TestLoad_InvalidWriteTimeout(t *testing.T) {
	setEnv(t, "SERVER_WRITE_TIMEOUT", "invalid")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SERVER_WRITE_TIMEOUT")
}

func TestLoad_InvalidShutdownTimeout(t *testing.T) {
	setEnv(t, "SERVER_SHUTDOWN_TIMEOUT", "invalid")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SERVER_SHUTDOWN_TIMEOUT")
}

func TestLoad_InvalidDBMaxOpenConns(t *testing.T) {
	setEnv(t, "DB_MAX_OPEN_CONNS", "invalid")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "DB_MAX_OPEN_CONNS")
}

func TestLoad_InvalidDBMaxIdleConns(t *testing.T) {
	setEnv(t, "DB_MAX_IDLE_CONNS", "invalid")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "DB_MAX_IDLE_CONNS")
}

func TestLoad_InvalidDBConnMaxLifetime(t *testing.T) {
	setEnv(t, "DB_CONN_MAX_LIFETIME", "invalid")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "DB_CONN_MAX_LIFETIME")
}

func TestLoad_InvalidRedisPoolSize(t *testing.T) {
	setEnv(t, "REDIS_POOL_SIZE", "invalid")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "REDIS_POOL_SIZE")
}

func TestLoad_InvalidRedisDB(t *testing.T) {
	setEnv(t, "REDIS_DB", "invalid")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "REDIS_DB")
}

func TestLoad_InvalidMaxURLLength(t *testing.T) {
	setEnv(t, "SECURITY_MAX_URL_LENGTH", "invalid")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SECURITY_MAX_URL_LENGTH")
}

func TestLoad_InvalidRateLimitRequests(t *testing.T) {
	setEnv(t, "RATE_LIMIT_REQUESTS", "invalid")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "RATE_LIMIT_REQUESTS")
}

func TestLoad_InvalidRateLimitWindow(t *testing.T) {
	setEnv(t, "RATE_LIMIT_WINDOW", "invalid")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "RATE_LIMIT_WINDOW")
}

func TestLoad_InvalidRedisCacheTTL(t *testing.T) {
	setEnv(t, "REDIS_CACHE_TTL", "invalid")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "REDIS_CACHE_TTL")
}

func TestLoad_InvalidURLShortCodeLen(t *testing.T) {
	setEnv(t, "URL_SHORT_CODE_LEN", "invalid")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "URL_SHORT_CODE_LEN")
}

func TestLoad_InvalidURLIDGenMaxRetries(t *testing.T) {
	setEnv(t, "URL_IDGEN_MAX_RETRIES", "invalid")

	_, err := Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "URL_IDGEN_MAX_RETRIES")
}
