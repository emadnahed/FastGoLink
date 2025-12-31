package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gourl/gourl/internal/config"
	"github.com/gourl/gourl/internal/handlers"
	"github.com/gourl/gourl/pkg/logger"
)

func testConfig() *config.Config {
	return &config.Config{
		App: config.AppConfig{
			Env:      "test",
			LogLevel: "error",
		},
		Server: config.ServerConfig{
			Host:            "127.0.0.1",
			Port:            0, // Let the OS assign a port
			ReadTimeout:     5 * time.Second,
			WriteTimeout:    10 * time.Second,
			ShutdownTimeout: 5 * time.Second,
		},
	}
}

func TestNewServer(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "error")
	cfg := testConfig()

	srv := New(cfg, log)

	assert.NotNil(t, srv)
	assert.NotNil(t, srv.HealthHandler())
}

func TestServer_StartAndShutdown(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "error")
	cfg := testConfig()

	srv := New(cfg, log)

	// Start server in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start()
	}()

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Server should be running
	assert.True(t, srv.IsRunning())

	// Shutdown the server
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := srv.Shutdown(ctx)
	assert.NoError(t, err)

	// Server should no longer be running
	assert.False(t, srv.IsRunning())
}

func TestServer_HealthEndpoint(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "error")
	cfg := testConfig()

	srv := New(cfg, log)

	// Start server in background
	go func() { _ = srv.Start() }()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	// Get the actual address
	addr := srv.Addr()
	require.NotEmpty(t, addr)

	// Make request to /health
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/health", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var health handlers.HealthResponse
	err = json.NewDecoder(resp.Body).Decode(&health)
	require.NoError(t, err)

	assert.Equal(t, "healthy", health.Status)
}

func TestServer_ReadyEndpoint(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "error")
	cfg := testConfig()

	srv := New(cfg, log)

	// Start server in background
	go func() { _ = srv.Start() }()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	addr := srv.Addr()

	// Make request to /ready
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/ready", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var ready handlers.ReadyResponse
	err = json.NewDecoder(resp.Body).Decode(&ready)
	require.NoError(t, err)

	assert.Equal(t, "ready", ready.Status)
}

func TestServer_ReadyEndpoint_NotReady(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "error")
	cfg := testConfig()

	srv := New(cfg, log)
	srv.HealthHandler().SetReady(false)

	// Start server in background
	go func() { _ = srv.Start() }()
	defer func() { _ = srv.Shutdown(context.Background()) }()

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	addr := srv.Addr()

	// Make request to /ready
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/ready", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestServer_GracefulShutdown(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "error")
	cfg := testConfig()

	srv := New(cfg, log)

	// Start server
	go func() { _ = srv.Start() }()

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)
	require.True(t, srv.IsRunning())

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := srv.Shutdown(ctx)
	assert.NoError(t, err)
	assert.False(t, srv.IsRunning())
}

func TestServer_ShutdownTimeout(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "error")
	cfg := testConfig()

	srv := New(cfg, log)

	// Start server
	go func() { _ = srv.Start() }()
	time.Sleep(100 * time.Millisecond)

	// Shutdown with very short timeout (but should still work since no active connections)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Even with a short timeout, shutdown should succeed if there are no active connections
	err := srv.Shutdown(ctx)
	// May or may not error depending on timing, but server should be stopped
	_ = err

	// Give it a moment to fully stop
	time.Sleep(50 * time.Millisecond)
	assert.False(t, srv.IsRunning())
}
