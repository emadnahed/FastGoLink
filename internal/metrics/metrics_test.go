package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler(t *testing.T) {
	handler := Handler()
	require.NotNil(t, handler)

	// Test that it returns a valid HTTP handler
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	// Check for a metric that's always present
	assert.Contains(t, rec.Body.String(), "cache_hits_total")
}

func TestRecordRequest(t *testing.T) {
	// This should not panic
	RecordRequest("GET", "/test", 200, 100*time.Millisecond)
	RecordRequest("POST", "/api/v1/shorten", 201, 50*time.Millisecond)
	RecordRequest("GET", "/nonexistent", 404, 10*time.Millisecond)
}

func TestRecordCacheHit(t *testing.T) {
	// This should not panic
	RecordCacheHit()
}

func TestRecordCacheMiss(t *testing.T) {
	// This should not panic
	RecordCacheMiss()
}

func TestRecordDBQuery(t *testing.T) {
	// This should not panic
	RecordDBQuery("create", 50*time.Millisecond)
	RecordDBQuery("read", 10*time.Millisecond)
	RecordDBQuery("delete", 30*time.Millisecond)
}

func TestRecordURLCreated(t *testing.T) {
	// This should not panic
	RecordURLCreated()
}

func TestRecordRedirect(t *testing.T) {
	// This should not panic
	RecordRedirect()
}

func TestRecordRateLimited(t *testing.T) {
	// This should not panic
	RecordRateLimited()
}
