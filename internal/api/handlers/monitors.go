package handlers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nmslite/nmslite/internal/api/common"
	"github.com/nmslite/nmslite/internal/channels"
	"github.com/nmslite/nmslite/internal/database/dbgen"
)

type MonitorHandler struct {
	*common.CRUDHandler[dbgen.Monitor]
}

func NewMonitorHandler(deps *common.Dependencies) *MonitorHandler {
	h := &MonitorHandler{}

	h.CRUDHandler = &common.CRUDHandler[dbgen.Monitor]{
		Deps: deps,
		Name: "Monitor",
		// CacheType removed, manual handling below
	}

	// Wrap generic functions to add Push-Model Cache Invalidation
	h.ListFunc = h.list

	// Create: generic create -> fetch full data -> push event
	h.CreateFunc = func(ctx context.Context, input dbgen.Monitor) (dbgen.Monitor, error) {
		m, err := h.create(ctx, input)
		if err == nil {
			h.pushUpdate(ctx, m.ID)
		}
		return m, err
	}

	h.GetFunc = h.get

	// Update: generic update -> fetch full data -> push event
	h.UpdateFunc = func(ctx context.Context, id uuid.UUID, input dbgen.Monitor) (dbgen.Monitor, error) {
		m, err := h.update(ctx, id, input)
		if err == nil {
			h.pushUpdate(ctx, m.ID)
		}
		return m, err
	}

	// Delete: generic delete -> push delete event
	h.DeleteFunc = func(ctx context.Context, id uuid.UUID) error {
		err := h.delete(ctx, id)
		if err == nil {
			h.pushDelete(ctx, id)
		}
		return err
	}

	return h
}

// pushUpdate fetches the joined monitor data and sends it to the scheduler
func (h *MonitorHandler) pushUpdate(ctx context.Context, id uuid.UUID) {
	if h.Deps.Events == nil {
		return
	}
	row, err := h.Deps.Q.GetMonitorWithCredentials(ctx, id)
	if err != nil {
		// Log but don't fail the request?
		// The generic handler uses h.Deps.Logger usually, but CRUDHandler doesn't expose it directly as public field?
		// Deps is public.
		if h.Deps.Logger != nil {
			h.Deps.Logger.Error("failed to fetch monitor for cache push", "monitor_id", id, "error", err)
		}
		return
	}
	h.Deps.Events.CacheInvalidate <- channels.CacheInvalidateEvent{
		UpdateType: "update",
		Monitors:   []dbgen.GetMonitorWithCredentialsRow{row},
	}
}

// pushDelete sends a delete signal to the scheduler
func (h *MonitorHandler) pushDelete(ctx context.Context, id uuid.UUID) {
	if h.Deps.Events == nil {
		return
	}
	h.Deps.Events.CacheInvalidate <- channels.CacheInvalidateEvent{
		UpdateType: "delete",
		MonitorIDs: []uuid.UUID{id},
	}
}

func (h *MonitorHandler) list(ctx context.Context) ([]dbgen.Monitor, error) {
	return h.Deps.Q.ListMonitors(ctx)
}

func (h *MonitorHandler) create(ctx context.Context, input dbgen.Monitor) (dbgen.Monitor, error) {
	if err := h.validateMonitorInput(input); err != nil {
		return dbgen.Monitor{}, err
	}

	displayName := input.DisplayName
	if !displayName.Valid || displayName.String == "" {
		displayName = pgtype.Text{String: input.IpAddress.String(), Valid: true}
	}
	hostname := input.Hostname
	if !hostname.Valid || hostname.String == "" {
		hostname = pgtype.Text{String: input.IpAddress.String(), Valid: true}
	}

	params := dbgen.CreateMonitorParams{
		DisplayName:            displayName,
		Hostname:               hostname,
		IpAddress:              input.IpAddress,
		PluginID:               input.PluginID,
		CredentialProfileID:    input.CredentialProfileID,
		DiscoveryProfileID:     input.DiscoveryProfileID,
		Port:                   input.Port,
		PollingIntervalSeconds: input.PollingIntervalSeconds,
		Status:                 input.Status,
	}

	return h.Deps.Q.CreateMonitor(ctx, params)
}

func (h *MonitorHandler) validateMonitorInput(input dbgen.Monitor) error {
	if !input.IpAddress.IsValid() {
		return fmt.Errorf("ip_address is required and must be valid")
	}
	if input.PluginID == "" {
		return fmt.Errorf("plugin_id is required")
	}
	if input.CredentialProfileID == uuid.Nil {
		return fmt.Errorf("credential_profile_id is required")
	}
	if input.DiscoveryProfileID == uuid.Nil {
		return fmt.Errorf("discovery_profile_id is required")
	}
	return nil
}

func (h *MonitorHandler) get(ctx context.Context, id uuid.UUID) (dbgen.Monitor, error) {
	return h.Deps.Q.GetMonitor(ctx, id)
}

func (h *MonitorHandler) update(ctx context.Context, id uuid.UUID, input dbgen.Monitor) (dbgen.Monitor, error) {
	existing, err := h.Deps.Q.GetMonitor(ctx, id)
	if err != nil {
		return dbgen.Monitor{}, err
	}

	// Merge Logic: if input field is "Valid" (present in JSON), update it.
	params := dbgen.UpdateMonitorParams{
		ID:                     id,
		DisplayName:            existing.DisplayName,
		Hostname:               existing.Hostname,
		IpAddress:              existing.IpAddress,
		PluginID:               existing.PluginID,
		CredentialProfileID:    existing.CredentialProfileID,
		PollingIntervalSeconds: existing.PollingIntervalSeconds,
		Port:                   existing.Port,
		Status:                 existing.Status,
	}

	if input.DisplayName.Valid {
		params.DisplayName = input.DisplayName
	}
	if input.Hostname.Valid {
		params.Hostname = input.Hostname
	}
	// IpAddress is netip.Addr, not pgtype. It's a struct (not pointer).
	// netip.Addr{} is invalid. valid one returns IsValid() = true.
	// Input JSON string "" -> invalid Addr?
	// If user sends valid IP, we update.
	if input.IpAddress.IsValid() {
		params.IpAddress = input.IpAddress
	}
	if input.PluginID != "" {
		params.PluginID = input.PluginID
	}
	if input.CredentialProfileID != uuid.Nil {
		params.CredentialProfileID = input.CredentialProfileID
	}
	if input.PollingIntervalSeconds.Valid {
		params.PollingIntervalSeconds = input.PollingIntervalSeconds
	}
	if input.Port.Valid {
		params.Port = input.Port
	}
	if input.Status.Valid {
		params.Status = input.Status
	}

	return h.Deps.Q.UpdateMonitor(ctx, params)
}

func (h *MonitorHandler) delete(ctx context.Context, id uuid.UUID) error {
	return h.Deps.Q.DeleteMonitor(ctx, id)
}

// Metrics Query Logic

type MetricsQueryRequest struct {
	DeviceIDs []uuid.UUID `json:"device_ids"`
	Prefix    string      `json:"prefix,omitempty"`
	Start     time.Time   `json:"start"`
	End       time.Time   `json:"end"`
	Limit     int         `json:"limit,omitempty"`
	Latest    bool        `json:"latest,omitempty"`
}

type MetricsQueryResponse struct {
	Data  map[string]map[string]float64 `json:"data"`
	Count int                           `json:"count"`
	Query MetricsQueryRequest           `json:"query"`
}

func (h *MonitorHandler) QueryMetrics(w http.ResponseWriter, r *http.Request) {
	req, ok := common.DecodeJSON[MetricsQueryRequest](w, r)
	if !ok {
		return
	}

	if len(req.DeviceIDs) == 0 || req.Start.IsZero() || req.End.IsZero() {
		common.SendError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "device_ids, start, and end are required", nil)
		return
	}
	if req.Limit == 0 {
		req.Limit = 100
	}

	// Validate Device IDs
	validIDs, err := h.Deps.Q.GetExistingMonitorIDs(r.Context(), req.DeviceIDs)
	if err != nil {
		common.HandleDBError(w, r, err, "Device IDs")
		return
	}

	if len(validIDs) == 0 {
		emptyData := make(map[string]map[string]float64)
		for _, id := range req.DeviceIDs {
			emptyData[id.String()] = make(map[string]float64)
		}
		common.SendJSON(w, http.StatusOK, MetricsQueryResponse{Data: emptyData, Query: req})
		return
	}

	// Query
	prefix := "%"
	if req.Prefix != "" {
		prefix = req.Prefix + ".%"
	}

	var dbRows []dbgen.Metric
	if req.Latest {
		dbRows, err = h.Deps.Q.GetLatestMetricsByDeviceAndPrefix(r.Context(), dbgen.GetLatestMetricsByDeviceAndPrefixParams{
			DeviceIds:         validIDs,
			MetricNamePattern: prefix,
			StartTime:         pgtype.Timestamptz{Time: req.Start, Valid: true},
			EndTime:           pgtype.Timestamptz{Time: req.End, Valid: true},
		})
	} else {
		dbRows, err = h.Deps.Q.GetMetricsByDeviceAndPrefix(r.Context(), dbgen.GetMetricsByDeviceAndPrefixParams{
			DeviceIds:         validIDs,
			MetricNamePattern: prefix,
			StartTime:         pgtype.Timestamptz{Time: req.Start, Valid: true},
			EndTime:           pgtype.Timestamptz{Time: req.End, Valid: true},
			LimitCount:        int32(req.Limit),
		})
	}

	if common.HandleDBError(w, r, err, "Metrics") {
		return
	}

	// Group Data
	groupedData := make(map[string]map[string]float64)
	for _, id := range req.DeviceIDs {
		groupedData[id.String()] = make(map[string]float64)
	}

	count := 0
	for _, row := range dbRows {
		did := row.DeviceID.String()
		if _, exists := groupedData[did]; !exists {
			groupedData[did] = make(map[string]float64)
		}
		if _, exists := groupedData[did][row.Name]; !exists {
			groupedData[did][row.Name] = row.Value
			count++
		}
	}

	common.SendJSON(w, http.StatusOK, MetricsQueryResponse{
		Data:  groupedData,
		Count: count,
		Query: req,
	})
}
