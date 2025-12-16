package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/nmslite/nmslite/internal/api/common"
	"github.com/nmslite/nmslite/internal/database/dbgen"
	"github.com/nmslite/nmslite/internal/globals"
	"github.com/nmslite/nmslite/internal/protocols"
)

// CredentialHandler handles credential profile endpoints
type CredentialHandler struct {
	Deps *common.Dependencies
}

func NewCredentialHandler(deps *common.Dependencies) *CredentialHandler {
	return &CredentialHandler{
		Deps: deps,
	}
}

// List handles GET requests
func (h *CredentialHandler) List(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.Deps.Q.ListCredentialProfiles(r.Context())
	if common.HandleDBError(w, r, err, "Credential Profile") {
		return
	}
	// Decrypt
	for i := range profiles {
		var encryptedStr string
		if err := json.Unmarshal(profiles[i].Payload, &encryptedStr); err == nil {
			if decrypted, err := h.Deps.Decrypt(encryptedStr); err == nil {
				profiles[i].Payload = decrypted
			}
		}
	}
	common.SendListResponse(w, profiles, len(profiles))
}

// Create handles POST requests
func (h *CredentialHandler) Create(w http.ResponseWriter, r *http.Request) {
	input, ok := common.DecodeJSON[dbgen.CredentialProfile](w, r)
	if !ok {
		return
	}

	if input.Name == "" || input.Protocol == "" {
		common.SendError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Name and Protocol are required", nil)
		return
	}
	if err := validateCredentials(h.Deps.Registry, input.Protocol, input.Payload); err != nil {
		common.SendError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}

	// Encrypt
	encrypted, err := h.Deps.Encrypt(input.Payload)
	if err != nil {
		common.SendError(w, r, http.StatusInternalServerError, "ENCRYPTION_ERROR", "Failed to encrypt payload", err)
		return
	}
	// Wrap as JSON string for storage
	encryptedJSON := json.RawMessage(fmt.Sprintf("%q", encrypted))

	params := dbgen.CreateCredentialProfileParams{
		Name:        input.Name,
		Description: input.Description,
		Protocol:    input.Protocol,
		Payload:     encryptedJSON,
	}
	profile, err := h.Deps.Q.CreateCredentialProfile(r.Context(), params)
	if common.HandleDBError(w, r, err, "Credential Profile") {
		return
	}

	common.SendJSON(w, http.StatusCreated, profile)
}

// Get handles GET /{id} requests
func (h *CredentialHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := common.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	profile, err := h.Deps.Q.GetCredentialProfile(r.Context(), id)
	if common.HandleDBError(w, r, err, "Credential Profile") {
		return
	}
	// Decrypt
	var encryptedStr string
	if err := json.Unmarshal(profile.Payload, &encryptedStr); err == nil {
		if decrypted, err := h.Deps.Decrypt(encryptedStr); err == nil {
			profile.Payload = decrypted
		}
	}
	common.SendJSON(w, http.StatusOK, profile)
}

// Update handles PUT/PATCH /{id} requests
func (h *CredentialHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := common.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	input, ok := common.DecodeJSON[dbgen.CredentialProfile](w, r)
	if !ok {
		return
	}

	// Validate if protocol/data provided
	if input.Protocol != "" && len(input.Payload) > 0 {
		if err := validateCredentials(h.Deps.Registry, input.Protocol, input.Payload); err != nil {
			common.SendError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
			return
		}
	}

	// Encrypt if present
	if len(input.Payload) > 0 {
		encrypted, err := h.Deps.Encrypt(input.Payload)
		if err != nil {
			common.SendError(w, r, http.StatusInternalServerError, "ENCRYPTION_ERROR", "Failed to encrypt payload", err)
			return
		}
		input.Payload = json.RawMessage(fmt.Sprintf("%q", encrypted))
	}

	params := dbgen.UpdateCredentialProfileParams{
		ID:          id,
		Name:        input.Name,
		Description: input.Description,
		Protocol:    input.Protocol,
		Payload:     input.Payload,
	}
	profile, err := h.Deps.Q.UpdateCredentialProfile(r.Context(), params)
	if common.HandleDBError(w, r, err, "Credential Profile") {
		return
	}

	// Push update to monitors
	h.pushUpdate(r.Context(), id)

	common.SendJSON(w, http.StatusOK, profile)
}

// Delete handles DELETE /{id} requests
func (h *CredentialHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := common.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	err := h.Deps.Q.DeleteCredentialProfile(r.Context(), id)
	if common.HandleDBError(w, r, err, "Credential Profile") {
		return
	}

	common.SendJSON(w, http.StatusNoContent, nil)
}

// pushUpdate fetches all monitors using this credential profile and pushes them to scheduler
func (h *CredentialHandler) pushUpdate(ctx context.Context, credentialID uuid.UUID) {
	if h.Deps.Events == nil {
		return
	}

	monitors, err := h.Deps.Q.GetMonitorsWithCredentialsByCredentialID(ctx, credentialID)
	if err != nil {
		if h.Deps.Logger != nil {
			h.Deps.Logger.Error("failed to fetch monitors for credential cache push", "credential_id", credentialID, "error", err)
		}
		return
	}

	if len(monitors) == 0 {
		return
	}

	var updates []dbgen.GetMonitorWithCredentialsRow
	for _, m := range monitors {
		updates = append(updates, dbgen.GetMonitorWithCredentialsRow{
			ID:                     m.ID,
			DisplayName:            m.DisplayName,
			Hostname:               m.Hostname,
			IpAddress:              m.IpAddress,
			PluginID:               m.PluginID,
			CredentialProfileID:    m.CredentialProfileID,
			DiscoveryProfileID:     m.DiscoveryProfileID,
			Port:                   m.Port,
			PollingIntervalSeconds: m.PollingIntervalSeconds,
			Status:                 m.Status,
			CreatedAt:              m.CreatedAt,
			UpdatedAt:              m.UpdatedAt,
			Payload:                m.Payload,
		})
	}

	h.Deps.Events.CacheInvalidate <- globals.CacheInvalidateEvent{
		UpdateType: "update",
		Monitors:   updates,
	}
}

func validateCredentials(registry *protocols.Registry, protocol string, data json.RawMessage) error {
	_, err := registry.ValidateCredentials(protocol, data)
	return err
}
