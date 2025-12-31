// Package config handles application configuration.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all configuration for the application.
type Config struct {
	App      AppConfig
	Server   ServerConfig
	Database DatabaseConfig
	Redis    RedisConfig
	URL      URLConfig
	Rate     RateLimitConfig
}

// AppConfig holds application-level configuration.
type AppConfig struct {
	Env      string
	LogLevel string
}

// IsDevelopment returns true if the app is running in development mode.
func (a AppConfig) IsDevelopment() bool {
	return a.Env == "development" || a.Env == "dev"
}

// IsProduction returns true if the app is running in production mode.
func (a AppConfig) IsProduction() bool {
	return a.Env == "production" || a.Env == "prod"
}

// ServerConfig holds server-specific configuration.
type ServerConfig struct {
	Host            string
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// Address returns the server address in host:port format.
func (s ServerConfig) Address() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// DatabaseConfig holds database connection configuration.
type DatabaseConfig struct {
	Host            string
	Port            int
	User            string
	Password        string
	DBName          string
	SSLMode         string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// RedisConfig holds Redis connection configuration.
type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
	PoolSize int
}

// URLConfig holds URL shortener specific configuration.
type URLConfig struct {
	BaseURL       string
	ShortCodeLen  int
	DefaultExpiry time.Duration
	IDGenStrategy string
}

// RateLimitConfig holds rate limiting configuration.
type RateLimitConfig struct {
	Requests int
	Window   time.Duration
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{}

	// App config
	cfg.App.Env = getEnvOrDefault("APP_ENV", "development")
	cfg.App.LogLevel = getEnvOrDefault("LOG_LEVEL", "info")

	// Server config
	cfg.Server.Host = getEnvOrDefault("SERVER_HOST", "0.0.0.0")

	port, err := getEnvAsInt("SERVER_PORT", 8080)
	if err != nil {
		return nil, fmt.Errorf("invalid SERVER_PORT: %w", err)
	}
	cfg.Server.Port = port

	readTimeout, err := getEnvAsDuration("SERVER_READ_TIMEOUT", 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("invalid SERVER_READ_TIMEOUT: %w", err)
	}
	cfg.Server.ReadTimeout = readTimeout

	writeTimeout, err := getEnvAsDuration("SERVER_WRITE_TIMEOUT", 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("invalid SERVER_WRITE_TIMEOUT: %w", err)
	}
	cfg.Server.WriteTimeout = writeTimeout

	shutdownTimeout, err := getEnvAsDuration("SERVER_SHUTDOWN_TIMEOUT", 30*time.Second)
	if err != nil {
		return nil, fmt.Errorf("invalid SERVER_SHUTDOWN_TIMEOUT: %w", err)
	}
	cfg.Server.ShutdownTimeout = shutdownTimeout

	// Database config
	cfg.Database.Host = getEnvOrDefault("DB_HOST", "localhost")
	dbPort, err := getEnvAsInt("DB_PORT", 5432)
	if err != nil {
		return nil, fmt.Errorf("invalid DB_PORT: %w", err)
	}
	cfg.Database.Port = dbPort
	cfg.Database.User = getEnvOrDefault("DB_USER", "gourl")
	cfg.Database.Password = getEnvOrDefault("DB_PASSWORD", "")
	cfg.Database.DBName = getEnvOrDefault("DB_NAME", "gourl")
	cfg.Database.SSLMode = getEnvOrDefault("DB_SSLMODE", "disable")

	maxOpenConns, err := getEnvAsInt("DB_MAX_OPEN_CONNS", 25)
	if err != nil {
		return nil, fmt.Errorf("invalid DB_MAX_OPEN_CONNS: %w", err)
	}
	cfg.Database.MaxOpenConns = maxOpenConns

	maxIdleConns, err := getEnvAsInt("DB_MAX_IDLE_CONNS", 5)
	if err != nil {
		return nil, fmt.Errorf("invalid DB_MAX_IDLE_CONNS: %w", err)
	}
	cfg.Database.MaxIdleConns = maxIdleConns

	connMaxLifetime, err := getEnvAsDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("invalid DB_CONN_MAX_LIFETIME: %w", err)
	}
	cfg.Database.ConnMaxLifetime = connMaxLifetime

	// Redis config
	cfg.Redis.Host = getEnvOrDefault("REDIS_HOST", "localhost")
	redisPort, err := getEnvAsInt("REDIS_PORT", 6379)
	if err != nil {
		return nil, fmt.Errorf("invalid REDIS_PORT: %w", err)
	}
	cfg.Redis.Port = redisPort
	cfg.Redis.Password = getEnvOrDefault("REDIS_PASSWORD", "")
	redisDB, err := getEnvAsInt("REDIS_DB", 0)
	if err != nil {
		return nil, fmt.Errorf("invalid REDIS_DB: %w", err)
	}
	cfg.Redis.DB = redisDB
	redisPoolSize, err := getEnvAsInt("REDIS_POOL_SIZE", 10)
	if err != nil {
		return nil, fmt.Errorf("invalid REDIS_POOL_SIZE: %w", err)
	}
	cfg.Redis.PoolSize = redisPoolSize

	return cfg, nil
}

// DatabaseEnabled returns true if database configuration is provided.
func (c *Config) DatabaseEnabled() bool {
	return c.Database.Host != "" && c.Database.Password != ""
}

// RedisEnabled returns true if Redis configuration is provided.
func (c *Config) RedisEnabled() bool {
	return c.Redis.Host != ""
}

// getEnvOrDefault returns the environment variable value or a default.
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvAsInt returns the environment variable as an integer.
func getEnvAsInt(key string, defaultValue int) (int, error) {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return 0, err
	}
	return value, nil
}

// getEnvAsDuration returns the environment variable as a duration.
func getEnvAsDuration(key string, defaultValue time.Duration) (time.Duration, error) {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue, nil
	}
	value, err := time.ParseDuration(valueStr)
	if err != nil {
		return 0, err
	}
	return value, nil
}
