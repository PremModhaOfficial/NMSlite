package api

import (
	"encoding/json"
	"net/http"
	"time"
)

// HealthHandler handles health check endpoints
type HealthHandler struct{}

// NewHealthHandler creates a new health handler
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// HealthResponse represents the health check response
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
}

// ReadinessResponse represents the readiness check response
type ReadinessResponse struct {
	Status    string            `json:"status"`
	Timestamp time.Time         `json:"timestamp"`
	Checks    map[string]string `json:"checks,omitempty"`
	Error     string            `json:"error,omitempty"`
}

// Health handles GET /health (liveness probe)
func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Status:    "ok",
		Timestamp: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// Ready handles GET /ready (readiness probe)
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	// TODO: Add actual dependency checks
	// - Database connection
	// - Queue accessibility
	// - Plugin directory readable

	checks := map[string]string{
		"database": "ok",
		"queue":    "ok",
		"plugins":  "ok",
	}

	response := ReadinessResponse{
		Status:    "ready",
		Timestamp: time.Now(),
		Checks:    checks,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}
