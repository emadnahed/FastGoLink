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

	"github.com/emadnahed/FastGoLink/internal/config"
	"github.com/emadnahed/FastGoLink/internal/handlers"
	"github.com/emadnahed/FastGoLink/pkg/logger"
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

func TestServer_SetterGetters(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "error")
	cfg := testConfig()

	srv := New(cfg, log)

	// Test URL handler setter/getter
	t.Run("URL handler", func(t *testing.T) {
		assert.Nil(t, srv.URLHandler())

		urlHandler := &handlers.URLHandler{}
		srv.SetURLHandler(urlHandler)

		assert.Equal(t, urlHandler, srv.URLHandler())
	})

	// Test redirect handler setter/getter
	t.Run("redirect handler", func(t *testing.T) {
		assert.Nil(t, srv.RedirectHandler())

		redirectHandler := &handlers.RedirectHandler{}
		srv.SetRedirectHandler(redirectHandler)

		assert.Equal(t, redirectHandler, srv.RedirectHandler())
	})

	// Test analytics handler setter/getter
	t.Run("analytics handler", func(t *testing.T) {
		assert.Nil(t, srv.AnalyticsHandler())

		analyticsHandler := &handlers.AnalyticsHandler{}
		srv.SetAnalyticsHandler(analyticsHandler)

		assert.Equal(t, analyticsHandler, srv.AnalyticsHandler())
	})

	// Test URL repository setter/getter
	t.Run("URL repository", func(t *testing.T) {
		assert.Nil(t, srv.URLRepository())

		// We can test with nil since we're just testing the setter/getter
		srv.SetURLRepository(nil)
		assert.Nil(t, srv.URLRepository())
	})
}

func TestServer_HandleShorten_NoHandler(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "error")
	cfg := testConfig()

	srv := New(cfg, log)

	// Start server (without URL handler set)
	go func() { _ = srv.Start() }()
	defer func() { _ = srv.Shutdown(context.Background()) }()
	time.Sleep(100 * time.Millisecond)

	addr := srv.Addr()

	// Make request to /api/v1/shorten without handler configured
	ctx := context.Background()
	body := bytes.NewBufferString(`{"url":"https://example.com"}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+"/api/v1/shorten", body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestServer_HandleGetURL_NoHandler(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "error")
	cfg := testConfig()

	srv := New(cfg, log)

	go func() { _ = srv.Start() }()
	defer func() { _ = srv.Shutdown(context.Background()) }()
	time.Sleep(100 * time.Millisecond)

	addr := srv.Addr()

	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/api/v1/urls/abc123", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestServer_HandleDeleteURL_NoHandler(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "error")
	cfg := testConfig()

	srv := New(cfg, log)

	go func() { _ = srv.Start() }()
	defer func() { _ = srv.Shutdown(context.Background()) }()
	time.Sleep(100 * time.Millisecond)

	addr := srv.Addr()

	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, "http://"+addr+"/api/v1/urls/abc123", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestServer_HandleRedirect_NoHandler(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "error")
	cfg := testConfig()

	srv := New(cfg, log)

	go func() { _ = srv.Start() }()
	defer func() { _ = srv.Shutdown(context.Background()) }()
	time.Sleep(100 * time.Millisecond)

	addr := srv.Addr()

	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/abc123", nil)
	require.NoError(t, err)

	// Don't follow redirects
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestServer_HandleAnalytics_NoHandler(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "error")
	cfg := testConfig()

	srv := New(cfg, log)

	go func() { _ = srv.Start() }()
	defer func() { _ = srv.Shutdown(context.Background()) }()
	time.Sleep(100 * time.Millisecond)

	addr := srv.Addr()

	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/api/v1/analytics/abc123", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestServer_HandleGetURL_InvalidShortCode(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "error")
	cfg := testConfig()

	srv := New(cfg, log)
	srv.SetURLHandler(&handlers.URLHandler{})

	go func() { _ = srv.Start() }()
	defer func() { _ = srv.Shutdown(context.Background()) }()
	time.Sleep(100 * time.Millisecond)

	addr := srv.Addr()

	tests := []struct {
		name string
		path string
	}{
		{"empty short code", "/api/v1/urls/"},
		{"short code with slash", "/api/v1/urls/abc/def"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+tt.path, nil)
			require.NoError(t, err)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	}
}

func TestServer_HandleDeleteURL_InvalidShortCode(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "error")
	cfg := testConfig()

	srv := New(cfg, log)
	srv.SetURLHandler(&handlers.URLHandler{})

	go func() { _ = srv.Start() }()
	defer func() { _ = srv.Shutdown(context.Background()) }()
	time.Sleep(100 * time.Millisecond)

	addr := srv.Addr()

	tests := []struct {
		name string
		path string
	}{
		{"empty short code", "/api/v1/urls/"},
		{"short code with slash", "/api/v1/urls/abc/def"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			req, err := http.NewRequestWithContext(ctx, http.MethodDelete, "http://"+addr+tt.path, nil)
			require.NoError(t, err)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	}
}

func TestServer_HandleAnalytics_InvalidShortCode(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "error")
	cfg := testConfig()

	srv := New(cfg, log)
	srv.SetAnalyticsHandler(&handlers.AnalyticsHandler{})

	go func() { _ = srv.Start() }()
	defer func() { _ = srv.Shutdown(context.Background()) }()
	time.Sleep(100 * time.Millisecond)

	addr := srv.Addr()

	tests := []struct {
		name string
		path string
	}{
		{"empty short code", "/api/v1/analytics/"},
		{"short code with slash", "/api/v1/analytics/abc/def"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+tt.path, nil)
			require.NoError(t, err)

			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	}
}

func TestExtractShortCode(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		prefix   string
		expected string
	}{
		{"valid path", "/api/v1/urls/abc123", "/api/v1/urls/", "abc123"},
		{"empty after prefix", "/api/v1/urls/", "/api/v1/urls/", ""},
		{"wrong prefix", "/api/v2/urls/abc123", "/api/v1/urls/", ""},
		{"no prefix match", "/other/path", "/api/v1/urls/", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractShortCode(tt.path, tt.prefix)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestServer_WithRateLimiting(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "error")
	cfg := testConfig()
	cfg.Rate.Enabled = true
	cfg.Rate.Requests = 100
	cfg.Rate.Window = time.Minute

	srv := New(cfg, log)

	go func() { _ = srv.Start() }()
	defer func() { _ = srv.Shutdown(context.Background()) }()
	time.Sleep(100 * time.Millisecond)

	addr := srv.Addr()

	// Make a request and check for rate limit headers
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/health", nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	// Rate limit headers should be present
	assert.NotEmpty(t, resp.Header.Get("X-RateLimit-Limit"))
	assert.NotEmpty(t, resp.Header.Get("X-RateLimit-Remaining"))
}

func TestServer_Addr_NotRunning(t *testing.T) {
	var buf bytes.Buffer
	log := logger.New(&buf, "error")
	cfg := testConfig()

	srv := New(cfg, log)

	// Server not started yet, Addr should return empty string
	assert.Empty(t, srv.Addr())
}

