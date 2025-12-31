// Package main is the entry point for the GoURL API server.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/gourl/gourl/internal/cache"
	"github.com/gourl/gourl/internal/config"
	"github.com/gourl/gourl/internal/database"
	"github.com/gourl/gourl/internal/handlers"
	"github.com/gourl/gourl/internal/idgen"
	"github.com/gourl/gourl/internal/repository"
	"github.com/gourl/gourl/internal/server"
	"github.com/gourl/gourl/internal/services"
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

	// Connect to database if configured
	var dbRouter *database.ShardRouter
	if cfg.DatabaseEnabled() {
		log.Info("connecting to database",
			"host", cfg.Database.Host,
			"port", cfg.Database.Port,
			"database", cfg.Database.DBName,
		)

		ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ReadTimeout)
		dbRouter, err = database.SingleShardRouter(ctx, &cfg.Database)
		cancel()

		if err != nil {
			log.Warn("database connection failed, continuing without database",
				"error", err.Error(),
			)
		} else {
			log.Info("database connected successfully")

			// Add database health check
			srv.HealthHandler().AddCheck("database", func() bool {
				ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ReadTimeout)
				defer cancel()
				return dbRouter.HealthCheck(ctx) == nil
			})

			defer dbRouter.Close()
		}
	} else {
		log.Info("database not configured, skipping connection")
	}

	// Connect to Redis if configured
	var redisCache *cache.RedisCache
	if cfg.RedisEnabled() {
		log.Info("connecting to Redis",
			"host", cfg.Redis.Host,
			"port", cfg.Redis.Port,
		)

		ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ReadTimeout)
		redisCache, err = cache.NewRedisCache(ctx, &cfg.Redis)
		cancel()

		if err != nil {
			log.Warn("Redis connection failed, continuing without cache",
				"error", err.Error(),
			)
		} else {
			log.Info("Redis connected successfully")

			// Add Redis health check
			srv.HealthHandler().AddCheck("redis", func() bool {
				ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ReadTimeout)
				defer cancel()
				return redisCache.Ping(ctx) == nil
			})

			defer func() {
				if err := redisCache.Close(); err != nil {
					log.Error("failed to close Redis connection", "error", err.Error())
				}
			}()
		}
	} else {
		log.Info("Redis not configured, skipping connection")
	}

	// Wire up the URL repository chain
	if dbRouter != nil {
		// Get the database pool (using shard 0 for single-shard setup)
		dbPool := dbRouter.GetShard("")
		baseRepo := repository.NewPostgresURLRepository(dbPool)

		var urlRepo repository.URLRepository
		if redisCache != nil {
			// Create cached repository with Redis
			log.Info("enabling repository caching",
				"key_prefix", cfg.Redis.KeyPrefix,
				"cache_ttl", cfg.Redis.CacheTTL.String(),
			)
			urlCache := cache.NewURLCache(redisCache, cfg.Redis.KeyPrefix, cfg.Redis.CacheTTL)
			urlRepo = repository.NewCachedURLRepository(baseRepo, urlCache, cfg.Redis.CacheTTL)
		} else {
			// Use base repository without caching
			urlRepo = baseRepo
		}

		srv.SetURLRepository(urlRepo)
		log.Info("URL repository configured")

		// Create ID generator with collision detection
		baseGen := idgen.NewRandomGenerator(cfg.URL.ShortCodeLen)
		collisionGen := idgen.NewCollisionAwareGenerator(baseGen, urlRepo, cfg.URL.IDGenMaxRetries)

		// Create URL service and handler
		urlService := services.NewURLService(urlRepo, collisionGen, cfg.URL.BaseURL)
		urlHandler := handlers.NewURLHandler(urlService)
		srv.SetURLHandler(urlHandler)
		log.Info("URL shortening API configured",
			"base_url", cfg.URL.BaseURL,
			"code_length", cfg.URL.ShortCodeLen,
		)
	}

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
