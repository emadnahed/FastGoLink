// Package server provides the HTTP server implementation.
package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/gourl/gourl/internal/config"
	"github.com/gourl/gourl/internal/handlers"
	"github.com/gourl/gourl/internal/repository"
	"github.com/gourl/gourl/pkg/logger"
)

// Server represents the HTTP server.
type Server struct {
	cfg           *config.Config
	log           *logger.Logger
	httpServer    *http.Server
	healthHandler *handlers.HealthHandler
	urlRepo       repository.URLRepository
	listener      net.Listener
	running       bool
	mu            sync.RWMutex
}

// New creates a new Server instance.
func New(cfg *config.Config, log *logger.Logger) *Server {
	s := &Server{
		cfg:           cfg,
		log:           log,
		healthHandler: handlers.NewHealthHandler(),
	}

	// Create HTTP server
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	s.httpServer = &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      mux,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	return s
}

// registerRoutes sets up the HTTP routes.
func (s *Server) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", s.healthHandler.Health)
	mux.HandleFunc("/ready", s.healthHandler.Ready)
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
