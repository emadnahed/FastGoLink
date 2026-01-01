// Package benchmark contains performance benchmarks for GoURL.
package benchmark

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gourl/gourl/internal/config"
	"github.com/gourl/gourl/internal/handlers"
	"github.com/gourl/gourl/internal/idgen"
	"github.com/gourl/gourl/internal/models"
	"github.com/gourl/gourl/internal/server"
	"github.com/gourl/gourl/internal/services"
	"github.com/gourl/gourl/pkg/logger"
)

// InMemoryURLRepository implements repository.URLRepository for benchmarking.
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

// setupBenchServer creates a test server for benchmarking and returns its URL.
func setupBenchServer(b *testing.B) (string, func()) {
	b.Helper()

	cfg := &config.Config{
		App: config.AppConfig{
			Env:      "test",
			LogLevel: "error",
		},
		Server: config.ServerConfig{
			Host:            "127.0.0.1",
			Port:            0,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
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
	collisionGen := idgen.NewCollisionAwareGenerator(baseGen, repo, 5)

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
	if addr == "" {
		b.Fatal("server failed to start")
	}

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}

	return "http://" + addr, cleanup
}

// BenchmarkHealthEndpoint benchmarks the /health endpoint.
func BenchmarkHealthEndpoint(b *testing.B) {
	baseURL, cleanup := setupBenchServer(b)
	defer cleanup()

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := client.Get(baseURL + "/health")
		if err != nil {
			b.Error(err)
			continue
		}
		resp.Body.Close()
	}
}

// BenchmarkShortenURL benchmarks URL shortening.
func BenchmarkShortenURL(b *testing.B) {
	baseURL, cleanup := setupBenchServer(b)
	defer cleanup()

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reqBody := fmt.Sprintf(`{"url":"https://example.com/bench/%d"}`, i)
		resp, err := client.Post(
			baseURL+"/api/v1/shorten",
			"application/json",
			bytes.NewBufferString(reqBody),
		)
		if err != nil {
			b.Error(err)
			continue
		}
		resp.Body.Close()
	}
}

// BenchmarkShortenURLParallel benchmarks parallel URL shortening.
func BenchmarkShortenURLParallel(b *testing.B) {
	baseURL, cleanup := setupBenchServer(b)
	defer cleanup()

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        200,
			MaxIdleConnsPerHost: 200,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	var counter int64

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := atomic.AddInt64(&counter, 1)
			reqBody := fmt.Sprintf(`{"url":"https://example.com/parallel/%d"}`, i)
			resp, err := client.Post(
				baseURL+"/api/v1/shorten",
				"application/json",
				bytes.NewBufferString(reqBody),
			)
			if err != nil {
				continue // Ignore errors in parallel benchmark
			}
			resp.Body.Close()
		}
	})
}

// BenchmarkRedirect benchmarks URL redirect (the critical path).
func BenchmarkRedirect(b *testing.B) {
	baseURL, cleanup := setupBenchServer(b)
	defer cleanup()

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        200,
			MaxIdleConnsPerHost: 200,
			IdleConnTimeout:     90 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		},
	}

	// Create a URL to redirect
	reqBody := `{"url":"https://example.com/redirect-bench"}`
	resp, err := client.Post(
		baseURL+"/api/v1/shorten",
		"application/json",
		bytes.NewBufferString(reqBody),
	)
	if err != nil {
		b.Fatal(err)
	}

	var shortenResp handlers.ShortenResponse
	json.NewDecoder(resp.Body).Decode(&shortenResp)
	resp.Body.Close()

	redirectURL := baseURL + "/" + shortenResp.ShortCode

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := client.Get(redirectURL)
		if err != nil {
			b.Error(err)
			continue
		}
		resp.Body.Close()
	}
}

// BenchmarkRedirectLatency measures redirect latency with percentiles.
func BenchmarkRedirectLatency(b *testing.B) {
	baseURL, cleanup := setupBenchServer(b)
	defer cleanup()

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 100,
			IdleConnTimeout:     90 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Create a URL to redirect
	reqBody := `{"url":"https://example.com/latency-bench"}`
	resp, err := client.Post(
		baseURL+"/api/v1/shorten",
		"application/json",
		bytes.NewBufferString(reqBody),
	)
	if err != nil {
		b.Fatal(err)
	}

	var shortenResp handlers.ShortenResponse
	json.NewDecoder(resp.Body).Decode(&shortenResp)
	resp.Body.Close()

	redirectURL := baseURL + "/" + shortenResp.ShortCode

	// Warm up
	for i := 0; i < 10; i++ {
		resp, _ := client.Get(redirectURL)
		if resp != nil {
			resp.Body.Close()
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resp, err := client.Get(redirectURL)
		if err != nil {
			b.Error(err)
			continue
		}
		resp.Body.Close()
	}
}

// BenchmarkIDGeneration benchmarks ID generation.
func BenchmarkIDGeneration(b *testing.B) {
	gen := idgen.NewRandomGenerator(7)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := gen.Generate()
			if err != nil {
				b.Error(err)
			}
		}
	})
}

// BenchmarkSnowflakeIDGeneration benchmarks Snowflake ID generation.
func BenchmarkSnowflakeIDGeneration(b *testing.B) {
	gen, err := idgen.NewSnowflakeGenerator(1, 7)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, err := gen.Generate()
			if err != nil {
				b.Error(err)
			}
		}
	})
}

// BenchmarkConcurrentLoad simulates realistic concurrent load.
func BenchmarkConcurrentLoad(b *testing.B) {
	baseURL, cleanup := setupBenchServer(b)
	defer cleanup()

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        200,
			MaxIdleConnsPerHost: 200,
			IdleConnTimeout:     90 * time.Second,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Pre-create some URLs
	var shortCodes []string
	for i := 0; i < 100; i++ {
		reqBody := fmt.Sprintf(`{"url":"https://example.com/concurrent/%d"}`, i)
		resp, err := client.Post(
			baseURL+"/api/v1/shorten",
			"application/json",
			bytes.NewBufferString(reqBody),
		)
		if err != nil {
			b.Fatal(err)
		}

		var shortenResp handlers.ShortenResponse
		json.NewDecoder(resp.Body).Decode(&shortenResp)
		resp.Body.Close()
		shortCodes = append(shortCodes, shortenResp.ShortCode)
	}

	var counter int64

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			i := atomic.AddInt64(&counter, 1)
			// 80% redirects, 20% shortens (typical real-world ratio)
			if i%5 == 0 {
				// Shorten
				reqBody := fmt.Sprintf(`{"url":"https://example.com/load/%d"}`, i)
				resp, err := client.Post(
					baseURL+"/api/v1/shorten",
					"application/json",
					bytes.NewBufferString(reqBody),
				)
				if err != nil {
					continue
				}
				resp.Body.Close()
			} else {
				// Redirect
				code := shortCodes[int(i)%len(shortCodes)]
				resp, err := client.Get(baseURL + "/" + code)
				if err != nil {
					continue
				}
				resp.Body.Close()
			}
		}
	})
}

// TestConcurrencyStress tests system under sustained concurrent load.
func TestConcurrencyStress(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	baseURL, cleanup := setupStressServer(t)
	defer cleanup()

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        1000,
			MaxIdleConnsPerHost: 1000,
			MaxConnsPerHost:     1000,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Create URLs first
	var shortCodes []string
	for i := 0; i < 50; i++ {
		reqBody := fmt.Sprintf(`{"url":"https://example.com/stress/%d"}`, i)
		resp, err := client.Post(
			baseURL+"/api/v1/shorten",
			"application/json",
			bytes.NewBufferString(reqBody),
		)
		if err != nil {
			t.Fatal(err)
		}

		var shortenResp handlers.ShortenResponse
		json.NewDecoder(resp.Body).Decode(&shortenResp)
		resp.Body.Close()
		shortCodes = append(shortCodes, shortenResp.ShortCode)
	}

	// Test parameters
	concurrency := 100
	requestsPerWorker := 100
	totalRequests := concurrency * requestsPerWorker

	var (
		successCount int64
		failCount    int64
		totalLatency int64
		mu           sync.Mutex
		latencies    []time.Duration
	)

	latencies = make([]time.Duration, 0, totalRequests)

	var wg sync.WaitGroup
	start := time.Now()

	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for r := 0; r < requestsPerWorker; r++ {
				code := shortCodes[(workerID+r)%len(shortCodes)]
				reqStart := time.Now()

				resp, err := client.Get(baseURL + "/" + code)
				latency := time.Since(reqStart)

				if err != nil {
					atomic.AddInt64(&failCount, 1)
					continue
				}
				resp.Body.Close()

				if resp.StatusCode == http.StatusFound {
					atomic.AddInt64(&successCount, 1)
					atomic.AddInt64(&totalLatency, int64(latency))

					mu.Lock()
					latencies = append(latencies, latency)
					mu.Unlock()
				} else {
					atomic.AddInt64(&failCount, 1)
				}
			}
		}(w)
	}

	wg.Wait()
	duration := time.Since(start)

	// Calculate percentiles
	if len(latencies) > 0 {
		sortDurations(latencies)
		p50 := latencies[len(latencies)*50/100]
		p95 := latencies[len(latencies)*95/100]
		p99 := latencies[len(latencies)*99/100]

		rps := float64(successCount) / duration.Seconds()
		avgLatency := time.Duration(totalLatency / successCount)

		t.Logf("\n"+
			"═══════════════════════════════════════════════════════════════\n"+
			"  CONCURRENCY STRESS TEST RESULTS\n"+
			"═══════════════════════════════════════════════════════════════\n"+
			"  Concurrency:     %d workers\n"+
			"  Total Requests:  %d\n"+
			"  Duration:        %v\n"+
			"───────────────────────────────────────────────────────────────\n"+
			"  Successful:      %d (%.2f%%)\n"+
			"  Failed:          %d\n"+
			"  RPS:             %.2f req/sec\n"+
			"───────────────────────────────────────────────────────────────\n"+
			"  Avg Latency:     %v\n"+
			"  P50 Latency:     %v\n"+
			"  P95 Latency:     %v\n"+
			"  P99 Latency:     %v\n"+
			"═══════════════════════════════════════════════════════════════\n",
			concurrency,
			totalRequests,
			duration,
			successCount, float64(successCount)/float64(totalRequests)*100,
			failCount,
			rps,
			avgLatency,
			p50,
			p95,
			p99,
		)

		// Assertions
		if float64(successCount)/float64(totalRequests) < 0.99 {
			t.Errorf("Success rate below 99%%: got %.2f%%", float64(successCount)/float64(totalRequests)*100)
		}
		if p99 > 100*time.Millisecond {
			t.Errorf("P99 latency too high: got %v, want < 100ms", p99)
		}
	}
}

// TestLatencyPercentiles tests latency distribution under load.
func TestLatencyPercentiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping latency test in short mode")
	}

	baseURL, cleanup := setupStressServer(t)
	defer cleanup()

	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// Create a URL
	reqBody := `{"url":"https://example.com/latency-percentile"}`
	resp, err := client.Post(
		baseURL+"/api/v1/shorten",
		"application/json",
		bytes.NewBufferString(reqBody),
	)
	if err != nil {
		t.Fatal(err)
	}

	var shortenResp handlers.ShortenResponse
	json.NewDecoder(resp.Body).Decode(&shortenResp)
	resp.Body.Close()

	redirectURL := baseURL + "/" + shortenResp.ShortCode

	// Warm up
	for i := 0; i < 100; i++ {
		resp, _ := client.Get(redirectURL)
		if resp != nil {
			resp.Body.Close()
		}
	}

	// Measure latencies
	numRequests := 1000
	latencies := make([]time.Duration, 0, numRequests)

	for i := 0; i < numRequests; i++ {
		start := time.Now()
		resp, err := client.Get(redirectURL)
		latency := time.Since(start)

		if err != nil {
			continue
		}
		resp.Body.Close()
		latencies = append(latencies, latency)
	}

	if len(latencies) == 0 {
		t.Fatal("No successful requests")
	}

	sortDurations(latencies)

	min := latencies[0]
	max := latencies[len(latencies)-1]
	p50 := latencies[len(latencies)*50/100]
	p90 := latencies[len(latencies)*90/100]
	p95 := latencies[len(latencies)*95/100]
	p99 := latencies[len(latencies)*99/100]

	var total time.Duration
	for _, l := range latencies {
		total += l
	}
	avg := total / time.Duration(len(latencies))

	t.Logf("\n"+
		"═══════════════════════════════════════════════════════════════\n"+
		"  LATENCY PERCENTILE ANALYSIS\n"+
		"═══════════════════════════════════════════════════════════════\n"+
		"  Requests:  %d\n"+
		"───────────────────────────────────────────────────────────────\n"+
		"  Min:       %v\n"+
		"  Avg:       %v\n"+
		"  P50:       %v\n"+
		"  P90:       %v\n"+
		"  P95:       %v\n"+
		"  P99:       %v\n"+
		"  Max:       %v\n"+
		"═══════════════════════════════════════════════════════════════\n",
		len(latencies),
		min, avg, p50, p90, p95, p99, max,
	)

	// Assertions for in-memory repository
	if p50 > 5*time.Millisecond {
		t.Errorf("P50 latency too high: got %v, want < 5ms", p50)
	}
	if p99 > 50*time.Millisecond {
		t.Errorf("P99 latency too high: got %v, want < 50ms", p99)
	}
}

// setupStressServer creates a server for stress testing.
func setupStressServer(t *testing.T) (string, func()) {
	t.Helper()

	cfg := &config.Config{
		App: config.AppConfig{
			Env:      "test",
			LogLevel: "error",
		},
		Server: config.ServerConfig{
			Host:            "127.0.0.1",
			Port:            0,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    30 * time.Second,
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

	repo := NewInMemoryURLRepository()
	srv.SetURLRepository(repo)

	baseGen := idgen.NewRandomGenerator(cfg.URL.ShortCodeLen)
	collisionGen := idgen.NewCollisionAwareGenerator(baseGen, repo, 5)

	urlService := services.NewURLService(repo, collisionGen, cfg.URL.BaseURL)
	urlHandler := handlers.NewURLHandler(urlService)
	srv.SetURLHandler(urlHandler)

	redirectService := services.NewRedirectService(repo)
	redirectHandler := handlers.NewRedirectHandler(redirectService)
	srv.SetRedirectHandler(redirectHandler)

	go func() { _ = srv.Start() }()
	time.Sleep(100 * time.Millisecond)

	addr := srv.Addr()
	if addr == "" {
		t.Fatal("server failed to start")
	}

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}

	return "http://" + addr, cleanup
}

// sortDurations sorts a slice of durations in place using insertion sort.
func sortDurations(d []time.Duration) {
	for i := 1; i < len(d); i++ {
		key := d[i]
		j := i - 1
		for j >= 0 && d[j] > key {
			d[j+1] = d[j]
			j--
		}
		d[j+1] = key
	}
}
