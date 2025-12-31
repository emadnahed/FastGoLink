// Package config handles application configuration.
package config

import "time"

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

// ServerConfig holds server-specific configuration.
type ServerConfig struct {
	Host            string
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
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
