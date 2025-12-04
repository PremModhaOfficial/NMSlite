package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/nmslite/nmslite/internal/model"
	"github.com/nmslite/nmslite/internal/store"
)

// DeviceHandler handles device endpoints
type DeviceHandler struct {
	store *store.MockStore
}

// NewDeviceHandler creates a new device handler
func NewDeviceHandler(s *store.MockStore) *DeviceHandler {
	return &DeviceHandler{store: s}
}

// ListDevices lists all devices
func (h *DeviceHandler) ListDevices(w http.ResponseWriter, r *http.Request) {
	devices := h.store.ListDevices()
	respondSuccess(w, http.StatusOK, devices)
}

// CreateDevice creates a new device
func (h *DeviceHandler) CreateDevice(w http.ResponseWriter, r *http.Request) {
	var req CreateDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	// Validate IP
	if req.IP == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "IP address is required")
		return
	}

	if req.PollingInterval == 0 {
		req.PollingInterval = 60
	}

	device := &model.Device{
		IP:              req.IP,
		Hostname:        req.Hostname,
		OS:              req.OS,
		PollingInterval: req.PollingInterval,
	}

	device = h.store.CreateDevice(device)
	respondSuccess(w, http.StatusCreated, device)
}

// GetDevice retrieves a device by ID
func (h *DeviceHandler) GetDevice(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "Invalid device ID")
		return
	}

	device := h.store.GetDevice(id)
	if device == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "Device not found")
		return
	}

	respondSuccess(w, http.StatusOK, device)
}

// UpdateDevice updates a device
func (h *DeviceHandler) UpdateDevice(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "Invalid device ID")
		return
	}

	device := h.store.GetDevice(id)
	if device == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "Device not found")
		return
	}

	var req UpdateDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	updates := &model.Device{
		Hostname:        req.Hostname,
		OS:              req.OS,
		Status:          req.Status,
		PollingInterval: req.PollingInterval,
	}

	device = h.store.UpdateDevice(id, updates)
	respondSuccess(w, http.StatusOK, device)
}

// DeleteDevice deletes a device
func (h *DeviceHandler) DeleteDevice(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "Invalid device ID")
		return
	}

	if !h.store.DeleteDevice(id) {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "Device not found")
		return
	}

	respondSuccess(w, http.StatusOK, map[string]string{"message": "Device deleted"})
}

// DiscoverDevices discovers devices in a subnet
func (h *DeviceHandler) DiscoverDevices(w http.ResponseWriter, r *http.Request) {
	var req DiscoverRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	if req.Subnet == "" {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Subnet is required")
		return
	}

	// Mock discovery - create fake discovered devices
	response := map[string]interface{}{
		"subnet": req.Subnet,
		"count":  3,
		"discovered": []map[string]interface{}{
			{
				"ip":       "192.168.1.101",
				"hostname": "SERVER-02",
				"os":       "Windows Server 2022",
			},
			{
				"ip":       "192.168.1.102",
				"hostname": "SERVER-03",
				"os":       "Windows Server 2019",
			},
			{
				"ip":       "192.168.1.103",
				"hostname": "WORKSTATION-01",
				"os":       "Windows 10",
			},
		},
	}

	respondSuccess(w, http.StatusOK, response)
}

// ProvisionDevice provisions a device for monitoring
func (h *DeviceHandler) ProvisionDevice(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "Invalid device ID")
		return
	}

	device := h.store.GetDevice(id)
	if device == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "Device not found")
		return
	}

	var req ProvisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	// Verify credential exists
	if cred := h.store.GetCredential(req.CredentialID); cred == nil {
		respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "Credential not found")
		return
	}

	// Update device status
	if req.PollingInterval == 0 {
		req.PollingInterval = 60
	}

	device = h.store.UpdateDevice(id, &model.Device{
		Status:          "provisioned",
		PollingInterval: req.PollingInterval,
	})

	response := map[string]interface{}{
		"device_id": device.ID,
		"status":    device.Status,
		"message":   "Device provisioned for monitoring",
	}

	respondSuccess(w, http.StatusOK, response)
}

// DeprovisionDevice deprovisions a device
func (h *DeviceHandler) DeprovisionDevice(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_ID", "Invalid device ID")
		return
	}

	device := h.store.GetDevice(id)
	if device == nil {
		respondError(w, http.StatusNotFound, "NOT_FOUND", "Device not found")
		return
	}

	device = h.store.UpdateDevice(id, &model.Device{
		Status: "discovered",
	})

	response := map[string]interface{}{
		"device_id": device.ID,
		"status":    device.Status,
		"message":   "Device deprovisioned",
	}

	respondSuccess(w, http.StatusOK, response)
}
