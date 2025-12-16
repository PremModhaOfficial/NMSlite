package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nmslite/nmslite/internal/api/common"
	"github.com/nmslite/nmslite/internal/database/dbgen"
	"github.com/nmslite/nmslite/internal/discovery"
)

type DiscoveredDeviceHandler struct {
	querier     dbgen.Querier
	provisioner *discovery.Provisioner
}

func NewDiscoveredDeviceHandler(querier dbgen.Querier, provisioner *discovery.Provisioner) *DiscoveredDeviceHandler {
	return &DiscoveredDeviceHandler{
		querier:     querier,
		provisioner: provisioner,
	}
}

func (h *DiscoveredDeviceHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.List)
	r.Delete("/{id}", h.Delete)
	r.Post("/{id}/provision", h.Provision)
	return r
}

func (h *DiscoveredDeviceHandler) List(w http.ResponseWriter, r *http.Request) {
	// Check for filter params
	profileIDStr := r.URL.Query().Get("discovery_profile_id")

	var devices []dbgen.DiscoveredDevice
	var err error

	if profileIDStr != "" {
		// Filter by profile
		profileID, err := uuid.Parse(profileIDStr)
		if err != nil {
			common.SendError(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid discovery_profile_id", err)
			return
		}

		// Note: we need to wrap uuid.UUID into uuid.NullUUID for the query param if it was NullUUID,
		// but the query ListDiscoveredDevices takes pgtype.UUID (or uuid.NullUUID depending on Schema).
		// Let's check query signature.

		devices, err = h.querier.ListDiscoveredDevices(r.Context(), uuid.NullUUID{UUID: profileID, Valid: true})
	} else {
		// Global list
		devices, err = h.querier.ListAllDiscoveredDevices(r.Context())
	}

	if common.HandleDBError(w, r, err, "Discovered Devices") {
		return
	}

	common.SendListResponse(w, devices, len(devices))
}

func (h *DiscoveredDeviceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := common.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	err := h.querier.DeleteDiscoveredDevice(r.Context(), id)
	if common.HandleDBError(w, r, err, "Discovered Device") {
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *DiscoveredDeviceHandler) Provision(w http.ResponseWriter, r *http.Request) {
	id, ok := common.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	monitor, err := h.provisioner.ProvisionFromID(r.Context(), id)
	if err != nil {
		common.SendError(w, r, http.StatusInternalServerError, "PROVISION_FAILED", "Failed to provision device", err.Error())
		return
	}

	if monitor == nil {
		// Should catch db errors inside provisioner usually, but just in case
		common.SendError(w, r, http.StatusInternalServerError, "PROVISION_FAILED", "Provisioning returned no monitor", nil)
		return
	}

	common.SendJSON(w, http.StatusOK, map[string]interface{}{
		"status":     "provisioned",
		"monitor_id": monitor.ID,
	})
}
