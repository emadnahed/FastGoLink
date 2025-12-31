// Package models contains domain models and entities.
package models

import (
	"errors"
	"net/url"
	"strings"
	"time"
)

// URL represents a shortened URL entity.
type URL struct {
	ID          int64      `json:"id"`
	ShortCode   string     `json:"short_code"`
	OriginalURL string     `json:"original_url"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	ClickCount  int64      `json:"click_count"`
}

// URLCreate represents the data needed to create a new URL.
type URLCreate struct {
	OriginalURL string
	ShortCode   string
	ExpiresAt   *time.Time
}

// Validation errors
var (
	ErrEmptyURL        = errors.New("url cannot be empty")
	ErrInvalidURL      = errors.New("invalid url format")
	ErrEmptyShortCode  = errors.New("short code cannot be empty")
	ErrShortCodeLength = errors.New("short code must be between 1 and 10 characters")
	ErrURLExpired      = errors.New("url has expired")
	ErrURLNotFound     = errors.New("url not found")
)

// Validate validates the URL model.
func (u *URL) Validate() error {
	if u.ShortCode == "" {
		return ErrEmptyShortCode
	}
	if len(u.ShortCode) > 10 {
		return ErrShortCodeLength
	}
	if u.OriginalURL == "" {
		return ErrEmptyURL
	}
	if !isValidURL(u.OriginalURL) {
		return ErrInvalidURL
	}
	return nil
}

// IsExpired checks if the URL has expired.
func (u *URL) IsExpired() bool {
	if u.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*u.ExpiresAt)
}

// Validate validates the URLCreate data.
func (c *URLCreate) Validate() error {
	if c.OriginalURL == "" {
		return ErrEmptyURL
	}
	if !isValidURL(c.OriginalURL) {
		return ErrInvalidURL
	}
	if c.ShortCode != "" {
		if len(c.ShortCode) > 10 {
			return ErrShortCodeLength
		}
	}
	return nil
}

// isValidURL checks if the string is a valid URL.
func isValidURL(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}

	u, err := url.Parse(s)
	if err != nil {
		return false
	}

	// Must have a scheme (http or https)
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}

	// Must have a host
	if u.Host == "" {
		return false
	}

	return true
}
