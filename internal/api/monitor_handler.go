package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"

	"net"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nmslite/nmslite/internal/database/db_gen"
	"github.com/sqlc-dev/pqtype"
)

// Helper to convert string IP to pqtype.Inet
func stringToInet(ipStr string) (pqtype.Inet, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return pqtype.Inet{}, net.InvalidAddrError("invalid IP address")
	}
	// Determine mask based on IP version (optional, but good for completeness)
	mask := 32
	if ip.To4() == nil {
		mask = 128
	}
	return pqtype.Inet{IPNet: net.IPNet{IP: ip, Mask: net.CIDRMask(mask, mask)}, Valid: true}, nil
}

// MonitorHandler handles monitor (device) endpoints
type MonitorHandler struct {
	q db_gen.Querier
}

// NewMonitorHandler creates a new monitor handler
func NewMonitorHandler(q db_gen.Querier) *MonitorHandler {
	return &MonitorHandler{q: q}
}

// List handles GET /api/v1/monitors
func (h *MonitorHandler) List(w http.ResponseWriter, r *http.Request) {
	monitors, err := h.q.ListMonitors(r.Context())
	if err != nil {
		sendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Failed to list monitors", err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"data":  monitors,
		"total": len(monitors),
	})
}

// Get handles GET /api/v1/monitors/{id}
func (h *MonitorHandler) Get(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid UUID format", err)
		return
	}

	monitor, err := h.q.GetMonitor(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			sendError(w, r, http.StatusNotFound, "NOT_FOUND", "Monitor not found", nil)
			return
		}
		sendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Failed to get monitor", err)
		return
	}

	sendJSON(w, http.StatusOK, monitor)
}

// Update handles PATCH /api/v1/monitors/{id}
func (h *MonitorHandler) Update(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid UUID format", err)
		return
	}

	var input struct {
		DisplayName            string `json:"display_name"`
		Hostname               string `json:"hostname"`
		IPAddress              string `json:"ip_address"`
		PluginID               string `json:"plugin_id"`
		CredentialProfileID    string `json:"credential_profile_id"`
		PollingIntervalSeconds int32  `json:"polling_interval_seconds"`
		Port                   int32  `json:"port"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body", err)
		return
	}

	// Get existing to merge
	existing, err := h.q.GetMonitor(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			sendError(w, r, http.StatusNotFound, "NOT_FOUND", "Monitor not found", nil)
			return
		}
		sendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Failed to fetch existing monitor", err)
		return
	}

	// Prepare params with defaults from existing
	params := db_gen.UpdateMonitorParams{
		ID:                     id,
		DisplayName:            existing.DisplayName,
		Hostname:               existing.Hostname,
		IpAddress:              existing.IpAddress, // Note: db_gen uses IpAddress (capital I)
		PluginID:               existing.PluginID,
		CredentialProfileID:    existing.CredentialProfileID,
		PollingIntervalSeconds: existing.PollingIntervalSeconds,
		Port:                   existing.Port,
	}

	// Apply updates
	if input.DisplayName != "" {
		params.DisplayName = sql.NullString{String: input.DisplayName, Valid: true}
	}
	if input.Hostname != "" {
		params.Hostname = sql.NullString{String: input.Hostname, Valid: true}
	}
	if input.IPAddress != "" {
		ipInet, err := stringToInet(input.IPAddress)
		if err != nil {
			sendError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid IP Address", err)
			return
		}
		params.IpAddress = ipInet
	}
	if input.PluginID != "" {
		params.PluginID = input.PluginID
	}
	if input.CredentialProfileID != "" {
		if id, err := uuid.Parse(input.CredentialProfileID); err == nil {
			params.CredentialProfileID = uuid.NullUUID{UUID: id, Valid: true}
		}
	}
	if input.PollingIntervalSeconds > 0 {
		params.PollingIntervalSeconds = sql.NullInt32{Int32: input.PollingIntervalSeconds, Valid: true}
	}
	if input.Port > 0 {
		params.Port = sql.NullInt32{Int32: input.Port, Valid: true}
	}

	monitor, err := h.q.UpdateMonitor(r.Context(), params)
	if err != nil {
		sendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Failed to update monitor", err)
		return
	}

	sendJSON(w, http.StatusOK, monitor)
}

// Delete handles DELETE /api/v1/monitors/{id}
func (h *MonitorHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid UUID format", err)
		return
	}

	err = h.q.DeleteMonitor(r.Context(), id)
	if err != nil {
		sendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Failed to delete monitor", err)
		return
	}

	sendJSON(w, http.StatusNoContent, nil)
}

// Restore handles PATCH /api/v1/monitors/{id}/restore
func (h *MonitorHandler) Restore(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement monitor restoration
	sendError(w, r, http.StatusNotImplemented, "NOT_IMPLEMENTED", "This endpoint is not yet implemented", nil)
}

// GetMetrics handles GET /api/v1/monitors/{id}/metrics
func (h *MonitorHandler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement metrics retrieval
	sendError(w, r, http.StatusNotImplemented, "NOT_IMPLEMENTED", "This endpoint is not yet implemented", nil)
}
