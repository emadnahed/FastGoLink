// Package middleware contains HTTP middleware components.
package middleware

import (
	"context"
	"net/http"
)

// Middleware wraps an http.Handler with additional behavior.
type Middleware func(http.Handler) http.Handler

// contextKey is the type for context keys used by middleware.
type contextKey string

const (
	// RequestIDKey is the context key for request ID.
	RequestIDKey contextKey = "request_id"
	// ClientIPKey is the context key for client IP.
	ClientIPKey contextKey = "client_ip"
)

// GetRequestID retrieves the request ID from context.
func GetRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(RequestIDKey).(string); ok {
		return id
	}
	return ""
}

// GetClientIP retrieves the client IP from context.
func GetClientIP(ctx context.Context) string {
	if ip, ok := ctx.Value(ClientIPKey).(string); ok {
		return ip
	}
	return ""
}

// Chain holds a sequence of middlewares to be applied to handlers.
type Chain struct {
	middlewares []Middleware
}

// New creates a new middleware chain with the given middlewares.
func New(middlewares ...Middleware) *Chain {
	return &Chain{
		middlewares: append([]Middleware{}, middlewares...),
	}
}

// Then applies the middleware chain to the given handler.
// Middlewares are applied in order: first middleware wraps the entire chain.
func (c *Chain) Then(h http.Handler) http.Handler {
	if h == nil {
		h = http.DefaultServeMux
	}

	for i := len(c.middlewares) - 1; i >= 0; i-- {
		h = c.middlewares[i](h)
	}

	return h
}

// ThenFunc applies the middleware chain to the given handler function.
func (c *Chain) ThenFunc(fn http.HandlerFunc) http.Handler {
	return c.Then(fn)
}

// Append creates a new chain with the given middleware appended.
// The original chain is not modified.
func (c *Chain) Append(middlewares ...Middleware) *Chain {
	newMiddlewares := make([]Middleware, 0, len(c.middlewares)+len(middlewares))
	newMiddlewares = append(newMiddlewares, c.middlewares...)
	newMiddlewares = append(newMiddlewares, middlewares...)
	return &Chain{middlewares: newMiddlewares}
}

// Extend is an alias for Append for better readability when adding multiple middlewares.
func (c *Chain) Extend(middlewares ...Middleware) *Chain {
	return c.Append(middlewares...)
}
