package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nmslite/nmslite/internal/auth"
	"github.com/nmslite/nmslite/internal/database/db_gen"
)

// CredentialHandler handles credential profile endpoints
type CredentialHandler struct {
	q           db_gen.Querier
	authService *auth.Service
}

// NewCredentialHandler creates a new credential handler
func NewCredentialHandler(q db_gen.Querier, authService *auth.Service) *CredentialHandler {
	return &CredentialHandler{
		q:           q,
		authService: authService,
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

	sendJSON(w, http.StatusOK, map[string]interface{}{
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

	// Validate input (basic)
	if input.Name == "" || input.Protocol == "" {
		sendError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Name and Protocol are required", nil)
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

	params := db_gen.CreateCredentialProfileParams{
		Name:           input.Name,
		Description:    sql.NullString{String: input.Description, Valid: input.Description != ""},
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
		if err == sql.ErrNoRows {
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

	// Encrypt credential data
	encryptedStr, err := h.authService.Encrypt(input.CredentialData)
	if err != nil {
		sendError(w, r, http.StatusInternalServerError, "ENCRYPTION_ERROR", "Failed to encrypt credentials", err)
		return
	}

	// Wrap as JSON string
	encryptedJSON := json.RawMessage(fmt.Sprintf("%q", encryptedStr))

	params := db_gen.UpdateCredentialProfileParams{
		ID:             id,
		Name:           input.Name,
		Description:    sql.NullString{String: input.Description, Valid: input.Description != ""},
		Protocol:       input.Protocol,
		CredentialData: encryptedJSON,
	}

	profile, err := h.q.UpdateCredentialProfile(r.Context(), params)
	if err != nil {
		if err == sql.ErrNoRows {
			sendError(w, r, http.StatusNotFound, "NOT_FOUND", "Credential profile not found", nil)
			return
		}
		sendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Failed to update credential profile", err)
		return
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

	sendJSON(w, http.StatusNoContent, nil)
}
