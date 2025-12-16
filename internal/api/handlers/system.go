package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/nmslite/nmslite/internal/api/auth"
	"github.com/nmslite/nmslite/internal/api/common"
)

type SystemHandler struct {
	Deps *common.Dependencies
}

func NewSystemHandler(deps *common.Dependencies) *SystemHandler {
	return &SystemHandler{Deps: deps}
}

// Login handles POST /api/v1/login
func (h *SystemHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req auth.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		common.SendError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON payload", nil)
		return
	}

	if req.Username == "" || req.Password == "" {
		common.SendError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Username and password are required", nil)
		return
	}

	response, err := h.Deps.Auth.Login(req.Username, req.Password)
	if err != nil {
		common.SendError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid credentials", nil)
		return
	}

	common.SendJSON(w, http.StatusOK, response)
}

// ListProtocols handles GET /api/v1/protocols
func (h *SystemHandler) ListProtocols(w http.ResponseWriter, r *http.Request) {
	if h.Deps.Registry == nil {
		common.SendError(w, r, http.StatusInternalServerError, "REGISTRY_ERROR", "Protocol registry not initialized", nil)
		return
	}

	protocols := h.Deps.Registry.ListProtocols()
	common.SendListResponse(w, protocols, len(protocols))
}
