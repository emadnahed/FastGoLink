// Package metrics provides Prometheus metrics for observability.
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	// HTTPRequestsTotal counts total HTTP requests by method, path, and status.
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)

	// HTTPRequestDuration measures request latency in seconds.
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		},
		[]string{"method", "path"},
	)

	// CacheHitsTotal counts cache hits.
	CacheHitsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "cache_hits_total",
			Help: "Total number of cache hits",
		},
	)

	// CacheMissesTotal counts cache misses.
	CacheMissesTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "cache_misses_total",
			Help: "Total number of cache misses",
		},
	)

	// DBQueryDuration measures database query latency.
	DBQueryDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "db_query_duration_seconds",
			Help:    "Database query duration in seconds",
			Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1},
		},
		[]string{"operation"},
	)

	// ActiveConnections tracks current active connections.
	ActiveConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "active_connections",
			Help: "Number of active connections",
		},
	)

	// URLsCreatedTotal counts URLs created.
	URLsCreatedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "urls_created_total",
			Help: "Total number of URLs created",
		},
	)

	// RedirectsTotal counts redirect requests.
	RedirectsTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "redirects_total",
			Help: "Total number of redirect requests",
		},
	)

	// RateLimitedTotal counts rate-limited requests.
	RateLimitedTotal = promauto.NewCounter(
		prometheus.CounterOpts{
			Name: "rate_limited_total",
			Help: "Total number of rate-limited requests",
		},
	)
)

// Handler returns the Prometheus metrics HTTP handler.
func Handler() http.Handler {
	return promhttp.Handler()
}

// RecordRequest records an HTTP request metric.
func RecordRequest(method, path string, status int, duration time.Duration) {
	HTTPRequestsTotal.WithLabelValues(method, path, strconv.Itoa(status)).Inc()
	HTTPRequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())
}

// RecordCacheHit records a cache hit.
func RecordCacheHit() {
	CacheHitsTotal.Inc()
}

// RecordCacheMiss records a cache miss.
func RecordCacheMiss() {
	CacheMissesTotal.Inc()
}

// RecordDBQuery records a database query duration.
func RecordDBQuery(operation string, duration time.Duration) {
	DBQueryDuration.WithLabelValues(operation).Observe(duration.Seconds())
}

// RecordURLCreated records a URL creation.
func RecordURLCreated() {
	URLsCreatedTotal.Inc()
}

// RecordRedirect records a redirect.
func RecordRedirect() {
	RedirectsTotal.Inc()
}

// RecordRateLimited records a rate-limited request.
func RecordRateLimited() {
	RateLimitedTotal.Inc()
}
