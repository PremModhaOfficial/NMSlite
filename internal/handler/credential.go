package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/nmslite/nmslite/internal/model"
	"github.com/nmslite/nmslite/internal/store"
)

// CredentialHandler handles credential endpoints
type CredentialHandler struct {
	store *store.MockStore
}

// NewCredentialHandler creates a new credential handler
func NewCredentialHandler(s *store.MockStore) *CredentialHandler {
	return &CredentialHandler{store: s}
}

// ListCredentials lists all credentials
func (h *CredentialHandler) ListCredentials(w http.ResponseWriter, r *http.Request) {
	creds := h.store.ListCredentials()
	respondSuccess(w, http.StatusOK, creds)
}

// CreateCredential creates a new credential
func (h *CredentialHandler) CreateCredential(w http.ResponseWriter, r *http.Request) {
	var req CreateCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	// Validate required fields
	if req.Name == "" || req.Username == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Name, username, and password are required")
		return
	}

	if req.Port == 0 {
		if req.UseSSL {
			req.Port = 5986
		} else {
			req.Port = 5985
		}
	}

	cred := &model.Credential{
		Name:           req.Name,
		CredentialType: req.CredentialType,
		Username:       req.Username,
		Domain:         req.Domain,
		Port:           req.Port,
		UseSSL:         req.UseSSL,
	}

	cred = h.store.CreateCredential(cred)
	respondSuccess(w, http.StatusCreated, cred)
}

// GetCredential retrieves a credential by ID
func (h *CredentialHandler) GetCredential(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "Invalid credential ID")
		return
	}

	cred := h.store.GetCredential(id)
	if cred == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "Credential not found")
		return
	}

	respondSuccess(w, http.StatusOK, cred)
}

// UpdateCredential updates a credential
func (h *CredentialHandler) UpdateCredential(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "Invalid credential ID")
		return
	}

	cred := h.store.GetCredential(id)
	if cred == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "Credential not found")
		return
	}

	var req UpdateCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	updates := &model.Credential{
		Name:           req.Name,
		CredentialType: req.CredentialType,
		Username:       req.Username,
		Domain:         req.Domain,
		Port:           req.Port,
		UseSSL:         req.UseSSL,
	}

	cred = h.store.UpdateCredential(id, updates)
	respondSuccess(w, http.StatusOK, cred)
}

// DeleteCredential deletes a credential
func (h *CredentialHandler) DeleteCredential(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "Invalid credential ID")
		return
	}

	if !h.store.DeleteCredential(id) {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "Credential not found")
		return
	}

	respondSuccess(w, http.StatusOK, map[string]string{"message": "Credential deleted"})
}
