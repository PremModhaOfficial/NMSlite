package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nmslite/nmslite/internal/auth"
	"github.com/nmslite/nmslite/internal/channels"
	"github.com/nmslite/nmslite/internal/database/dbgen"
	"github.com/nmslite/nmslite/internal/protocols"
)

// CredentialHandler handles credential profile endpoints
type CredentialHandler struct {
	pool        *pgxpool.Pool
	q           dbgen.Querier
	authService *auth.Service
	registry    *protocols.Registry
	events      *channels.EventChannels
}

// NewCredentialHandler creates a new credential handler
func NewCredentialHandler(pool *pgxpool.Pool, authService *auth.Service, events *channels.EventChannels) *CredentialHandler {
	return &CredentialHandler{
		pool:        pool,
		q:           dbgen.New(pool),
		authService: authService,
		registry:    protocols.GetRegistry(),
		events:      events,
	}
}

// List handles GET /api/v1/credentials
func (h *CredentialHandler) List(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.q.ListCredentialProfiles(r.Context())
	if err != nil {
		sendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Failed to list credentials", err)
		return
	}

	// Decrypt credential data for all profiles
	for i := range profiles {
		var encryptedStr string
		// Try to unmarshal as string (encrypted format)
		if err := json.Unmarshal(profiles[i].CredentialData, &encryptedStr); err == nil {
			// If it's a string, try to decrypt
			if decrypted, err := h.authService.Decrypt(encryptedStr); err == nil {
				profiles[i].CredentialData = decrypted
			}
		}
		// If unmarshal fails or decrypt fails, we leave it as is (might be legacy unencrypted data)
	}

	sendJSON(w, http.StatusOK, map[string]any{
		"data":  profiles,
		"total": len(profiles),
	})
}

// Create handles POST /api/v1/credentials
func (h *CredentialHandler) Create(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name           string          `json:"name"`
		Description    string          `json:"description"`
		Protocol       string          `json:"protocol"`
		CredentialData json.RawMessage `json:"credential_data"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body", err)
		return
	}

	// Validate basic required fields
	if input.Name == "" || input.Protocol == "" {
		sendError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Name and Protocol are required", nil)
		return
	}

	// Validate credentials against protocol schema using struct-based validation
	_, err := h.registry.ValidateCredentials(input.Protocol, input.CredentialData)
	if err != nil {
		// Check if it's a ValidationErrors type for detailed response
		var validationErrs *protocols.ValidationErrors
		if errors.As(err, &validationErrs) {
			sendJSON(w, http.StatusBadRequest, map[string]any{
				"error": map[string]any{
					"code":    "VALIDATION_ERROR",
					"message": "Credential validation failed",
					"details": validationErrs.Errors,
				},
			})
			return
		}
		sendError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
		return
	}

	// Encrypt credential data
	encryptedStr, err := h.authService.Encrypt(input.CredentialData)
	if err != nil {
		sendError(w, r, http.StatusInternalServerError, "ENCRYPTION_ERROR", "Failed to encrypt credentials", err)
		return
	}

	// Wrap as JSON string
	encryptedJSON := json.RawMessage(fmt.Sprintf("%q", encryptedStr))

	params := dbgen.CreateCredentialProfileParams{
		Name:           input.Name,
		Description:    pgtype.Text{String: input.Description, Valid: input.Description != ""},
		Protocol:       input.Protocol,
		CredentialData: encryptedJSON,
	}

	profile, err := h.q.CreateCredentialProfile(r.Context(), params)
	if err != nil {
		sendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Failed to create credential profile", err)
		return
	}

	sendJSON(w, http.StatusCreated, profile)
}

// Get handles GET /api/v1/credentials/{id}
func (h *CredentialHandler) Get(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid UUID format", err)
		return
	}

	profile, err := h.q.GetCredentialProfile(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			sendError(w, r, http.StatusNotFound, "NOT_FOUND", "Credential profile not found", nil)
			return
		}
		sendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Failed to get credential profile", err)
		return
	}

	// Decrypt credential data
	var encryptedStr string
	if err := json.Unmarshal(profile.CredentialData, &encryptedStr); err == nil {
		if decrypted, err := h.authService.Decrypt(encryptedStr); err == nil {
			profile.CredentialData = decrypted
		}
	}

	sendJSON(w, http.StatusOK, profile)
}

// Update handles PUT /api/v1/credentials/{id}
func (h *CredentialHandler) Update(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid UUID format", err)
		return
	}

	var input struct {
		Name           string          `json:"name"`
		Description    string          `json:"description"`
		Protocol       string          `json:"protocol"`
		CredentialData json.RawMessage `json:"credential_data"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body", err)
		return
	}

	// Validate credentials against protocol schema using struct-based validation
	if input.Protocol != "" && len(input.CredentialData) > 0 {
		_, err := h.registry.ValidateCredentials(input.Protocol, input.CredentialData)
		if err != nil {
			// Check if it's a ValidationErrors type for detailed response
			var validationErrs *protocols.ValidationErrors
			if errors.As(err, &validationErrs) {
				sendJSON(w, http.StatusBadRequest, map[string]any{
					"error": map[string]any{
						"code":    "VALIDATION_ERROR",
						"message": "Credential validation failed",
						"details": validationErrs.Errors,
					},
				})
				return
			}
			sendError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), nil)
			return
		}
	}

	// Encrypt credential data
	encryptedStr, err := h.authService.Encrypt(input.CredentialData)
	if err != nil {
		sendError(w, r, http.StatusInternalServerError, "ENCRYPTION_ERROR", "Failed to encrypt credentials", err)
		return
	}

	// Wrap as JSON string
	encryptedJSON := json.RawMessage(fmt.Sprintf("%q", encryptedStr))

	params := dbgen.UpdateCredentialProfileParams{
		ID:             id,
		Name:           input.Name,
		Description:    pgtype.Text{String: input.Description, Valid: input.Description != ""},
		Protocol:       input.Protocol,
		CredentialData: encryptedJSON,
	}

	profile, err := h.q.UpdateCredentialProfile(r.Context(), params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			sendError(w, r, http.StatusNotFound, "NOT_FOUND", "Credential profile not found", nil)
			return
		}
		sendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Failed to update credential profile", err)
		return
	}

	// Invalidate cache
	select {
	case h.events.CacheInvalidate <- channels.CacheInvalidateEvent{
		EntityType: "credential",
		EntityID:   id,
		Timestamp:  time.Now(),
	}:
	default:
		slog.Warn("Failed to emit cache invalidation event", "entity_type", "credential", "id", id)
	}

	sendJSON(w, http.StatusOK, profile)
}

// Delete handles DELETE /api/v1/credentials/{id}
func (h *CredentialHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid UUID format", err)
		return
	}

	err = h.q.DeleteCredentialProfile(r.Context(), id)
	if err != nil {
		sendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Failed to delete credential profile", err)
		return
	}

	// Invalidate cache
	select {
	case h.events.CacheInvalidate <- channels.CacheInvalidateEvent{
		EntityType: "credential",
		EntityID:   id,
		Timestamp:  time.Now(),
	}:
	default:
		slog.Warn("Failed to emit cache invalidation event", "entity_type", "credential", "id", id)
	}

	sendJSON(w, http.StatusNoContent, nil)
}
