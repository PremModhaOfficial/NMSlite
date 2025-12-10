package api

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nmslite/nmslite/internal/auth"
	"github.com/nmslite/nmslite/internal/database/db_gen"
	"github.com/nmslite/nmslite/internal/eventbus"
	"time"
)

// DiscoveryHandler handles discovery profile endpoints
type DiscoveryHandler struct {
	q           db_gen.Querier
	authService *auth.Service
	eventBus    *eventbus.EventBus
}

// NewDiscoveryHandler creates a new discovery handler
func NewDiscoveryHandler(q db_gen.Querier, authService *auth.Service, eb *eventbus.EventBus) *DiscoveryHandler {
	return &DiscoveryHandler{
		q:           q,
		authService: authService,
		eventBus:    eb,
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
// Returns 202 Accepted immediately with a job_id for async polling
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

	// 2. Generate unique job ID for this discovery run
	jobID := uuid.New()

	// 3. Publish TopicDiscoveryRun event with profile ID and job ID
	runEvent := eventbus.DiscoveryRunEvent{
		JobID:     jobID,
		ProfileID: id,
		StartedAt: time.Now(),
	}

	err = h.eventBus.Publish(r.Context(), eventbus.TopicDiscoveryRun, runEvent)
	if err != nil {
		sendError(w, r, http.StatusInternalServerError, "EVENT_PUBLISH_ERROR", "Failed to publish discovery event", err)
		return
	}

	// 4. Create initial job entry in job store
	jobStore := eventbus.GetGlobalJobStore()
	jobStore.SetJob(&eventbus.DiscoveryJob{
		JobID:     jobID,
		ProfileID: id,
		Status:    "pending",
		Progress:  0,
		StartedAt: time.Now(),
	})

	// 5. Return 202 Accepted immediately with job_id
	// Suppress unused profile warning by using it
	_ = profile
	sendJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":     "accepted",
		"message":    "Discovery job queued for execution",
		"job_id":     jobID.String(),
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

// GetJob handles GET /api/v1/discoveries/{id}/jobs/{job_id}
// Returns the status and progress of a discovery job
func (h *DiscoveryHandler) GetJob(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	profileID, err := uuid.Parse(idStr)
	if err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid profile UUID format", err)
		return
	}

	jobIDStr := chi.URLParam(r, "job_id")
	jobID, err := uuid.Parse(jobIDStr)
	if err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_JOB_ID", "Invalid job UUID format", err)
		return
	}

	// Query the job store for job status
	jobStore := eventbus.GetGlobalJobStore()
	job, exists := jobStore.GetJob(jobID)
	if !exists {
		sendError(w, r, http.StatusNotFound, "JOB_NOT_FOUND", "Discovery job not found", nil)
		return
	}

	// Validate that the job belongs to the requested profile
	if job.ProfileID != profileID {
		sendError(w, r, http.StatusNotFound, "JOB_NOT_FOUND", "Job does not belong to this discovery profile", nil)
		return
	}

	// Return job status and progress
	sendJSON(w, http.StatusOK, map[string]interface{}{
		"job_id":        job.JobID.String(),
		"profile_id":    job.ProfileID.String(),
		"status":        job.Status,
		"progress":      job.Progress,
		"started_at":    job.StartedAt.Format(time.RFC3339),
		"completed_at":  job.CompletedAt,
		"devices_found": job.DevicesFound,
		"error":         job.Error,
	})
}
