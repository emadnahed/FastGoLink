package handlers

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// HealthResponse represents the response for the health endpoint.
type HealthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// ReadyResponse represents the response for the ready endpoint.
type ReadyResponse struct {
	Status    string            `json:"status"`
	Timestamp string            `json:"timestamp"`
	Checks    map[string]string `json:"checks,omitempty"`
}

// CheckFunc is a function that checks if a dependency is ready.
type CheckFunc func() bool

// HealthHandler handles health check endpoints.
type HealthHandler struct {
	ready  bool
	checks map[string]CheckFunc
	mu     sync.RWMutex
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{
		ready:  true,
		checks: make(map[string]CheckFunc),
	}
}

// Health handles the /health endpoint.
// This endpoint indicates if the service is running.
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Status:    "healthy",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	writeJSON(w, http.StatusOK, response)
}

// Ready handles the /ready endpoint.
// This endpoint indicates if the service is ready to accept traffic.
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	checks := make(map[string]string)
	allReady := h.ready

	// Run all registered checks
	for name, check := range h.checks {
		if check() {
			checks[name] = "ok"
		} else {
			checks[name] = "fail"
			allReady = false
		}
	}

	status := "ready"
	statusCode := http.StatusOK

	if !allReady {
		status = "not ready"
		statusCode = http.StatusServiceUnavailable
	}

	response := ReadyResponse{
		Status:    status,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	if len(checks) > 0 {
		response.Checks = checks
	}

	writeJSON(w, statusCode, response)
}

// SetReady sets the ready state.
func (h *HealthHandler) SetReady(ready bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ready = ready
}

// IsReady returns the current ready state.
func (h *HealthHandler) IsReady() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.ready
}

// AddCheck adds a dependency check.
func (h *HealthHandler) AddCheck(name string, check CheckFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks[name] = check
}

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
