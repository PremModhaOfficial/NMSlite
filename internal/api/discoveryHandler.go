package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nmslite/nmslite/internal/auth"
	"github.com/nmslite/nmslite/internal/channels"
	"github.com/nmslite/nmslite/internal/database/dbgen"
)

// DiscoveryHandler handles discovery profile endpoints
type DiscoveryHandler struct {
	q           dbgen.Querier
	authService *auth.Service
	events      *channels.EventChannels
}

// NewDiscoveryHandler creates a new discovery handler
func NewDiscoveryHandler(q dbgen.Querier, authService *auth.Service, events *channels.EventChannels) *DiscoveryHandler {
	return &DiscoveryHandler{
		q:           q,
		authService: authService,
		events:      events,
	}
}

// List handles GET /api/v1/discoveries
func (h *DiscoveryHandler) List(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.q.ListDiscoveryProfiles(r.Context())
	if handleDBError(w, r, err, "Discovery profiles") {
		return
	}

	// Decrypt target_value for all profiles
	for i := range profiles {
		// Attempt to decrypt; if it fails (e.g., legacy unencrypted), keep original
		if decrypted, err := h.authService.Decrypt(profiles[i].TargetValue); err == nil {
			profiles[i].TargetValue = string(decrypted)
		}
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"data":  profiles,
		"total": len(profiles),
	})
}

// Create handles POST /api/v1/discoveries
func (h *DiscoveryHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name                string `json:"name"`
		TargetValue         string `json:"target_value"`
		Port                int32  `json:"port"`
		PortScanTimeoutMs   int32  `json:"port_scan_timeout_ms"`
		CredentialProfileID string `json:"credential_profile_id"`
		AutoProvision       bool   `json:"auto_provision"`
		AutoRun             bool   `json:"auto_run"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body", err)
		return
	}

	if input.Name == "" || input.TargetValue == "" {
		sendError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Name and TargetValue are required", nil)
		return
	}

	// Encrypt TargetValue
	encryptedTarget, err := h.authService.Encrypt([]byte(input.TargetValue))
	if err != nil {
		sendError(w, r, http.StatusInternalServerError, "ENCRYPTION_ERROR", "Failed to encrypt target value", err)
		return
	}

	// Parse credential profile ID
	credID, err := uuid.Parse(input.CredentialProfileID)
	if err != nil {
		sendError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid credential_profile_id format", err)
		return
	}

	params := dbgen.CreateDiscoveryProfileParams{
		Name:                input.Name,
		TargetValue:         encryptedTarget,
		Port:                input.Port,
		PortScanTimeoutMs:   pgtype.Int4{Int32: input.PortScanTimeoutMs, Valid: input.PortScanTimeoutMs > 0},
		CredentialProfileID: credID,
		AutoProvision:       pgtype.Bool{Bool: input.AutoProvision, Valid: true},
		AutoRun:             pgtype.Bool{Bool: input.AutoRun, Valid: true},
	}

	profile, err := h.q.CreateDiscoveryProfile(r.Context(), params)
	if err != nil {
		sendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Failed to create discovery profile", err)
		return
	}

	// Return unencrypted value in response
	profile.TargetValue = input.TargetValue

	// If auto_run is enabled, trigger discovery immediately
	if input.AutoRun {
		startedEvent := channels.DiscoveryRequestEvent{
			ProfileID: profile.ID,
			StartedAt: time.Now(),
		}

		// Non-blocking send to trigger discovery
		select {
		case h.events.DiscoveryRequest <- startedEvent:
			// Event sent successfully
		case <-r.Context().Done():
			// Context cancelled, but profile is already created
		default:
			// Channel full - log but continue (profile created successfully)
		}
	}

	sendJSON(w, http.StatusCreated, profile)
}

// Get handles GET /api/v1/discoveries/{id}
func (h *DiscoveryHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	profile, err := h.q.GetDiscoveryProfile(r.Context(), id)
	if handleDBError(w, r, err, "Discovery profile") {
		return
	}

	// Decrypt target_value
	if decrypted, err := h.authService.Decrypt(profile.TargetValue); err == nil {
		profile.TargetValue = string(decrypted)
	}

	sendJSON(w, http.StatusOK, profile)
}

// Update handles PUT /api/v1/discoveries/{id}
func (h *DiscoveryHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var input struct {
		Name                string `json:"name"`
		TargetValue         string `json:"target_value"`
		Port                int32  `json:"port"`
		PortScanTimeoutMs   int32  `json:"port_scan_timeout_ms"`
		CredentialProfileID string `json:"credential_profile_id"`
		AutoProvision       bool   `json:"auto_provision"`
		AutoRun             bool   `json:"auto_run"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body", err)
		return
	}

	// Encrypt TargetValue
	encryptedTarget, err := h.authService.Encrypt([]byte(input.TargetValue))
	if err != nil {
		sendError(w, r, http.StatusInternalServerError, "ENCRYPTION_ERROR", "Failed to encrypt target value", err)
		return
	}

	// Parse credential profile ID
	credID, err := uuid.Parse(input.CredentialProfileID)
	if err != nil {
		sendError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Invalid credential_profile_id format", err)
		return
	}

	params := dbgen.UpdateDiscoveryProfileParams{
		ID:                  id,
		Name:                input.Name,
		TargetValue:         encryptedTarget,
		Port:                input.Port,
		PortScanTimeoutMs:   pgtype.Int4{Int32: input.PortScanTimeoutMs, Valid: input.PortScanTimeoutMs > 0},
		CredentialProfileID: credID,
		AutoProvision:       pgtype.Bool{Bool: input.AutoProvision, Valid: true},
		AutoRun:             pgtype.Bool{Bool: input.AutoRun, Valid: true},
	}

	profile, err := h.q.UpdateDiscoveryProfile(r.Context(), params)
	if handleDBError(w, r, err, "Discovery profile") {
		return
	}

	// Return unencrypted value in response
	profile.TargetValue = input.TargetValue

	sendJSON(w, http.StatusOK, profile)
}

// Delete handles DELETE /api/v1/discoveries/{id}
func (h *DiscoveryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	err := h.q.DeleteDiscoveryProfile(r.Context(), id)
	if handleDBError(w, r, err, "Discovery profile") {
		return
	}

	sendJSON(w, http.StatusNoContent, nil)
}

// Run handles POST /api/v1/discoveries/{id}/run
// Returns 202 Accepted immediately - discovery runs asynchronously
func (h *DiscoveryHandler) Run(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	// 1. Fetch Profile to validate it exists
	profile, err := h.q.GetDiscoveryProfile(r.Context(), id)
	if handleDBError(w, r, err, "Discovery profile") {
		return
	}

	// 2. Publish discovery started event to typed channel
	startedEvent := channels.DiscoveryRequestEvent{
		ProfileID: id,
		StartedAt: time.Now(),
	}

	// Non-blocking send with context
	select {
	case h.events.DiscoveryRequest <- startedEvent:
		// Event sent successfully
	case <-r.Context().Done():
		sendError(w, r, http.StatusInternalServerError, "EVENT_PUBLISH_ERROR", "Context cancelled while publishing event", r.Context().Err())
		return
	default:
		// Channel full - log warning but continue (matches old non-blocking behavior)
		// In production, consider returning an error or using a timeout
	}

	// 3. Return 202 Accepted immediately
	// Suppress unused profile warning by using it
	_ = profile
	sendJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":     "accepted",
		"message":    "Discovery started",
		"profile_id": id.String(),
	})
}

// GetResults handles GET /api/v1/discoveries/{id}/results
func (h *DiscoveryHandler) GetResults(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	results, err := h.q.ListDiscoveredDevices(r.Context(), uuid.NullUUID{UUID: id, Valid: true})
	if handleDBError(w, r, err, "Discovery results") {
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"data":  results,
		"total": len(results),
	})
}
