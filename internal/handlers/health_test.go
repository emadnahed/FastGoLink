package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthHandler(t *testing.T) {
	handler := NewHealthHandler()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.Health(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var response HealthResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response.Status)
	assert.NotEmpty(t, response.Timestamp)
}

func TestReadyHandler_Ready(t *testing.T) {
	handler := NewHealthHandler()

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	handler.Ready(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var response ReadyResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "ready", response.Status)
	assert.NotEmpty(t, response.Timestamp)
}

func TestReadyHandler_NotReady(t *testing.T) {
	handler := NewHealthHandler()
	handler.SetReady(false)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	handler.Ready(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var response ReadyResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "not ready", response.Status)
}

func TestHealthHandler_SetReady(t *testing.T) {
	handler := NewHealthHandler()

	// Default should be ready
	assert.True(t, handler.IsReady())

	handler.SetReady(false)
	assert.False(t, handler.IsReady())

	handler.SetReady(true)
	assert.True(t, handler.IsReady())
}

func TestHealthResponse_Structure(t *testing.T) {
	handler := NewHealthHandler()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.Health(rec, req)

	var response map[string]interface{}
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	// Verify expected fields exist
	assert.Contains(t, response, "status")
	assert.Contains(t, response, "timestamp")
}

func TestReadyResponse_WithChecks(t *testing.T) {
	handler := NewHealthHandler()

	// Add a dependency check
	handler.AddCheck("database", func() bool { return true })

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	handler.Ready(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var response ReadyResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "ready", response.Status)
	assert.Contains(t, response.Checks, "database")
	assert.Equal(t, "ok", response.Checks["database"])
}

func TestReadyResponse_WithFailingCheck(t *testing.T) {
	handler := NewHealthHandler()

	// Add a failing dependency check
	handler.AddCheck("database", func() bool { return false })

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	handler.Ready(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)

	var response ReadyResponse
	err := json.Unmarshal(rec.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "not ready", response.Status)
	assert.Contains(t, response.Checks, "database")
	assert.Equal(t, "fail", response.Checks["database"])
}
