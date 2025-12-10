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
	"github.com/sqlc-dev/pqtype"
	"net"
	"time"
)

// DiscoveryHandler handles discovery profile endpoints
type DiscoveryHandler struct {
	q           db_gen.Querier
	authService *auth.Service
}

// NewDiscoveryHandler creates a new discovery handler
func NewDiscoveryHandler(q db_gen.Querier, authService *auth.Service) *DiscoveryHandler {
	return &DiscoveryHandler{
		q:           q,
		authService: authService,
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
		TargetType           string          `json:"target_type"`
		TargetValue          string          `json:"target_value"`
		Ports                json.RawMessage `json:"ports"`
		PortScanTimeoutMs    int32           `json:"port_scan_timeout_ms"`
		CredentialProfileIDs json.RawMessage `json:"credential_profile_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body", err)
		return
	}

	if input.Name == "" || input.TargetType == "" || input.TargetValue == "" {
		sendError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Name, TargetType, and TargetValue are required", nil)
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
		TargetType:           input.TargetType,
		TargetValue:          encryptedTarget,
		Ports:                input.Ports,
		PortScanTimeoutMs:    sql.NullInt32{Int32: input.PortScanTimeoutMs, Valid: input.PortScanTimeoutMs > 0},
		CredentialProfileIds: input.CredentialProfileIDs,
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
		if err == sql.ErrNoRows {
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
		TargetType           string          `json:"target_type"`
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
		TargetType:           input.TargetType,
		TargetValue:          encryptedTarget,
		Ports:                input.Ports,
		PortScanTimeoutMs:    sql.NullInt32{Int32: input.PortScanTimeoutMs, Valid: input.PortScanTimeoutMs > 0},
		CredentialProfileIds: input.CredentialProfileIDs,
	}

	profile, err := h.q.UpdateDiscoveryProfile(r.Context(), params)
	if err != nil {
		if err == sql.ErrNoRows {
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
func (h *DiscoveryHandler) Run(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid UUID format", err)
		return
	}

	// 1. Fetch Profile
	profile, err := h.q.GetDiscoveryProfile(r.Context(), id)
	if err != nil {
		sendError(w, r, http.StatusNotFound, "NOT_FOUND", "Discovery profile not found", err)
		return
	}

	// 2. Decrypt Target
	targetValue := profile.TargetValue
	if decrypted, err := h.authService.Decrypt(targetValue); err == nil {
		targetValue = string(decrypted)
	}

	// 3. Parse Ports
	var ports []int
	if err := json.Unmarshal(profile.Ports, &ports); err != nil {
		sendError(w, r, http.StatusInternalServerError, "DATA_ERROR", "Failed to parse ports", err)
		return
	}

	// 4. Parse Credential IDs (Placeholder for future plugin usage)
	// Currently unused but kept for schema structure

	// 5. Run Scan (Synchronous)
	// Clear previous results for this profile first (Simplification for this demo)
	h.q.ClearDiscoveredDevices(r.Context(), uuid.NullUUID{UUID: id, Valid: true})

	discoveredCount := 0
	targetIP := targetValue 
	
	for _, port := range ports {
		address := fmt.Sprintf("%s:%d", targetIP, port)
		timeout := time.Duration(profile.PortScanTimeoutMs.Int32) * time.Millisecond
		if timeout == 0 {
			timeout = 1 * time.Second
		}

		conn, err := net.DialTimeout("tcp", address, timeout)
		if err == nil {
			conn.Close()
			// Port is OPEN -> Add to Discovered Devices (Staging)
			
			_, err := h.q.CreateDiscoveredDevice(r.Context(), db_gen.CreateDiscoveredDeviceParams{
				DiscoveryProfileID: uuid.NullUUID{UUID: profile.ID, Valid: true},
				IpAddress:          pqtype.Inet{IPNet: net.IPNet{IP: net.ParseIP(targetIP), Mask: net.CIDRMask(32, 32)}, Valid: true},
				Hostname:           sql.NullString{String: targetIP, Valid: true},
				Port:               int32(port),
				Status:             sql.NullString{String: "new", Valid: true},
			})
			
			if err == nil {
				discoveredCount++
			}
		}
	}

	// 6. Update Profile Status
	h.q.UpdateDiscoveryProfile(r.Context(), db_gen.UpdateDiscoveryProfileParams{
		ID:                   id,
		Name:                 profile.Name,
		TargetType:           profile.TargetType,
		TargetValue:          profile.TargetValue, // Encrypted
		Ports:                profile.Ports,
		PortScanTimeoutMs:    profile.PortScanTimeoutMs,
		CredentialProfileIds: profile.CredentialProfileIds,
		// We should add status fields update to sqlc but for now we just return success
	})

	sendJSON(w, http.StatusOK, map[string]interface{}{
		"status": "completed",
		"devices_discovered": discoveredCount,
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
		"data": results,
		"total": len(results),
	})
}

// GetJob handles GET /api/v1/discoveries/{id}/jobs/{job_id}
func (h *DiscoveryHandler) GetJob(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement discovery job status retrieval
	sendError(w, r, http.StatusNotImplemented, "NOT_IMPLEMENTED", "This endpoint is not yet implemented", nil)
}
