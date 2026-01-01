package middleware

import (
	"context"
	"net"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

const (
	// HeaderXRequestID is the header name for request ID.
	HeaderXRequestID = "X-Request-ID"
	// HeaderXForwardedFor is the header name for forwarded client IP.
	HeaderXForwardedFor = "X-Forwarded-For"
	// HeaderXRealIP is the header name for real client IP.
	HeaderXRealIP = "X-Real-IP"
)

// requestIDMaxLength is the maximum length for a valid request ID.
const requestIDMaxLength = 128

// validRequestIDRegex matches alphanumeric strings with dashes and underscores.
var validRequestIDRegex = regexp.MustCompile(`^[a-zA-Z0-9\-_]+$`)

// RequestID returns a middleware that adds a unique request ID to each request.
// If the request already has a valid X-Request-ID header, it will be used.
// Otherwise, a new UUID v4 will be generated.
func RequestID() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requestID := r.Header.Get(HeaderXRequestID)

			// Validate existing request ID or generate a new one
			if !isValidRequestID(requestID) {
				requestID = uuid.New().String()
			}

			// Set the request ID in the response header
			w.Header().Set(HeaderXRequestID, requestID)

			// Add to context
			ctx := context.WithValue(r.Context(), RequestIDKey, requestID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// isValidRequestID checks if the request ID is valid.
// Valid IDs are non-empty, not too long, and contain only safe characters.
func isValidRequestID(id string) bool {
	if id == "" || len(id) > requestIDMaxLength {
		return false
	}
	return validRequestIDRegex.MatchString(id)
}

// ClientIP returns a middleware that extracts the client IP address and stores it in context.
// If trustProxy is true, it will check X-Forwarded-For and X-Real-IP headers.
// trustedProxies can be used to limit which proxy IPs are trusted.
func ClientIP(trustProxy bool, trustedProxies []string) Middleware {
	trustedSet := make(map[string]bool)
	for _, ip := range trustedProxies {
		trustedSet[ip] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			clientIP := extractClientIP(r, trustProxy, trustedSet)

			// Add to context
			ctx := context.WithValue(r.Context(), ClientIPKey, clientIP)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractClientIP extracts the client IP from the request.
func extractClientIP(r *http.Request, trustProxy bool, trustedProxies map[string]bool) string {
	remoteIP := extractIPFromAddr(r.RemoteAddr)

	if !trustProxy {
		return remoteIP
	}

	// Check if the immediate connection is from a trusted proxy
	if len(trustedProxies) > 0 && !trustedProxies[remoteIP] {
		return remoteIP
	}

	// Check X-Forwarded-For header first
	if xff := r.Header.Get(HeaderXForwardedFor); xff != "" {
		// X-Forwarded-For can contain multiple IPs: client, proxy1, proxy2
		// The first IP is typically the original client
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			clientIP := strings.TrimSpace(ips[0])
			if clientIP != "" {
				return clientIP
			}
		}
	}

	// Check X-Real-IP header
	if xri := r.Header.Get(HeaderXRealIP); xri != "" {
		return strings.TrimSpace(xri)
	}

	return remoteIP
}

// extractIPFromAddr extracts the IP address from an address string (host:port or just host).
func extractIPFromAddr(addr string) string {
	// Try to split host and port
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// If there's no port, the whole string is the host
		return addr
	}
	return host
}
