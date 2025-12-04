package handler

import (
	"net/http"
)

// HealthHandler handles health check endpoint
type HealthHandler struct{}

// NewHealthHandler creates a new health handler
func NewHealthHandler() *HealthHandler {
	return &HealthHandler{}
}

// HealthCheck returns the health status
func (h *HealthHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":  "ok",
		"service": "NMSlite Mock API",
		"version": "1.0.0",
	}
	respondSuccess(w, http.StatusOK, response)
}
