// Package ratelimit provides rate limiting functionality.
package ratelimit

import (
	"context"
	"errors"
	"time"
)

// ErrRateLimitExceeded is returned when the rate limit is exceeded.
var ErrRateLimitExceeded = errors.New("rate limit exceeded")

// Result contains the outcome of a rate limit check.
type Result struct {
	Allowed    bool          // Whether the request is allowed
	Remaining  int           // Remaining requests in the current window
	ResetAfter time.Duration // Time until the oldest request expires
	RetryAfter time.Duration // Suggested retry time (if blocked)
	Limit      int           // The configured limit
}

// Limiter defines the rate limiting interface.
type Limiter interface {
	// Allow checks if a request from the given identifier is allowed.
	// Returns the result and any error encountered.
	Allow(ctx context.Context, identifier string) (*Result, error)

	// Reset clears the rate limit state for an identifier.
	Reset(ctx context.Context, identifier string) error

	// Close releases any resources held by the limiter.
	Close() error
}

// Config holds rate limiter configuration.
type Config struct {
	Requests int           // Maximum requests per window
	Window   time.Duration // Time window size
}

// DefaultConfig returns a default configuration.
func DefaultConfig() Config {
	return Config{
		Requests: 100,
		Window:   time.Minute,
	}
}
