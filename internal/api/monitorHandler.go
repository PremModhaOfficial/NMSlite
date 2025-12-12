package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nmslite/nmslite/internal/channels"
	"github.com/nmslite/nmslite/internal/database"
	"github.com/nmslite/nmslite/internal/database/dbgen"
)

// MonitorHandler handles monitor (device) endpoints
type MonitorHandler struct {
	pool   *pgxpool.Pool
	q      dbgen.Querier
	events *channels.EventChannels
}

// NewMonitorHandler creates a new monitor handler
func NewMonitorHandler(pool *pgxpool.Pool, events *channels.EventChannels) *MonitorHandler {
	return &MonitorHandler{
		pool:   pool,
		q:      dbgen.New(pool),
		events: events,
	}
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

// Create handles POST /api/v1/monitors
// Manual provisioning of a monitor without discovery
func (h *MonitorHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input struct {
		DisplayName            string `json:"display_name"`
		Hostname               string `json:"hostname"`
		IPAddress              string `json:"ip_address"`
		PluginID               string `json:"plugin_id"`
		CredentialProfileID    string `json:"credential_profile_id"`
		DiscoveryProfileID     string `json:"discovery_profile_id"`
		PollingIntervalSeconds *int32 `json:"polling_interval_seconds"`
		Port                   *int32 `json:"port"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body", err)
		return
	}

	// Validate required fields
	if input.IPAddress == "" {
		sendError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "ip_address is required", nil)
		return
	}
	if input.PluginID == "" {
		sendError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "plugin_id is required", nil)
		return
	}
	if input.CredentialProfileID == "" {
		sendError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "credential_profile_id is required", nil)
		return
	}
	if input.DiscoveryProfileID == "" {
		sendError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "discovery_profile_id is required", nil)
		return
	}

	// Parse IP address
	ipAddr, err := database.StringToInet(input.IPAddress)
	if err != nil {
		sendError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid IP address format", err)
		return
	}

	// Parse credential profile ID
	credUUID, err := uuid.Parse(input.CredentialProfileID)
	if err != nil {
		sendError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid credential_profile_id format", err)
		return
	}

	// Parse discovery profile ID
	discUUID, err := uuid.Parse(input.DiscoveryProfileID)
	if err != nil {
		sendError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid discovery_profile_id format", err)
		return
	}

	// Build params - use defaults for display_name/hostname if not provided
	displayName := input.DisplayName
	if displayName == "" {
		displayName = input.IPAddress
	}
	hostname := input.Hostname
	if hostname == "" {
		hostname = input.IPAddress
	}

	params := dbgen.CreateMonitorParams{
		DisplayName:         pgtype.Text{String: displayName, Valid: true},
		Hostname:            pgtype.Text{String: hostname, Valid: true},
		IpAddress:           ipAddr,
		PluginID:            input.PluginID,
		CredentialProfileID: credUUID,
		DiscoveryProfileID:  discUUID,
		Port:                pgtype.Int4{Int32: 0, Valid: false},
		Column8:             nil, // Use SQL default (60)
		Column9:             nil, // Use SQL default ('active')
	}

	// Set port if provided
	if input.Port != nil && *input.Port > 0 {
		params.Port = pgtype.Int4{Int32: *input.Port, Valid: true}
	}

	// Set polling interval if provided
	if input.PollingIntervalSeconds != nil && *input.PollingIntervalSeconds > 0 {
		params.Column8 = *input.PollingIntervalSeconds
	}

	monitor, err := h.q.CreateMonitor(r.Context(), params)
	if err != nil {
		sendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Failed to create monitor", err)
		return
	}

	sendJSON(w, http.StatusCreated, monitor)
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
		if errors.Is(err, pgx.ErrNoRows) {
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
		Status                 string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body", err)
		return
	}

	// Get existing to merge
	existing, err := h.q.GetMonitor(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			sendError(w, r, http.StatusNotFound, "NOT_FOUND", "Monitor not found", nil)
			return
		}
		sendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Failed to fetch existing monitor", err)
		return
	}

	// Prepare params with defaults from existing
	params := dbgen.UpdateMonitorParams{
		ID:                     id,
		DisplayName:            existing.DisplayName,
		Hostname:               existing.Hostname,
		IpAddress:              existing.IpAddress, // Note: dbgen uses IpAddress (capital I)
		PluginID:               existing.PluginID,
		CredentialProfileID:    existing.CredentialProfileID,
		PollingIntervalSeconds: existing.PollingIntervalSeconds,
		Port:                   existing.Port,
		Status:                 existing.Status,
	}

	// Apply updates
	if input.DisplayName != "" {
		params.DisplayName = pgtype.Text{String: input.DisplayName, Valid: true}
	}
	if input.Hostname != "" {
		params.Hostname = pgtype.Text{String: input.Hostname, Valid: true}
	}
	if input.IPAddress != "" {
		ipInet, err := database.StringToInet(input.IPAddress)
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
			params.CredentialProfileID = id
		}
	}
	if input.PollingIntervalSeconds > 0 {
		params.PollingIntervalSeconds = pgtype.Int4{Int32: input.PollingIntervalSeconds, Valid: true}
	}
	if input.Port > 0 {
		params.Port = pgtype.Int4{Int32: input.Port, Valid: true}
	}
	if input.Status != "" {
		params.Status = pgtype.Text{String: input.Status, Valid: true}
	}

	monitor, err := h.q.UpdateMonitor(r.Context(), params)
	if err != nil {
		sendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Failed to update monitor", err)
		return
	}

	// Invalidate cache
	select {
	case h.events.CacheInvalidate <- channels.CacheInvalidateEvent{
		EntityType: "monitor",
		EntityID:   id,
		Timestamp:  time.Now(),
	}:
	default:
		slog.Warn("Failed to emit cache invalidation event", "entity_type", "monitor", "id", id)
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

	// Invalidate cache
	select {
	case h.events.CacheInvalidate <- channels.CacheInvalidateEvent{
		EntityType: "monitor",
		EntityID:   id,
		Timestamp:  time.Now(),
	}:
	default:
		slog.Warn("Failed to emit cache invalidation event", "entity_type", "monitor", "id", id)
	}

	sendJSON(w, http.StatusNoContent, nil)
}

// Restore handles PATCH /api/v1/monitors/{id}/restore
func (h *MonitorHandler) Restore(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement monitor restoration
	sendError(w, r, http.StatusNotImplemented, "NOT_IMPLEMENTED", "This endpoint is not yet implemented", nil)
}

// QueryMetrics handles POST /api/v1/metrics/query
// Accepts batch device_ids in request body, returns grouped results
func (h *MonitorHandler) QueryMetrics(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var req MetricsQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body", err)
		return
	}

	// Validate device_ids not empty
	if len(req.DeviceIDs) == 0 {
		sendError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "device_ids array cannot be empty", nil)
		return
	}

	// Validate monitors exist in a single query
	validDeviceIDs, err := h.validateMonitorIDs(r.Context(), req.DeviceIDs)
	if err != nil {
		sendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Failed to validate device IDs", err)
		return
	}

	// If no valid devices, return empty result with all requested IDs showing empty arrays
	if len(validDeviceIDs) == 0 {
		emptyData := make(map[string][]MetricRow)
		for _, id := range req.DeviceIDs {
			emptyData[id.String()] = []MetricRow{}
		}
		response := &BatchMetricsQueryResponse{
			Data:  emptyData,
			Count: 0,
		}
		sendJSON(w, http.StatusOK, response)
		return
	}

	// Execute query with valid device IDs
	response, err := ExecuteMetricsQuery(r.Context(), h.pool, validDeviceIDs, req)
	if err != nil {
		sendError(w, r, http.StatusInternalServerError, "QUERY_ERROR", "Failed to query metrics", err)
		return
	}

	sendJSON(w, http.StatusOK, response)
}

// validateMonitorIDs validates monitor IDs exist in a single DB query
// Returns only the IDs that exist and are not soft-deleted
func (h *MonitorHandler) validateMonitorIDs(ctx context.Context, ids []uuid.UUID) ([]uuid.UUID, error) {
	return h.q.GetExistingMonitorIDs(ctx, ids)
}
