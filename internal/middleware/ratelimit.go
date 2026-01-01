package middleware

import (
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gourl/gourl/internal/ratelimit"
)

// RateLimitConfig holds configuration for the rate limit middleware.
type RateLimitConfig struct {
	TrustProxy   bool     // Trust X-Forwarded-For header
	APIKeyHeader string   // Header name for API key (e.g., "X-API-Key")
	TrustedProxies []string // List of trusted proxy IPs
}

// RateLimitResponse is the JSON response for rate limited requests.
type RateLimitResponse struct {
	Error      string `json:"error"`
	Code       string `json:"code"`
	RetryAfter int    `json:"retry_after"`
}

// RateLimit returns a middleware that rate limits requests.
// It uses the provided limiter to check if requests should be allowed.
func RateLimit(limiter ratelimit.Limiter, cfg RateLimitConfig) Middleware {
	trustedSet := make(map[string]bool)
	for _, ip := range cfg.TrustedProxies {
		trustedSet[ip] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Determine the identifier for rate limiting
			identifier := getIdentifier(r, cfg, trustedSet)

			// Check rate limit
			result, err := limiter.Allow(r.Context(), identifier)
			if err != nil {
				// Fail open on error - log and continue
				next.ServeHTTP(w, r)
				return
			}

			// Set rate limit headers
			setRateLimitHeaders(w, result)

			if !result.Allowed {
				// Rate limited
				writeRateLimitResponse(w, result)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// getIdentifier determines the rate limit identifier for the request.
// It prefers API key if configured and provided, otherwise uses client IP.
func getIdentifier(r *http.Request, cfg RateLimitConfig, trustedProxies map[string]bool) string {
	// Check for API key first
	if cfg.APIKeyHeader != "" {
		apiKey := r.Header.Get(cfg.APIKeyHeader)
		if apiKey != "" {
			return "api:" + apiKey
		}
	}

	// Get client IP
	ip := getClientIPForRateLimit(r, cfg.TrustProxy, trustedProxies)
	return "ip:" + ip
}

// getClientIPForRateLimit extracts the client IP for rate limiting.
func getClientIPForRateLimit(r *http.Request, trustProxy bool, trustedProxies map[string]bool) string {
	// First check if IP is in context (from ClientIP middleware)
	if ip := GetClientIP(r.Context()); ip != "" {
		return ip
	}

	remoteIP := extractIP(r.RemoteAddr)

	if !trustProxy {
		return remoteIP
	}

	// Check if the immediate connection is from a trusted proxy
	if len(trustedProxies) > 0 && !trustedProxies[remoteIP] {
		return remoteIP
	}

	// Check X-Forwarded-For header
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			clientIP := strings.TrimSpace(ips[0])
			if clientIP != "" {
				return clientIP
			}
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}

	return remoteIP
}

// extractIP extracts the IP address from an address string.
func extractIP(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

// setRateLimitHeaders sets the rate limit headers on the response.
func setRateLimitHeaders(w http.ResponseWriter, result *ratelimit.Result) {
	w.Header().Set("X-RateLimit-Limit", strconv.Itoa(result.Limit))
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(result.Remaining))

	if result.ResetAfter > 0 {
		resetTime := time.Now().Add(result.ResetAfter).Unix()
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTime, 10))
	}

	if !result.Allowed && result.RetryAfter > 0 {
		retrySeconds := int(result.RetryAfter.Seconds())
		if retrySeconds < 1 {
			retrySeconds = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(retrySeconds))
	}
}

// writeRateLimitResponse writes the 429 response.
func writeRateLimitResponse(w http.ResponseWriter, result *ratelimit.Result) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)

	retrySeconds := int(result.RetryAfter.Seconds())
	if retrySeconds < 1 {
		retrySeconds = 1
	}

	resp := RateLimitResponse{
		Error:      "rate limit exceeded",
		Code:       "RATE_LIMIT_EXCEEDED",
		RetryAfter: retrySeconds,
	}

	json.NewEncoder(w).Encode(resp)
}
