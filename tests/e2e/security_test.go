package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gourl/gourl/internal/config"
	"github.com/gourl/gourl/internal/handlers"
	"github.com/gourl/gourl/internal/idgen"
	"github.com/gourl/gourl/internal/security"
	"github.com/gourl/gourl/internal/server"
	"github.com/gourl/gourl/internal/services"
	"github.com/gourl/gourl/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2E_RequestIDHeader(t *testing.T) {
	srv, baseURL, cleanup := testServerWithRateLimitDisabled(t)
	defer cleanup()

	t.Run("generates request ID for all responses", func(t *testing.T) {
		resp := httpGet(t, baseURL+"/health")
		defer resp.Body.Close()

		requestID := resp.Header.Get("X-Request-ID")
		assert.NotEmpty(t, requestID, "X-Request-ID header should be set")
	})

	t.Run("preserves incoming request ID", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, baseURL+"/health", nil)
		require.NoError(t, err)
		req.Header.Set("X-Request-ID", "my-trace-12345")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, "my-trace-12345", resp.Header.Get("X-Request-ID"))
	})

	_ = srv // Keep srv reference
}

func TestE2E_RateLimiting(t *testing.T) {
	// Create server with low rate limit for testing
	srv, baseURL, cleanup := testServerWithRateLimit(t, 3, 10*time.Second)
	defer cleanup()

	t.Run("allows requests under limit", func(t *testing.T) {
		// Make 3 requests (the limit)
		for i := 0; i < 3; i++ {
			resp := httpGet(t, baseURL+"/health")
			resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		}
	})

	t.Run("returns 429 when over limit", func(t *testing.T) {
		// The 4th request should be rate limited
		resp := httpGet(t, baseURL+"/health")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
		assert.NotEmpty(t, resp.Header.Get("Retry-After"))
		assert.Equal(t, "0", resp.Header.Get("X-RateLimit-Remaining"))
	})

	_ = srv
}

func TestE2E_MaliciousURLRejection(t *testing.T) {
	srv, baseURL, cleanup := testServerWithSecurity(t)
	defer cleanup()

	testCases := []struct {
		name         string
		url          string
		expectedCode int
		expectedErr  string
	}{
		{
			name:         "blocks javascript scheme",
			url:          "javascript:alert('xss')",
			expectedCode: http.StatusBadRequest,
			expectedErr:  "DANGEROUS_URL",
		},
		{
			name:         "blocks data scheme",
			url:          "data:text/html,<script>alert('xss')</script>",
			expectedCode: http.StatusBadRequest,
			expectedErr:  "DANGEROUS_URL",
		},
		{
			name:         "blocks localhost",
			url:          "http://localhost/admin",
			expectedCode: http.StatusBadRequest,
			expectedErr:  "PRIVATE_IP_BLOCKED",
		},
		{
			name:         "blocks private IP 127.0.0.1",
			url:          "http://127.0.0.1/path",
			expectedCode: http.StatusBadRequest,
			expectedErr:  "PRIVATE_IP_BLOCKED",
		},
		{
			name:         "blocks private IP 192.168.x.x",
			url:          "http://192.168.1.1/internal",
			expectedCode: http.StatusBadRequest,
			expectedErr:  "PRIVATE_IP_BLOCKED",
		},
		{
			name:         "blocks private IP 10.x.x.x",
			url:          "http://10.0.0.1/secret",
			expectedCode: http.StatusBadRequest,
			expectedErr:  "PRIVATE_IP_BLOCKED",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			body := map[string]string{"url": tc.url}

			resp := httpPost(t, baseURL+"/api/v1/shorten", body)
			defer resp.Body.Close()

			assert.Equal(t, tc.expectedCode, resp.StatusCode)

			var errResp map[string]string
			err := json.NewDecoder(resp.Body).Decode(&errResp)
			require.NoError(t, err)
			assert.Equal(t, tc.expectedErr, errResp["code"])
		})
	}

	_ = srv
}

func TestE2E_ValidURLAccepted(t *testing.T) {
	srv, baseURL, cleanup := testServerWithSecurity(t)
	defer cleanup()

	validURLs := []string{
		"https://example.com",
		"https://example.com/path",
		"https://example.com/path?query=value",
		"http://example.com:8080/path",
	}

	for _, validURL := range validURLs {
		t.Run(validURL, func(t *testing.T) {
			body := map[string]string{"url": validURL}

			resp := httpPost(t, baseURL+"/api/v1/shorten", body)
			defer resp.Body.Close()

			assert.Equal(t, http.StatusCreated, resp.StatusCode, "valid URL should be accepted")
		})
	}

	_ = srv
}

func TestE2E_RateLimitHeaders(t *testing.T) {
	srv, baseURL, cleanup := testServerWithRateLimit(t, 10, time.Minute)
	defer cleanup()

	resp := httpGet(t, baseURL+"/health")
	defer resp.Body.Close()

	assert.Equal(t, "10", resp.Header.Get("X-RateLimit-Limit"))
	assert.NotEmpty(t, resp.Header.Get("X-RateLimit-Remaining"))

	_ = srv
}

// testServerWithRateLimitDisabled creates a test server with rate limiting disabled.
func testServerWithRateLimitDisabled(t *testing.T) (*server.Server, string, func()) {
	t.Helper()

	var buf bytes.Buffer
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:            "127.0.0.1",
			Port:            0,
			ReadTimeout:     5 * time.Second,
			WriteTimeout:    10 * time.Second,
			ShutdownTimeout: 5 * time.Second,
		},
		Rate: config.RateLimitConfig{
			Enabled:  false,
			Requests: 100,
			Window:   time.Minute,
		},
		Security: config.SecurityConfig{
			MaxURLLength:    2048,
			AllowPrivateIPs: true,
		},
	}

	log := logger.New(&buf, "error")
	srv := server.New(cfg, log)

	go func() {
		_ = srv.Start()
	}()

	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	addr := srv.Addr()
	baseURL := "http://" + addr

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}

	return srv, baseURL, cleanup
}

// testServerWithRateLimit creates a test server with specified rate limit.
func testServerWithRateLimit(t *testing.T, requests int, window time.Duration) (*server.Server, string, func()) {
	t.Helper()

	var buf bytes.Buffer
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:            "127.0.0.1",
			Port:            0,
			ReadTimeout:     5 * time.Second,
			WriteTimeout:    10 * time.Second,
			ShutdownTimeout: 5 * time.Second,
		},
		Rate: config.RateLimitConfig{
			Enabled:  true,
			Requests: requests,
			Window:   window,
		},
		Security: config.SecurityConfig{
			MaxURLLength:    2048,
			AllowPrivateIPs: true,
		},
	}

	log := logger.New(&buf, "error")
	srv := server.New(cfg, log)

	go func() {
		_ = srv.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	addr := srv.Addr()
	baseURL := "http://" + addr

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}

	return srv, baseURL, cleanup
}

// testServerWithSecurity creates a test server with URL service and security enabled.
func testServerWithSecurity(t *testing.T) (*server.Server, string, func()) {
	t.Helper()

	var buf bytes.Buffer
	cfg := &config.Config{
		Server: config.ServerConfig{
			Host:            "127.0.0.1",
			Port:            0,
			ReadTimeout:     5 * time.Second,
			WriteTimeout:    10 * time.Second,
			ShutdownTimeout: 5 * time.Second,
		},
		URL: config.URLConfig{
			BaseURL:      "http://localhost:8080",
			ShortCodeLen: 7,
		},
		Rate: config.RateLimitConfig{
			Enabled:  false,
			Requests: 100,
			Window:   time.Minute,
		},
		Security: config.SecurityConfig{
			MaxURLLength:    2048,
			AllowPrivateIPs: false, // Block private IPs
		},
	}

	log := logger.New(&buf, "error")
	srv := server.New(cfg, log)

	// Set up URL service with in-memory repository
	repo := NewInMemoryURLRepository()
	gen := idgen.NewRandomGenerator(cfg.URL.ShortCodeLen)
	sanitizer := security.NewSanitizer(security.Config{
		MaxURLLength:    cfg.Security.MaxURLLength,
		AllowPrivateIPs: cfg.Security.AllowPrivateIPs,
	})
	urlService := services.NewURLServiceWithSanitizer(repo, gen, sanitizer, cfg.URL.BaseURL)
	urlHandler := handlers.NewURLHandler(urlService)
	srv.SetURLHandler(urlHandler)

	go func() {
		_ = srv.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	addr := srv.Addr()
	baseURL := "http://" + addr

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}

	return srv, baseURL, cleanup
}

// httpGet helper using httptest pattern
func httpGetLocal(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}
