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

	"github.com/gourl/gourl/internal/analytics"
	"github.com/gourl/gourl/internal/config"
	"github.com/gourl/gourl/internal/handlers"
	"github.com/gourl/gourl/internal/idgen"
	"github.com/gourl/gourl/internal/server"
	"github.com/gourl/gourl/internal/services"
	"github.com/gourl/gourl/pkg/logger"
)

func TestE2E_AnalyticsEndpoint(t *testing.T) {
	srv, baseURL, clickCounter, cleanup := testServerWithAnalytics(t)
	defer cleanup()

	// First create a URL
	body := map[string]string{"url": "https://example.com/analytics-test"}
	resp := httpPost(t, baseURL+"/api/v1/shorten", body)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&createResp)
	resp.Body.Close()
	require.NoError(t, err)

	shortCode := createResp["short_code"].(string)

	t.Run("returns stats for existing URL", func(t *testing.T) {
		resp := httpGet(t, baseURL+"/api/v1/analytics/"+shortCode)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var stats map[string]interface{}
		err := json.NewDecoder(resp.Body).Decode(&stats)
		require.NoError(t, err)

		assert.Equal(t, shortCode, stats["short_code"])
		assert.Equal(t, float64(0), stats["click_count"])
	})

	t.Run("returns 404 for non-existent URL", func(t *testing.T) {
		resp := httpGet(t, baseURL+"/api/v1/analytics/nonexistent")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("click counter records clicks", func(t *testing.T) {
		// Make a redirect request to record a click
		client := &http.Client{
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse // Don't follow redirects
			},
		}

		req, err := http.NewRequest(http.MethodGet, baseURL+"/"+shortCode, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()

		assert.Equal(t, http.StatusFound, resp.StatusCode)

		// Wait for async processing
		time.Sleep(50 * time.Millisecond)

		// Check pending stats
		pendingStats := clickCounter.GetPendingStats()
		assert.Equal(t, int64(1), pendingStats[shortCode], "should have 1 pending click")
	})

	_ = srv
}

func TestE2E_ClickCounterBatching(t *testing.T) {
	srv, baseURL, clickCounter, cleanup := testServerWithAnalytics(t)
	defer cleanup()

	// Create a URL
	body := map[string]string{"url": "https://example.com/batch-test"}
	resp := httpPost(t, baseURL+"/api/v1/shorten", body)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var createResp map[string]interface{}
	err := json.NewDecoder(resp.Body).Decode(&createResp)
	resp.Body.Close()
	require.NoError(t, err)

	shortCode := createResp["short_code"].(string)

	// Record multiple clicks
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	for i := 0; i < 5; i++ {
		req, err := http.NewRequest(http.MethodGet, baseURL+"/"+shortCode, nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
	}

	// Wait for async processing
	time.Sleep(50 * time.Millisecond)

	// Check accumulated pending stats
	pendingStats := clickCounter.GetPendingStats()
	assert.Equal(t, int64(5), pendingStats[shortCode], "should have 5 pending clicks")

	_ = srv
}

// testServerWithAnalytics creates a test server with analytics configured.
func testServerWithAnalytics(t *testing.T) (*server.Server, string, *analytics.ClickCounter, func()) {
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
			AllowPrivateIPs: true,
		},
	}

	log := logger.New(&buf, "error")
	srv := server.New(cfg, log)

	// Set up repository
	repo := NewInMemoryURLRepository()
	gen := idgen.NewRandomGenerator(cfg.URL.ShortCodeLen)
	urlService := services.NewURLService(repo, gen, cfg.URL.BaseURL)
	urlHandler := handlers.NewURLHandler(urlService)
	srv.SetURLHandler(urlHandler)

	// Set up analytics
	flusher := analytics.NewRepositoryFlusher(repo, log)
	clickCounter := analytics.NewClickCounter(analytics.Config{
		FlushInterval: 100 * time.Millisecond, // Short interval for testing
		BatchSize:     100,
		ChannelBuffer: 1000,
	}, flusher)

	// Set up redirect with analytics
	redirectService := services.NewRedirectServiceWithAnalytics(repo, clickCounter)
	redirectHandler := handlers.NewRedirectHandler(redirectService)
	srv.SetRedirectHandler(redirectHandler)

	// Set up analytics endpoint
	analyticsService := services.NewAnalyticsServiceWithPendingStats(repo, clickCounter)
	analyticsHandler := handlers.NewAnalyticsHandler(analyticsService)
	srv.SetAnalyticsHandler(analyticsHandler)

	go func() {
		_ = srv.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	addr := srv.Addr()
	baseURL := "http://" + addr

	cleanup := func() {
		clickCounter.Stop()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}

	return srv, baseURL, clickCounter, cleanup
}
