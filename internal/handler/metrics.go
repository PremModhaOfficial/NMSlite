package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/nmslite/nmslite/internal/model"
	"github.com/nmslite/nmslite/internal/store"
)

// MetricsHandler handles metrics endpoints
type MetricsHandler struct {
	store *store.MockStore
}

// NewMetricsHandler creates a new metrics handler
func NewMetricsHandler(s *store.MockStore) *MetricsHandler {
	return &MetricsHandler{store: s}
}

// GetLatestMetrics retrieves the latest metrics for a device
func (h *MetricsHandler) GetLatestMetrics(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "Invalid device ID")
		return
	}

	// Verify device exists
	if h.store.GetDevice(id) == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "Device not found")
		return
	}

	metrics := h.store.GetLatestMetrics(id)
	if metrics == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "No metrics found for this device")
		return
	}

	respondSuccess(w, http.StatusOK, metrics)
}

// GetMetricsHistory retrieves historical metrics for a device
func (h *MetricsHandler) GetMetricsHistory(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "Invalid device ID")
		return
	}

	// Verify device exists
	if h.store.GetDevice(id) == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "Device not found")
		return
	}

	var req HistoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Allow empty body, use defaults
	}

	limit := req.Limit
	if limit == 0 || limit > 100 {
		limit = 24 // Default 24 hours worth
	}

	metrics := h.store.GetMetricsHistory(id, limit)
	if metrics == nil {
		metrics = []*model.DeviceMetrics{}
	}

	response := map[string]interface{}{
		"device_id": id,
		"count":     len(metrics),
		"metrics":   metrics,
	}

	respondSuccess(w, http.StatusOK, response)
}
