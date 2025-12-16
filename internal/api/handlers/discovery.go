package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/nmslite/nmslite/internal/api/common"
	"github.com/nmslite/nmslite/internal/channels"
	"github.com/nmslite/nmslite/internal/database/dbgen"
)

// DiscoveryHandler handles discovery profile endpoints
type DiscoveryHandler struct {
	*common.CRUDHandler[dbgen.DiscoveryProfile]
}

func NewDiscoveryHandler(deps *common.Dependencies) *DiscoveryHandler {
	h := &DiscoveryHandler{}

	h.CRUDHandler = &common.CRUDHandler[dbgen.DiscoveryProfile]{
		Deps: deps,
		Name: "Discovery Profile",
	}

	h.ListFunc = h.list
	h.CreateFunc = h.create
	h.GetFunc = h.get
	h.UpdateFunc = h.update
	h.DeleteFunc = h.delete

	return h
}

func (h *DiscoveryHandler) list(ctx context.Context) ([]dbgen.DiscoveryProfile, error) {
	profiles, err := h.Deps.Q.ListDiscoveryProfiles(ctx)
	if err != nil {
		return nil, err
	}
	for i := range profiles {
		if decrypted, err := h.Deps.Decrypt(profiles[i].TargetValue); err == nil {
			profiles[i].TargetValue = string(decrypted)
		}
	}
	return profiles, nil
}

func (h *DiscoveryHandler) create(ctx context.Context, input dbgen.DiscoveryProfile) (dbgen.DiscoveryProfile, error) {
	if input.Name == "" || input.TargetValue == "" {
		return dbgen.DiscoveryProfile{}, http.ErrMissingFile
	}

	encrypted, err := h.Deps.Encrypt([]byte(input.TargetValue))
	if err != nil {
		return dbgen.DiscoveryProfile{}, err
	}
	// Don't modify input pointer target value for safety, use param directly?
	// input is value receiver here effectively (though struct could contain pointers).

	params := dbgen.CreateDiscoveryProfileParams{
		Name:                input.Name,
		TargetValue:         encrypted,
		Port:                input.Port,
		PortScanTimeoutMs:   input.PortScanTimeoutMs,
		CredentialProfileID: input.CredentialProfileID,
		AutoProvision:       input.AutoProvision,
		AutoRun:             input.AutoRun,
	}

	profile, err := h.Deps.Q.CreateDiscoveryProfile(ctx, params)
	if err != nil {
		return profile, err
	}

	if input.AutoRun.Bool {
		triggerDiscovery(ctx, h.Deps, profile.ID)
	}
	return profile, nil
}

func (h *DiscoveryHandler) get(ctx context.Context, id uuid.UUID) (dbgen.DiscoveryProfile, error) {
	profile, err := h.Deps.Q.GetDiscoveryProfile(ctx, id)
	if err != nil {
		return dbgen.DiscoveryProfile{}, err
	}
	if decrypted, err := h.Deps.Decrypt(profile.TargetValue); err == nil {
		profile.TargetValue = string(decrypted)
	}
	return profile, nil
}

func (h *DiscoveryHandler) update(ctx context.Context, id uuid.UUID, input dbgen.DiscoveryProfile) (dbgen.DiscoveryProfile, error) {
	encrypted, err := h.Deps.Encrypt([]byte(input.TargetValue))
	if err != nil {
		return dbgen.DiscoveryProfile{}, err
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
	return h.Deps.Q.UpdateDiscoveryProfile(ctx, params)
}

func (h *DiscoveryHandler) delete(ctx context.Context, id uuid.UUID) error {
	return h.Deps.Q.DeleteDiscoveryProfile(ctx, id)
}

func triggerDiscovery(ctx context.Context, deps *common.Dependencies, id uuid.UUID) {
	if deps.Events == nil {
		return
	}
	select {
	case deps.Events.DiscoveryRequest <- channels.DiscoveryRequestEvent{
		ProfileID: id,
		StartedAt: time.Now(),
	}:
	case <-ctx.Done():
	default:
		// Log full?
	}
}

// Run handles POST /api/v1/discoveries/{id}/run
func (h *DiscoveryHandler) Run(w http.ResponseWriter, r *http.Request) {
	id, ok := common.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	// Validate existence using GetFunc (simplest way to ensure it exists and we have access)
	_, err := h.GetFunc(r.Context(), id)
	if common.HandleDBError(w, r, err, "Discovery profile") {
		return
	}

	triggerDiscovery(r.Context(), h.Deps, id)

	common.SendJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":     "accepted",
		"message":    "Discovery started",
		"profile_id": id.String(),
	})
}

// GetResults handles GET /api/v1/discoveries/{id}/results
func (h *DiscoveryHandler) GetResults(w http.ResponseWriter, r *http.Request) {
	id, ok := common.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	results, err := h.Deps.Q.ListDiscoveredDevices(r.Context(), uuid.NullUUID{UUID: id, Valid: true})
	if common.HandleDBError(w, r, err, "Discovery results") {
		return
	}

	common.SendListResponse(w, results, len(results))
}
