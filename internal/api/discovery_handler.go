package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nmslite/nmslite/internal/auth"
	"github.com/nmslite/nmslite/internal/channels"
	"github.com/nmslite/nmslite/internal/database/db_gen"
)

// DiscoveryHandler handles discovery profile endpoints
type DiscoveryHandler struct {
	pool        *pgxpool.Pool
	q           db_gen.Querier
	authService *auth.Service
	events      *channels.EventChannels
}

// NewDiscoveryHandler creates a new discovery handler
func NewDiscoveryHandler(pool *pgxpool.Pool, authService *auth.Service, events *channels.EventChannels) *DiscoveryHandler {
	return &DiscoveryHandler{
		pool:        pool,
		q:           db_gen.New(pool),
		authService: authService,
		events:      events,
	}
}

// List handles GET /api/v1/discoveries
func (h *DiscoveryHandler) List(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.q.ListDiscoveryProfiles(r.Context())
	if err != nil {
		sendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Failed to list discovery profiles", err)
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
		Name                 string          `json:"name"`
		TargetValue          string          `json:"target_value"`
		Ports                json.RawMessage `json:"ports"`
		PortScanTimeoutMs    int32           `json:"port_scan_timeout_ms"`
		CredentialProfileIDs json.RawMessage `json:"credential_profile_ids"`
		AutoProvision        bool            `json:"auto_provision"`
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

	params := db_gen.CreateDiscoveryProfileParams{
		Name:                 input.Name,
		TargetValue:          encryptedTarget,
		Ports:                input.Ports,
		PortScanTimeoutMs:    pgtype.Int4{Int32: input.PortScanTimeoutMs, Valid: input.PortScanTimeoutMs > 0},
		CredentialProfileIds: input.CredentialProfileIDs,
		AutoProvision:        pgtype.Bool{Bool: input.AutoProvision, Valid: true},
	}

	profile, err := h.q.CreateDiscoveryProfile(r.Context(), params)
	if err != nil {
		sendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Failed to create discovery profile", err)
		return
	}

	// Return unencrypted value in response
	profile.TargetValue = input.TargetValue

	sendJSON(w, http.StatusCreated, profile)
}

// Get handles GET /api/v1/discoveries/{id}
func (h *DiscoveryHandler) Get(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid UUID format", err)
		return
	}

	profile, err := h.q.GetDiscoveryProfile(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			sendError(w, r, http.StatusNotFound, "NOT_FOUND", "Discovery profile not found", nil)
			return
		}
		sendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Failed to get discovery profile", err)
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
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid UUID format", err)
		return
	}

	var input struct {
		Name                 string          `json:"name"`
		TargetValue          string          `json:"target_value"`
		Ports                json.RawMessage `json:"ports"`
		PortScanTimeoutMs    int32           `json:"port_scan_timeout_ms"`
		CredentialProfileIDs json.RawMessage `json:"credential_profile_ids"`
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

	params := db_gen.UpdateDiscoveryProfileParams{
		ID:                   id,
		Name:                 input.Name,
		TargetValue:          encryptedTarget,
		Ports:                input.Ports,
		PortScanTimeoutMs:    pgtype.Int4{Int32: input.PortScanTimeoutMs, Valid: input.PortScanTimeoutMs > 0},
		CredentialProfileIds: input.CredentialProfileIDs,
	}

	profile, err := h.q.UpdateDiscoveryProfile(r.Context(), params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			sendError(w, r, http.StatusNotFound, "NOT_FOUND", "Discovery profile not found", nil)
			return
		}
		sendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Failed to update discovery profile", err)
		return
	}

	// Return unencrypted value in response
	profile.TargetValue = input.TargetValue

	sendJSON(w, http.StatusOK, profile)
}

// Delete handles DELETE /api/v1/discoveries/{id}
func (h *DiscoveryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid UUID format", err)
		return
	}

	err = h.q.DeleteDiscoveryProfile(r.Context(), id)
	if err != nil {
		sendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Failed to delete discovery profile", err)
		return
	}

	sendJSON(w, http.StatusNoContent, nil)
}

// Run handles POST /api/v1/discoveries/{id}/run
// Returns 202 Accepted immediately - discovery runs asynchronously
func (h *DiscoveryHandler) Run(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid UUID format", err)
		return
	}

	// 1. Fetch Profile to validate it exists
	profile, err := h.q.GetDiscoveryProfile(r.Context(), id)
	if err != nil {
		sendError(w, r, http.StatusNotFound, "NOT_FOUND", "Discovery profile not found", err)
		return
	}

	// 2. Publish discovery started event to typed channel
	startedEvent := channels.DiscoveryStartedEvent{
		ProfileID: id,
		StartedAt: time.Now(),
	}

	// Non-blocking send with context
	select {
	case h.events.DiscoveryStarted <- startedEvent:
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
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid UUID format", err)
		return
	}

	results, err := h.q.ListDiscoveredDevices(r.Context(), uuid.NullUUID{UUID: id, Valid: true})
	if err != nil {
		sendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Failed to list discovery results", err)
		return
	}

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"data":  results,
		"total": len(results),
	})
}
