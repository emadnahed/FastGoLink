// Package security provides URL sanitization and security utilities.
package security

import (
	"errors"
	"net"
	"net/url"
	"strings"
)

// Sanitization errors
var (
	ErrDangerousScheme = errors.New("dangerous URL scheme detected")
	ErrPrivateIP       = errors.New("private IP addresses not allowed")
	ErrBlockedHost     = errors.New("host is blocked")
	ErrURLTooLong      = errors.New("URL exceeds maximum length")
	ErrInvalidURL      = errors.New("invalid URL format")
	ErrEmptyURL        = errors.New("URL cannot be empty")
	ErrInvalidScheme   = errors.New("URL must use http or https scheme")
)

// dangerousSchemes contains URL schemes that can execute code.
var dangerousSchemes = map[string]bool{
	"javascript": true,
	"data":       true,
	"vbscript":   true,
	"file":       true,
}

// Config holds sanitizer configuration.
type Config struct {
	MaxURLLength    int      // Maximum allowed URL length
	AllowPrivateIPs bool     // Allow localhost, 10.x, 192.168.x, etc.
	BlockedHosts    []string // Explicitly blocked hostnames
}

// DefaultConfig returns the default sanitizer configuration.
func DefaultConfig() Config {
	return Config{
		MaxURLLength:    2048,
		AllowPrivateIPs: false,
		BlockedHosts:    nil,
	}
}

// Sanitizer validates and sanitizes URLs.
type Sanitizer struct {
	config       Config
	blockedHosts map[string]bool
}

// NewSanitizer creates a new URL sanitizer.
func NewSanitizer(cfg Config) *Sanitizer {
	blockedHosts := make(map[string]bool)
	for _, host := range cfg.BlockedHosts {
		blockedHosts[strings.ToLower(host)] = true
	}

	return &Sanitizer{
		config:       cfg,
		blockedHosts: blockedHosts,
	}
}

// Validate checks if a URL is safe and valid.
func (s *Sanitizer) Validate(rawURL string) error {
	// Check for empty URL
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ErrEmptyURL
	}

	// Check URL length
	if len(rawURL) > s.config.MaxURLLength {
		return ErrURLTooLong
	}

	// Parse the URL
	u, err := url.Parse(rawURL)
	if err != nil {
		return ErrInvalidURL
	}

	// Check scheme
	scheme := strings.ToLower(u.Scheme)
	if scheme == "" {
		return ErrInvalidScheme
	}

	// Check for dangerous schemes
	if dangerousSchemes[scheme] {
		return ErrDangerousScheme
	}

	// Only allow http and https
	if scheme != "http" && scheme != "https" {
		return ErrInvalidScheme
	}

	// Check host
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return ErrInvalidURL
	}

	// Check for blocked hosts
	if s.isBlockedHost(host) {
		return ErrBlockedHost
	}

	// Check for private IPs
	if !s.config.AllowPrivateIPs {
		if isPrivateHost(host) {
			return ErrPrivateIP
		}
	}

	return nil
}

// isBlockedHost checks if a host or any of its parent domains is blocked.
func (s *Sanitizer) isBlockedHost(host string) bool {
	// Check exact match
	if s.blockedHosts[host] {
		return true
	}

	// Check parent domains
	parts := strings.Split(host, ".")
	for i := 1; i < len(parts); i++ {
		parent := strings.Join(parts[i:], ".")
		if s.blockedHosts[parent] {
			return true
		}
	}

	return false
}

// isPrivateHost checks if a host is a private/local address.
func isPrivateHost(host string) bool {
	// Check for localhost
	if host == "localhost" {
		return true
	}

	// Try to parse as IP
	return isPrivateIP(host)
}

// isPrivateIP checks if an IP address is private/local.
func isPrivateIP(ipStr string) bool {
	// Handle IPv6 addresses in brackets
	ipStr = strings.TrimPrefix(ipStr, "[")
	ipStr = strings.TrimSuffix(ipStr, "]")

	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	// Check for loopback
	if ip.IsLoopback() {
		return true
	}

	// Check for private networks
	if ip.IsPrivate() {
		return true
	}

	// Check for link-local
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// Check for unspecified (0.0.0.0 or ::)
	if ip.IsUnspecified() {
		return true
	}

	return false
}
