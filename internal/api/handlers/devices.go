package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/nmslite/nmslite/internal/api/common"
	"github.com/nmslite/nmslite/internal/database/dbgen"
	"github.com/nmslite/nmslite/internal/discovery"
)

// DeviceHandler handles discovered device endpoints (renamed from discovered-devices to devices)
type DeviceHandler struct {
	queries     *dbgen.Queries
	provisioner *discovery.Provisioner
}

func NewDeviceHandler(queries *dbgen.Queries, provisioner *discovery.Provisioner) *DeviceHandler {
	return &DeviceHandler{
		queries:     queries,
		provisioner: provisioner,
	}
}

// Routes returns a chi.Router with all device routes
func (h *DeviceHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", h.List)
	r.Get("/{id}", h.Get)
	r.Delete("/{id}", h.Delete)
	r.Post("/{id}/provision", h.Provision)
	return r
}

// List handles GET /devices - lists all discovered devices
func (h *DeviceHandler) List(w http.ResponseWriter, r *http.Request) {
	devices, err := h.queries.ListAllDiscoveredDevices(r.Context())
	if common.HandleDBError(w, r, err, "Device") {
		return
	}
	common.SendListResponse(w, devices, len(devices))
}

// Get handles GET /devices/{id}
func (h *DeviceHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := common.ParseIDParam(w, r, "id")
	if !ok {
		return
	}

	device, err := h.queries.GetDiscoveredDevice(r.Context(), id)
	if common.HandleDBError(w, r, err, "Device") {
		return
	}
	common.SendJSON(w, http.StatusOK, device)
}

// Delete handles DELETE /devices/{id}
func (h *DeviceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, ok := common.ParseIDParam(w, r, "id")
	if !ok {
		return
	}

	err := h.queries.DeleteDiscoveredDevice(r.Context(), id)
	if common.HandleDBError(w, r, err, "Device") {
		return
	}
	common.SendJSON(w, http.StatusNoContent, nil)
}

// Provision handles POST /devices/{id}/provision - provisions a monitor from a discovered device
func (h *DeviceHandler) Provision(w http.ResponseWriter, r *http.Request) {
	id, ok := common.ParseIDParam(w, r, "id")
	if !ok {
		return
	}

	monitor, err := h.provisioner.ProvisionFromID(r.Context(), id)
	if err != nil {
		common.SendError(w, r, http.StatusInternalServerError, "PROVISION_ERROR", "Failed to provision device", err)
		return
	}

	common.SendJSON(w, http.StatusCreated, monitor)
}
