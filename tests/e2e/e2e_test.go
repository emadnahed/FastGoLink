// Package e2e contains end-to-end tests for full HTTP → DB → response flows.
package e2e

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
	"github.com/gourl/gourl/internal/server"
	"github.com/gourl/gourl/pkg/logger"
)

// TestSetupVerification verifies the E2E test framework is working.
func TestSetupVerification(t *testing.T) {
	t.Run("e2e test framework is operational", func(t *testing.T) {
		assert.True(t, true, "e2e test framework should be working")
	})
}

// testServer creates and starts a test server, returning cleanup function.
func testServer(t *testing.T) (*server.Server, string, func()) {
	t.Helper()

	cfg := &config.Config{
		App: config.AppConfig{
			Env:      "test",
			LogLevel: "error",
		},
		Server: config.ServerConfig{
			Host:            "127.0.0.1",
			Port:            0, // Let OS assign port
			ReadTimeout:     5 * time.Second,
			WriteTimeout:    10 * time.Second,
			ShutdownTimeout: 5 * time.Second,
		},
	}

	var buf bytes.Buffer
	log := logger.New(&buf, "error")
	srv := server.New(cfg, log)

	// Start server
	go func() { _ = srv.Start() }()

	// Wait for server to be ready
	time.Sleep(100 * time.Millisecond)

	addr := srv.Addr()
	require.NotEmpty(t, addr, "server should have an address")

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}

	return srv, "http://" + addr, cleanup
}

// httpGet makes a GET request with context.
func httpGet(t *testing.T, url string) *http.Response {
	t.Helper()
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func TestE2E_HealthEndpoint(t *testing.T) {
	_, baseURL, cleanup := testServer(t)
	defer cleanup()

	t.Run("GET /health returns healthy status", func(t *testing.T) {
		resp := httpGet(t, baseURL+"/health")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

		var health handlers.HealthResponse
		err := json.NewDecoder(resp.Body).Decode(&health)
		require.NoError(t, err)

		assert.Equal(t, "healthy", health.Status)
		assert.NotEmpty(t, health.Timestamp)

		// Verify timestamp is valid RFC3339
		_, err = time.Parse(time.RFC3339, health.Timestamp)
		assert.NoError(t, err)
	})

	t.Run("health endpoint is idempotent", func(t *testing.T) {
		// Multiple requests should return same result
		for i := 0; i < 3; i++ {
			resp := httpGet(t, baseURL+"/health")
			resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		}
	})
}

func TestE2E_ReadyEndpoint(t *testing.T) {
	srv, baseURL, cleanup := testServer(t)
	defer cleanup()

	t.Run("GET /ready returns ready status when healthy", func(t *testing.T) {
		resp := httpGet(t, baseURL+"/ready")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

		var ready handlers.ReadyResponse
		err := json.NewDecoder(resp.Body).Decode(&ready)
		require.NoError(t, err)

		assert.Equal(t, "ready", ready.Status)
		assert.NotEmpty(t, ready.Timestamp)
	})

	t.Run("GET /ready returns 503 when not ready", func(t *testing.T) {
		// Mark server as not ready
		srv.HealthHandler().SetReady(false)

		resp := httpGet(t, baseURL+"/ready")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

		var ready handlers.ReadyResponse
		err := json.NewDecoder(resp.Body).Decode(&ready)
		require.NoError(t, err)

		assert.Equal(t, "not ready", ready.Status)

		// Restore ready state
		srv.HealthHandler().SetReady(true)
	})

	t.Run("ready endpoint reflects dependency health", func(t *testing.T) {
		dbHealthy := true
		srv.HealthHandler().AddCheck("database", func() bool {
			return dbHealthy
		})

		// Should be ready when dependency is healthy
		resp := httpGet(t, baseURL+"/ready")
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Should be not ready when dependency fails
		dbHealthy = false
		resp = httpGet(t, baseURL+"/ready")
		defer resp.Body.Close()
		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

		var ready handlers.ReadyResponse
		err := json.NewDecoder(resp.Body).Decode(&ready)
		require.NoError(t, err)

		assert.Equal(t, "not ready", ready.Status)
		assert.Equal(t, "fail", ready.Checks["database"])
	})
}

func TestE2E_ServerLifecycle(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			Env:      "test",
			LogLevel: "error",
		},
		Server: config.ServerConfig{
			Host:            "127.0.0.1",
			Port:            0,
			ReadTimeout:     5 * time.Second,
			WriteTimeout:    10 * time.Second,
			ShutdownTimeout: 5 * time.Second,
		},
	}

	var buf bytes.Buffer
	log := logger.New(&buf, "error")
	srv := server.New(cfg, log)

	t.Run("server starts and stops cleanly", func(t *testing.T) {
		// Start server
		go func() { _ = srv.Start() }()
		time.Sleep(100 * time.Millisecond)

		assert.True(t, srv.IsRunning())

		addr := srv.Addr()
		require.NotEmpty(t, addr)

		// Verify server responds
		resp := httpGet(t, "http://"+addr+"/health")
		resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		// Graceful shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := srv.Shutdown(ctx)
		assert.NoError(t, err)
		assert.False(t, srv.IsRunning())

		// Verify server no longer responds
		ctx2 := context.Background()
		req, _ := http.NewRequestWithContext(ctx2, http.MethodGet, "http://"+addr+"/health", nil)
		_, err = http.DefaultClient.Do(req)
		assert.Error(t, err)
	})
}

func TestE2E_ConcurrentRequests(t *testing.T) {
	_, baseURL, cleanup := testServer(t)
	defer cleanup()

	t.Run("handles concurrent requests", func(t *testing.T) {
		const numRequests = 50
		results := make(chan int, numRequests)

		for i := 0; i < numRequests; i++ {
			go func() {
				ctx := context.Background()
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/health", nil)
				if err != nil {
					results <- 0
					return
				}
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					results <- 0
					return
				}
				resp.Body.Close()
				results <- resp.StatusCode
			}()
		}

		successCount := 0
		for i := 0; i < numRequests; i++ {
			if <-results == http.StatusOK {
				successCount++
			}
		}

		assert.Equal(t, numRequests, successCount)
	})
}
