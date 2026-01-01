// Package server provides the HTTP server implementation.
package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/gourl/gourl/internal/config"
	"github.com/gourl/gourl/internal/handlers"
	"github.com/gourl/gourl/internal/metrics"
	"github.com/gourl/gourl/internal/middleware"
	"github.com/gourl/gourl/internal/ratelimit"
	"github.com/gourl/gourl/internal/repository"
	"github.com/gourl/gourl/pkg/logger"
)

// Server represents the HTTP server.
type Server struct {
	cfg              *config.Config
	log              *logger.Logger
	httpServer       *http.Server
	healthHandler    *handlers.HealthHandler
	urlHandler       *handlers.URLHandler
	redirectHandler  *handlers.RedirectHandler
	analyticsHandler *handlers.AnalyticsHandler
	docsHandler      *handlers.DocsHandler
	urlRepo          repository.URLRepository
	rateLimiter      ratelimit.Limiter
	listener         net.Listener
	running          bool
	mu               sync.RWMutex
}

// New creates a new Server instance.
func New(cfg *config.Config, log *logger.Logger) *Server {
	s := &Server{
		cfg:           cfg,
		log:           log,
		healthHandler: handlers.NewHealthHandler(),
		docsHandler:   handlers.NewDocsHandler(cfg.URL.BaseURL, ""),
	}

	// Create HTTP server
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	// Build middleware chain
	handler := s.buildMiddlewareChain(mux)

	s.httpServer = &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      handler,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	return s
}

// buildMiddlewareChain creates the middleware chain for the server.
func (s *Server) buildMiddlewareChain(handler http.Handler) http.Handler {
	// Start with metrics and request ID middleware (always enabled)
	chain := middleware.New(
		middleware.Metrics(),
		middleware.RequestID(),
		middleware.ClientIP(s.cfg.Rate.TrustProxy, nil),
	)

	// Add rate limiting if enabled
	if s.cfg.Rate.Enabled {
		s.rateLimiter = ratelimit.NewMemoryLimiter(ratelimit.Config{
			Requests: s.cfg.Rate.Requests,
			Window:   s.cfg.Rate.Window,
		})

		chain = chain.Append(middleware.RateLimit(s.rateLimiter, middleware.RateLimitConfig{
			TrustProxy:   s.cfg.Rate.TrustProxy,
			APIKeyHeader: s.cfg.Rate.APIKeyHeader,
		}))

		s.log.Info("rate limiting enabled",
			"requests", s.cfg.Rate.Requests,
			"window", s.cfg.Rate.Window.String(),
		)
	}

	return chain.Then(handler)
}

// registerRoutes sets up the HTTP routes.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Health check routes (GET only)
	mux.HandleFunc("GET /health", s.healthHandler.Health)
	mux.HandleFunc("GET /ready", s.healthHandler.Ready)

	// Metrics endpoint for Prometheus
	mux.Handle("GET /metrics", metrics.Handler())

	// API Documentation routes (Scalar, ReDoc, Swagger UI)
	mux.HandleFunc("GET /docs", s.docsHandler.ScalarUI)
	mux.HandleFunc("GET /docs/", s.docsHandler.ScalarUI)
	mux.HandleFunc("GET /docs/openapi.yaml", s.docsHandler.OpenAPISpec)
	mux.HandleFunc("GET /docs/redoc", s.docsHandler.Redoc)
	mux.HandleFunc("GET /docs/swagger", s.docsHandler.SwaggerUI)

	// API v1 routes - URL shortening
	mux.HandleFunc("POST /api/v1/shorten", s.handleShorten)
	mux.HandleFunc("GET /api/v1/urls/", s.handleGetURL)
	mux.HandleFunc("DELETE /api/v1/urls/", s.handleDeleteURL)

	// Analytics routes
	mux.HandleFunc("GET /api/v1/analytics/", s.handleAnalytics)

	// Redirect route - GET /{code} for URL redirects
	// Note: More specific routes like /health, /ready are matched first by Go's ServeMux
	mux.HandleFunc("GET /{code}", s.handleRedirect)
}

// handleShorten routes to the URL handler for shortening.
func (s *Server) handleShorten(w http.ResponseWriter, r *http.Request) {
	if s.urlHandler == nil {
		http.Error(w, "URL service not configured", http.StatusServiceUnavailable)
		return
	}
	s.urlHandler.Shorten(w, r)
}

// handleGetURL routes to the URL handler for getting URL info.
func (s *Server) handleGetURL(w http.ResponseWriter, r *http.Request) {
	if s.urlHandler == nil {
		http.Error(w, "URL service not configured", http.StatusServiceUnavailable)
		return
	}
	shortCode := extractShortCode(r.URL.Path, "/api/v1/urls/")
	if shortCode == "" || strings.Contains(shortCode, "/") {
		http.Error(w, "invalid short code format", http.StatusBadRequest)
		return
	}
	s.urlHandler.GetURL(w, r, shortCode)
}

// handleDeleteURL routes to the URL handler for deleting URLs.
func (s *Server) handleDeleteURL(w http.ResponseWriter, r *http.Request) {
	if s.urlHandler == nil {
		http.Error(w, "URL service not configured", http.StatusServiceUnavailable)
		return
	}
	shortCode := extractShortCode(r.URL.Path, "/api/v1/urls/")
	if shortCode == "" || strings.Contains(shortCode, "/") {
		http.Error(w, "invalid short code format", http.StatusBadRequest)
		return
	}
	s.urlHandler.DeleteURL(w, r, shortCode)
}

// handleRedirect routes to the redirect handler for URL redirects.
func (s *Server) handleRedirect(w http.ResponseWriter, r *http.Request) {
	if s.redirectHandler == nil {
		http.Error(w, "Redirect service not configured", http.StatusServiceUnavailable)
		return
	}
	shortCode := r.PathValue("code")
	if shortCode == "" {
		http.Error(w, "invalid short code", http.StatusBadRequest)
		return
	}
	s.redirectHandler.Redirect(w, r, shortCode)
}

// handleAnalytics routes to the analytics handler for stats.
func (s *Server) handleAnalytics(w http.ResponseWriter, r *http.Request) {
	if s.analyticsHandler == nil {
		http.Error(w, "Analytics service not configured", http.StatusServiceUnavailable)
		return
	}
	shortCode := extractShortCode(r.URL.Path, "/api/v1/analytics/")
	if shortCode == "" || strings.Contains(shortCode, "/") {
		http.Error(w, "invalid short code format", http.StatusBadRequest)
		return
	}
	s.analyticsHandler.GetStats(w, r, shortCode)
}

// extractShortCode extracts the short code from the URL path.
func extractShortCode(path, prefix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	return strings.TrimPrefix(path, prefix)
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	addr := s.cfg.Server.Address()

	// Create listener first to get the actual address (important when port is 0)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}

	s.mu.Lock()
	s.listener = listener
	s.running = true
	s.mu.Unlock()

	actualAddr := listener.Addr().String()
	s.log.Info("server starting", "address", actualAddr)

	// Start serving
	err = s.httpServer.Serve(listener)
	if err != nil && err != http.ErrServerClosed {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

// Shutdown gracefully shuts down the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.log.Info("server shutting down")

	// Mark as not ready during shutdown
	s.healthHandler.SetReady(false)

	err := s.httpServer.Shutdown(ctx)

	// Close rate limiter if it exists
	if s.rateLimiter != nil {
		if closeErr := s.rateLimiter.Close(); closeErr != nil {
			s.log.Error("failed to close rate limiter", "error", closeErr.Error())
		}
	}

	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	if err != nil {
		s.log.Error("shutdown error", "error", err.Error())
		return err
	}

	s.log.Info("server stopped")
	return nil
}

// IsRunning returns whether the server is running.
func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// Addr returns the server's address.
func (s *Server) Addr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.listener != nil {
		return s.listener.Addr().String()
	}
	return ""
}

// HealthHandler returns the health handler.
func (s *Server) HealthHandler() *handlers.HealthHandler {
	return s.healthHandler
}

// SetURLRepository sets the URL repository for the server.
func (s *Server) SetURLRepository(repo repository.URLRepository) {
	s.urlRepo = repo
}

// URLRepository returns the URL repository.
func (s *Server) URLRepository() repository.URLRepository {
	return s.urlRepo
}

// SetURLHandler sets the URL handler for the server.
func (s *Server) SetURLHandler(h *handlers.URLHandler) {
	s.urlHandler = h
}

// URLHandler returns the URL handler.
func (s *Server) URLHandler() *handlers.URLHandler {
	return s.urlHandler
}

// SetRedirectHandler sets the redirect handler for the server.
func (s *Server) SetRedirectHandler(h *handlers.RedirectHandler) {
	s.redirectHandler = h
}

// RedirectHandler returns the redirect handler.
func (s *Server) RedirectHandler() *handlers.RedirectHandler {
	return s.redirectHandler
}

// SetAnalyticsHandler sets the analytics handler for the server.
func (s *Server) SetAnalyticsHandler(h *handlers.AnalyticsHandler) {
	s.analyticsHandler = h
}

// AnalyticsHandler returns the analytics handler.
func (s *Server) AnalyticsHandler() *handlers.AnalyticsHandler {
	return s.analyticsHandler
}
