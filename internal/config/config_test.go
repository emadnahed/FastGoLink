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
