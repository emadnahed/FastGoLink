// Package e2e contains end-to-end tests for full HTTP → DB → response flows.
package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/emadnahed/FastGoLink/internal/config"
	"github.com/emadnahed/FastGoLink/internal/handlers"
	"github.com/emadnahed/FastGoLink/internal/idgen"
	"github.com/emadnahed/FastGoLink/internal/models"
	"github.com/emadnahed/FastGoLink/internal/server"
	"github.com/emadnahed/FastGoLink/internal/services"
	"github.com/emadnahed/FastGoLink/pkg/logger"
)

// InMemoryURLRepository implements repository.URLRepository for testing.
type InMemoryURLRepository struct {
	mu   sync.RWMutex
	urls map[string]*models.URL
	seq  int64
}

func NewInMemoryURLRepository() *InMemoryURLRepository {
	return &InMemoryURLRepository{
		urls: make(map[string]*models.URL),
	}
}

func (r *InMemoryURLRepository) Create(ctx context.Context, create *models.URLCreate) (*models.URL, error) {
	if err := create.Validate(); err != nil {
		return nil, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check for duplicate
	if _, exists := r.urls[create.ShortCode]; exists {
		return nil, errors.New("duplicate short code")
	}

	r.seq++
	url := &models.URL{
		ID:          r.seq,
		ShortCode:   create.ShortCode,
		OriginalURL: create.OriginalURL,
		CreatedAt:   time.Now(),
		ExpiresAt:   create.ExpiresAt,
		ClickCount:  0,
	}
	r.urls[create.ShortCode] = url
	return url, nil
}

func (r *InMemoryURLRepository) GetByShortCode(ctx context.Context, shortCode string) (*models.URL, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	url, exists := r.urls[shortCode]
	if !exists {
		return nil, models.ErrURLNotFound
	}
	return url, nil
}

func (r *InMemoryURLRepository) GetByID(ctx context.Context, id int64) (*models.URL, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, url := range r.urls {
		if url.ID == id {
			return url, nil
		}
	}
	return nil, models.ErrURLNotFound
}

func (r *InMemoryURLRepository) Delete(ctx context.Context, shortCode string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.urls[shortCode]; !exists {
		return models.ErrURLNotFound
	}
	delete(r.urls, shortCode)
	return nil
}

func (r *InMemoryURLRepository) IncrementClickCount(ctx context.Context, shortCode string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	url, exists := r.urls[shortCode]
	if !exists {
		return models.ErrURLNotFound
	}
	url.ClickCount++
	return nil
}

func (r *InMemoryURLRepository) BatchIncrementClickCounts(ctx context.Context, counts map[string]int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for shortCode, count := range counts {
		if url, exists := r.urls[shortCode]; exists {
			url.ClickCount += count
		}
	}
	return nil
}

func (r *InMemoryURLRepository) DeleteExpired(ctx context.Context) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var count int64
	now := time.Now()
	for code, url := range r.urls {
		if url.ExpiresAt != nil && url.ExpiresAt.Before(now) {
			delete(r.urls, code)
			count++
		}
	}
	return count, nil
}

func (r *InMemoryURLRepository) Exists(ctx context.Context, shortCode string) (bool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.urls[shortCode]
	return exists, nil
}

func (r *InMemoryURLRepository) HealthCheck(ctx context.Context) error {
	return nil
}

// testServerWithURLAPI creates a test server with URL API configured.
func testServerWithURLAPI(t *testing.T) (*server.Server, string, func()) {
	t.Helper()

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
		URL: config.URLConfig{
			BaseURL:      "http://localhost:8080",
			ShortCodeLen: 7,
		},
	}

	var buf bytes.Buffer
	log := logger.New(&buf, "error")
	srv := server.New(cfg, log)

	// Set up in-memory repository
	repo := NewInMemoryURLRepository()
	srv.SetURLRepository(repo)

	// Create ID generator with collision detection
	baseGen := idgen.NewRandomGenerator(cfg.URL.ShortCodeLen)
	collisionGen := idgen.NewCollisionAwareGenerator(baseGen, repo, 3)

	// Create URL service and handler
	urlService := services.NewURLService(repo, collisionGen, cfg.URL.BaseURL)
	urlHandler := handlers.NewURLHandler(urlService)
	srv.SetURLHandler(urlHandler)

	// Create redirect service and handler
	redirectService := services.NewRedirectService(repo)
	redirectHandler := handlers.NewRedirectHandler(redirectService)
	srv.SetRedirectHandler(redirectHandler)

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

// noRedirectClient returns an HTTP client that doesn't follow redirects.
func noRedirectClient() *http.Client {
	return &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// httpGetNoRedirect makes a GET request without following redirects.
func httpGetNoRedirect(t *testing.T, url string) *http.Response {
	t.Helper()
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	require.NoError(t, err)

	resp, err := noRedirectClient().Do(req)
	require.NoError(t, err)
	return resp
}

// httpPost makes a POST request with JSON body.
func httpPost(t *testing.T, url string, body interface{}) *http.Response {
	t.Helper()
	jsonBody, err := json.Marshal(body)
	require.NoError(t, err)

	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

// httpDelete makes a DELETE request.
func httpDelete(t *testing.T, url string) *http.Response {
	t.Helper()
	ctx := context.Background()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func TestE2E_ShortenURL(t *testing.T) {
	_, baseURL, cleanup := testServerWithURLAPI(t)
	defer cleanup()

	t.Run("POST /api/v1/shorten creates and returns short URL", func(t *testing.T) {
		reqBody := handlers.ShortenRequest{
			URL: "https://example.com/very/long/path?query=value",
		}

		resp := httpPost(t, baseURL+"/api/v1/shorten", reqBody)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)
		assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

		var shortenResp handlers.ShortenResponse
		err := json.NewDecoder(resp.Body).Decode(&shortenResp)
		require.NoError(t, err)

		assert.NotEmpty(t, shortenResp.ShortURL)
		assert.NotEmpty(t, shortenResp.ShortCode)
		assert.Equal(t, 7, len(shortenResp.ShortCode)) // Default code length
		assert.Equal(t, reqBody.URL, shortenResp.OriginalURL)
		assert.NotEmpty(t, shortenResp.CreatedAt)
		assert.Nil(t, shortenResp.ExpiresAt)
	})

	t.Run("POST /api/v1/shorten with expires_in creates expiring URL", func(t *testing.T) {
		reqBody := handlers.ShortenRequest{
			URL:       "https://example.com/expires",
			ExpiresIn: "24h",
		}

		resp := httpPost(t, baseURL+"/api/v1/shorten", reqBody)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var shortenResp handlers.ShortenResponse
		err := json.NewDecoder(resp.Body).Decode(&shortenResp)
		require.NoError(t, err)

		assert.NotNil(t, shortenResp.ExpiresAt)

		// Verify expiry is approximately 24 hours from now
		expiresAt, err := time.Parse(time.RFC3339, *shortenResp.ExpiresAt)
		require.NoError(t, err)
		expectedExpiry := time.Now().Add(24 * time.Hour)
		assert.WithinDuration(t, expectedExpiry, expiresAt, 5*time.Second)
	})

	t.Run("POST /api/v1/shorten with empty URL returns 400", func(t *testing.T) {
		reqBody := handlers.ShortenRequest{
			URL: "",
		}

		resp := httpPost(t, baseURL+"/api/v1/shorten", reqBody)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		var errResp handlers.ErrorResponse
		err := json.NewDecoder(resp.Body).Decode(&errResp)
		require.NoError(t, err)
		assert.Equal(t, "EMPTY_URL", errResp.Code)
	})

	t.Run("POST /api/v1/shorten with invalid URL returns 400", func(t *testing.T) {
		reqBody := handlers.ShortenRequest{
			URL: "not-a-valid-url",
		}

		resp := httpPost(t, baseURL+"/api/v1/shorten", reqBody)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		var errResp handlers.ErrorResponse
		err := json.NewDecoder(resp.Body).Decode(&errResp)
		require.NoError(t, err)
		assert.Equal(t, "INVALID_URL", errResp.Code)
	})
}

func TestE2E_GetURL(t *testing.T) {
	_, baseURL, cleanup := testServerWithURLAPI(t)
	defer cleanup()

	t.Run("GET /api/v1/urls/:code returns URL info for created URL", func(t *testing.T) {
		// First create a URL
		reqBody := handlers.ShortenRequest{
			URL: "https://example.com/gettest",
		}
		createResp := httpPost(t, baseURL+"/api/v1/shorten", reqBody)
		require.Equal(t, http.StatusCreated, createResp.StatusCode)

		var shortenResp handlers.ShortenResponse
		err := json.NewDecoder(createResp.Body).Decode(&shortenResp)
		createResp.Body.Close()
		require.NoError(t, err)

		// Now get the URL info
		resp := httpGet(t, baseURL+"/api/v1/urls/"+shortenResp.ShortCode)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var infoResp handlers.URLInfoResponse
		err = json.NewDecoder(resp.Body).Decode(&infoResp)
		require.NoError(t, err)

		assert.Equal(t, shortenResp.ShortCode, infoResp.ShortCode)
		assert.Equal(t, "https://example.com/gettest", infoResp.OriginalURL)
		assert.Equal(t, int64(0), infoResp.ClickCount)
	})

	t.Run("GET /api/v1/urls/:code returns 404 for non-existent URL", func(t *testing.T) {
		resp := httpGet(t, baseURL+"/api/v1/urls/notfound123")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		var errResp handlers.ErrorResponse
		err := json.NewDecoder(resp.Body).Decode(&errResp)
		require.NoError(t, err)
		assert.Equal(t, "NOT_FOUND", errResp.Code)
	})
}

func TestE2E_DeleteURL(t *testing.T) {
	_, baseURL, cleanup := testServerWithURLAPI(t)
	defer cleanup()

	t.Run("DELETE /api/v1/urls/:code removes URL", func(t *testing.T) {
		// First create a URL
		reqBody := handlers.ShortenRequest{
			URL: "https://example.com/deletetest",
		}
		createResp := httpPost(t, baseURL+"/api/v1/shorten", reqBody)
		require.Equal(t, http.StatusCreated, createResp.StatusCode)

		var shortenResp handlers.ShortenResponse
		err := json.NewDecoder(createResp.Body).Decode(&shortenResp)
		createResp.Body.Close()
		require.NoError(t, err)

		// Delete the URL
		deleteResp := httpDelete(t, baseURL+"/api/v1/urls/"+shortenResp.ShortCode)
		deleteResp.Body.Close()
		assert.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

		// Verify it's gone
		getResp := httpGet(t, baseURL+"/api/v1/urls/"+shortenResp.ShortCode)
		defer getResp.Body.Close()
		assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
	})

	t.Run("DELETE /api/v1/urls/:code returns 404 for non-existent URL", func(t *testing.T) {
		resp := httpDelete(t, baseURL+"/api/v1/urls/notfound456")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)

		var errResp handlers.ErrorResponse
		err := json.NewDecoder(resp.Body).Decode(&errResp)
		require.NoError(t, err)
		assert.Equal(t, "NOT_FOUND", errResp.Code)
	})
}

func TestE2E_URLFlow_CreateRetrieveDelete(t *testing.T) {
	_, baseURL, cleanup := testServerWithURLAPI(t)
	defer cleanup()

	t.Run("complete URL lifecycle: create -> retrieve -> delete -> verify gone", func(t *testing.T) {
		// Step 1: Create
		reqBody := handlers.ShortenRequest{
			URL:       "https://example.com/lifecycle-test",
			ExpiresIn: "1h",
		}
		createResp := httpPost(t, baseURL+"/api/v1/shorten", reqBody)
		require.Equal(t, http.StatusCreated, createResp.StatusCode)

		var shortenResp handlers.ShortenResponse
		err := json.NewDecoder(createResp.Body).Decode(&shortenResp)
		createResp.Body.Close()
		require.NoError(t, err)

		shortCode := shortenResp.ShortCode

		// Step 2: Retrieve
		getResp := httpGet(t, baseURL+"/api/v1/urls/"+shortCode)
		assert.Equal(t, http.StatusOK, getResp.StatusCode)

		var infoResp handlers.URLInfoResponse
		err = json.NewDecoder(getResp.Body).Decode(&infoResp)
		getResp.Body.Close()
		require.NoError(t, err)

		assert.Equal(t, shortCode, infoResp.ShortCode)
		assert.Equal(t, reqBody.URL, infoResp.OriginalURL)
		assert.NotNil(t, infoResp.ExpiresAt)

		// Step 3: Delete
		deleteResp := httpDelete(t, baseURL+"/api/v1/urls/"+shortCode)
		deleteResp.Body.Close()
		assert.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

		// Step 4: Verify gone
		verifyResp := httpGet(t, baseURL+"/api/v1/urls/"+shortCode)
		defer verifyResp.Body.Close()
		assert.Equal(t, http.StatusNotFound, verifyResp.StatusCode)
	})
}

func TestE2E_ConcurrentShortenRequests(t *testing.T) {
	_, baseURL, cleanup := testServerWithURLAPI(t)
	defer cleanup()

	t.Run("handles concurrent shorten requests", func(t *testing.T) {
		const numRequests = 20
		results := make(chan int, numRequests)
		codes := make(chan string, numRequests)

		for i := 0; i < numRequests; i++ {
			go func(n int) {
				reqBody := handlers.ShortenRequest{
					URL: "https://example.com/concurrent/" + string(rune('a'+n)),
				}

				resp := httpPost(t, baseURL+"/api/v1/shorten", reqBody)
				defer resp.Body.Close()

				if resp.StatusCode == http.StatusCreated {
					var shortenResp handlers.ShortenResponse
					if err := json.NewDecoder(resp.Body).Decode(&shortenResp); err == nil {
						codes <- shortenResp.ShortCode
					}
				}
				results <- resp.StatusCode
			}(i)
		}

		successCount := 0
		for i := 0; i < numRequests; i++ {
			if <-results == http.StatusCreated {
				successCount++
			}
		}

		assert.Equal(t, numRequests, successCount)

		// Verify all codes are unique
		close(codes)
		uniqueCodes := make(map[string]bool)
		for code := range codes {
			uniqueCodes[code] = true
		}
		assert.Equal(t, numRequests, len(uniqueCodes))
	})
}

func TestE2E_Redirect(t *testing.T) {
	_, baseURL, cleanup := testServerWithURLAPI(t)
	defer cleanup()

	t.Run("GET /:code redirects to original URL with 302", func(t *testing.T) {
		// First create a URL
		reqBody := handlers.ShortenRequest{
			URL: "https://example.com/redirect-test",
		}
		createResp := httpPost(t, baseURL+"/api/v1/shorten", reqBody)
		require.Equal(t, http.StatusCreated, createResp.StatusCode)

		var shortenResp handlers.ShortenResponse
		err := json.NewDecoder(createResp.Body).Decode(&shortenResp)
		createResp.Body.Close()
		require.NoError(t, err)

		// Now access the redirect endpoint (without following redirect)
		resp := httpGetNoRedirect(t, baseURL+"/"+shortenResp.ShortCode)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusFound, resp.StatusCode, "should return 302 Found")
		assert.Equal(t, "https://example.com/redirect-test", resp.Header.Get("Location"))
	})

	t.Run("GET /:code returns 404 for non-existent code", func(t *testing.T) {
		resp := httpGetNoRedirect(t, baseURL+"/notexist123")
		defer resp.Body.Close()

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("GET /:code returns 410 Gone for expired URL", func(t *testing.T) {
		// Create a URL with very short expiry
		reqBody := handlers.ShortenRequest{
			URL:       "https://example.com/expiring",
			ExpiresIn: "1ms", // Expires almost immediately
		}
		createResp := httpPost(t, baseURL+"/api/v1/shorten", reqBody)
		require.Equal(t, http.StatusCreated, createResp.StatusCode)

		var shortenResp handlers.ShortenResponse
		err := json.NewDecoder(createResp.Body).Decode(&shortenResp)
		createResp.Body.Close()
		require.NoError(t, err)

		// Wait for expiry
		time.Sleep(10 * time.Millisecond)

		// Try to redirect
		resp := httpGetNoRedirect(t, baseURL+"/"+shortenResp.ShortCode)
		defer resp.Body.Close()

		assert.Equal(t, http.StatusGone, resp.StatusCode)
	})

	t.Run("redirect increments click count", func(t *testing.T) {
		// Create a URL
		reqBody := handlers.ShortenRequest{
			URL: "https://example.com/click-count-test",
		}
		createResp := httpPost(t, baseURL+"/api/v1/shorten", reqBody)
		require.Equal(t, http.StatusCreated, createResp.StatusCode)

		var shortenResp handlers.ShortenResponse
		err := json.NewDecoder(createResp.Body).Decode(&shortenResp)
		createResp.Body.Close()
		require.NoError(t, err)

		// Access redirect multiple times
		for i := 0; i < 3; i++ {
			resp := httpGetNoRedirect(t, baseURL+"/"+shortenResp.ShortCode)
			resp.Body.Close()
			assert.Equal(t, http.StatusFound, resp.StatusCode)
		}

		// Check click count
		infoResp := httpGet(t, baseURL+"/api/v1/urls/"+shortenResp.ShortCode)
		defer infoResp.Body.Close()

		var urlInfo handlers.URLInfoResponse
		err = json.NewDecoder(infoResp.Body).Decode(&urlInfo)
		require.NoError(t, err)

		assert.Equal(t, int64(3), urlInfo.ClickCount)
	})
}

func TestE2E_RedirectLatency(t *testing.T) {
	_, baseURL, cleanup := testServerWithURLAPI(t)
	defer cleanup()

	t.Run("redirect latency is under 50ms for in-memory lookups", func(t *testing.T) {
		// Create a URL
		reqBody := handlers.ShortenRequest{
			URL: "https://example.com/latency-test",
		}
		createResp := httpPost(t, baseURL+"/api/v1/shorten", reqBody)
		require.Equal(t, http.StatusCreated, createResp.StatusCode)

		var shortenResp handlers.ShortenResponse
		err := json.NewDecoder(createResp.Body).Decode(&shortenResp)
		createResp.Body.Close()
		require.NoError(t, err)

		// Warm up the connection pool
		warmupResp := httpGetNoRedirect(t, baseURL+"/"+shortenResp.ShortCode)
		warmupResp.Body.Close()

		// Measure latency for multiple requests
		const numRequests = 10
		var totalLatency time.Duration

		for i := 0; i < numRequests; i++ {
			start := time.Now()
			resp := httpGetNoRedirect(t, baseURL+"/"+shortenResp.ShortCode)
			latency := time.Since(start)
			resp.Body.Close()

			assert.Equal(t, http.StatusFound, resp.StatusCode)
			totalLatency += latency
		}

		avgLatency := totalLatency / numRequests
		t.Logf("Average redirect latency: %v", avgLatency)

		// For in-memory repository, latency should be well under 50ms
		// (accounting for test environment variability)
		assert.Less(t, avgLatency, 50*time.Millisecond,
			"average redirect latency should be under 50ms for in-memory lookups")
	})
}

func TestE2E_RedirectFullFlow(t *testing.T) {
	_, baseURL, cleanup := testServerWithURLAPI(t)
	defer cleanup()

	t.Run("complete redirect flow: create -> redirect -> verify click count -> delete", func(t *testing.T) {
		originalURL := "https://example.com/full-flow-test"

		// Step 1: Create short URL
		reqBody := handlers.ShortenRequest{
			URL: originalURL,
		}
		createResp := httpPost(t, baseURL+"/api/v1/shorten", reqBody)
		require.Equal(t, http.StatusCreated, createResp.StatusCode)

		var shortenResp handlers.ShortenResponse
		err := json.NewDecoder(createResp.Body).Decode(&shortenResp)
		createResp.Body.Close()
		require.NoError(t, err)

		shortCode := shortenResp.ShortCode

		// Step 2: Verify redirect works
		redirectResp := httpGetNoRedirect(t, baseURL+"/"+shortCode)
		assert.Equal(t, http.StatusFound, redirectResp.StatusCode)
		assert.Equal(t, originalURL, redirectResp.Header.Get("Location"))
		redirectResp.Body.Close()

		// Step 3: Verify click count increased
		infoResp := httpGet(t, baseURL+"/api/v1/urls/"+shortCode)
		var urlInfo handlers.URLInfoResponse
		err = json.NewDecoder(infoResp.Body).Decode(&urlInfo)
		infoResp.Body.Close()
		require.NoError(t, err)
		assert.Equal(t, int64(1), urlInfo.ClickCount)

		// Step 4: Delete URL
		deleteResp := httpDelete(t, baseURL+"/api/v1/urls/"+shortCode)
		deleteResp.Body.Close()
		assert.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

		// Step 5: Verify redirect no longer works
		finalResp := httpGetNoRedirect(t, baseURL+"/"+shortCode)
		defer finalResp.Body.Close()
		assert.Equal(t, http.StatusNotFound, finalResp.StatusCode)
	})
}
