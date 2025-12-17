package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nmslite/nmslite/internal/api/common"
	"github.com/nmslite/nmslite/internal/database/dbgen"
	"github.com/nmslite/nmslite/internal/globals"
)

// DiscoveryHandler handles discovery profile endpoints
type DiscoveryHandler struct {
	Deps *common.Dependencies
}

func NewDiscoveryHandler(deps *common.Dependencies) *DiscoveryHandler {
	return &DiscoveryHandler{
		Deps: deps,
	}
}

// List handles GET requests
func (h *DiscoveryHandler) List(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.Deps.Q.ListDiscoveryProfiles(r.Context())
	if common.HandleDBError(w, r, err, "Discovery Profile") {
		return
	}
	for i := range profiles {
		if decrypted, err := h.Deps.Decrypt(profiles[i].TargetValue); err == nil {
			profiles[i].TargetValue = string(decrypted)
		}
	}
	common.SendListResponse(w, profiles, len(profiles))
}

// Create handles POST requests
func (h *DiscoveryHandler) Create(w http.ResponseWriter, r *http.Request) {
	input, ok := common.DecodeJSON[dbgen.DiscoveryProfile](w, r)
	if !ok {
		return
	}

	if input.Name == "" || input.TargetValue == "" {
		common.SendError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Name and TargetValue are required", nil)
		return
	}

	encrypted, err := h.Deps.Encrypt([]byte(input.TargetValue))
	if err != nil {
		common.SendError(w, r, http.StatusInternalServerError, "ENCRYPTION_ERROR", "Failed to encrypt target value", err)
		return
	}

	params := dbgen.CreateDiscoveryProfileParams{
		Name:                input.Name,
		TargetValue:         encrypted,
		Port:                input.Port,
		PortScanTimeoutMs:   input.PortScanTimeoutMs,
		CredentialProfileID: input.CredentialProfileID,
		AutoProvision:       input.AutoProvision,
		AutoRun:             input.AutoRun,
	}

	profile, err := h.Deps.Q.CreateDiscoveryProfile(r.Context(), params)
	if common.HandleDBError(w, r, err, "Discovery Profile") {
		return
	}

	if input.AutoRun.Bool {
		triggerDiscovery(r.Context(), h.Deps, profile.ID)
	}

	common.SendJSON(w, http.StatusCreated, profile)
}

// Get handles GET /{id} requests
func (h *DiscoveryHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := common.ParseIDParam(w, r, "id")
	if !ok {
		return
	}

	profile, err := h.Deps.Q.GetDiscoveryProfile(r.Context(), id)
	if common.HandleDBError(w, r, err, "Discovery Profile") {
		return
	}

	if decrypted, err := h.Deps.Decrypt(profile.TargetValue); err == nil {
		profile.TargetValue = string(decrypted)
	}

	common.SendJSON(w, http.StatusOK, profile)
}

// Update handles PUT/PATCH /{id} requests
func (h *DiscoveryHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := common.ParseIDParam(w, r, "id")
	if !ok {
		return
	}

	input, ok := common.DecodeJSON[dbgen.DiscoveryProfile](w, r)
	if !ok {
		return
	}

	// We need to re-encrypt if target value is provided (assuming full update or check logic)
	// But generically input struct might not distinguish unset vs empty string if we rely on DecodeJSON.
	// For simplicity, we assume frontend sends full object or we fetch and merge.
	// The original handler didn't fetch-merge explicitly in UpdateFunc, let's see.
	// It just did: encrypted, err := h.Deps.Encrypt([]byte(input.TargetValue))
	// If input.TargetValue is empty, it encrypts empty byte slice. Ideally, we should fetch existing if we want partial updates,
	// but the previous code didn't look like it did fetch-merge in UpdateFunc?
	// Ah, wait. The previous `update` func:
	// func (h *DiscoveryHandler) update(...) { encrypted... h.Deps.Q.UpdateDiscoveryProfile(...) }
	// It relies on input having all fields or at least TargetValue.
	// Let's stick to previous logic: Encrypt input.TargetValue.

	encrypted, err := h.Deps.Encrypt([]byte(input.TargetValue))
	if err != nil {
		common.SendError(w, r, http.StatusInternalServerError, "ENCRYPTION_ERROR", "Failed to encrypt target value", err)
		return
	}

	params := dbgen.UpdateDiscoveryProfileParams{
		ID:                  id,
		Name:                input.Name,
		TargetValue:         encrypted,
		Port:                input.Port,
		PortScanTimeoutMs:   input.PortScanTimeoutMs,
		CredentialProfileID: input.CredentialProfileID,
		AutoProvision:       input.AutoProvision,
		AutoRun:             input.AutoRun,
	}

	profile, err := h.Deps.Q.UpdateDiscoveryProfile(r.Context(), params)
	if common.HandleDBError(w, r, err, "Discovery Profile") {
		return
	}

	common.SendJSON(w, http.StatusOK, profile)
}

// Delete handles DELETE /{id} requests
func (h *DiscoveryHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := common.ParseIDParam(w, r, "id")
	if !ok {
		return
	}

	err := h.Deps.Q.DeleteDiscoveryProfile(r.Context(), id)
	if common.HandleDBError(w, r, err, "Discovery Profile") {
		return
	}

	common.SendJSON(w, http.StatusNoContent, nil)
}

func triggerDiscovery(ctx context.Context, deps *common.Dependencies, id int64) {
	if deps.Events == nil {
		return
	}
	select {
	case deps.Events.DiscoveryRequest <- globals.DiscoveryRequestEvent{
		ProfileID: id,
		StartedAt: time.Now(),
	}:
	case <-ctx.Done():
	case <-deps.Events.Done():
	default:
		// Log full?
	}
}

// Run handles POST /api/v1/discoveries/{id}/run
func (h *DiscoveryHandler) Run(w http.ResponseWriter, r *http.Request) {
	id, ok := common.ParseIDParam(w, r, "id")
	if !ok {
		return
	}

	// Validate existence
	_, err := h.Deps.Q.GetDiscoveryProfile(r.Context(), id)
	if common.HandleDBError(w, r, err, "Discovery profile") {
		return
	}

	triggerDiscovery(r.Context(), h.Deps, id)

	common.SendJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":     "accepted",
		"message":    "Discovery started",
		"profile_id": strconv.FormatInt(id, 10),
	})
}

// GetResults handles GET /api/v1/discoveries/{id}/results
func (h *DiscoveryHandler) GetResults(w http.ResponseWriter, r *http.Request) {
	id, ok := common.ParseIDParam(w, r, "id")
	if !ok {
		return
	}

	results, err := h.Deps.Q.ListDiscoveredDevices(r.Context(), pgtype.Int8{Int64: id, Valid: true})
	if common.HandleDBError(w, r, err, "Discovery results") {
		return
	}

	common.SendListResponse(w, results, len(results))
}
