// Package main is the entry point for the GoURL API server.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gourl/gourl/internal/config"
	"github.com/gourl/gourl/internal/server"
	"github.com/gourl/gourl/pkg/logger"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Create logger
	log := logger.New(os.Stdout, cfg.App.LogLevel)
	log = log.With("service", "gourl", "env", cfg.App.Env)

	log.Info("starting server",
		"host", cfg.Server.Host,
		"port", cfg.Server.Port,
	)

	// Create server
	srv := server.New(cfg, log)

	// Handle graceful shutdown
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	// Wait for shutdown signal or error
	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case sig := <-shutdown:
		log.Info("shutdown signal received", "signal", sig.String())

		// Create shutdown context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			return fmt.Errorf("graceful shutdown failed: %w", err)
		}

		log.Info("server stopped gracefully")
	}

	return nil
}
