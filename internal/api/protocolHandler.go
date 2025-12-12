package api

import (
	"net/http"

	"github.com/nmslite/nmslite/internal/protocols"
)

// ProtocolHandler handles protocol endpoints
type ProtocolHandler struct {
	registry *protocols.Registry
}

// NewProtocolHandler creates a new protocol handler
func NewProtocolHandler() *ProtocolHandler {
	return &ProtocolHandler{
		registry: protocols.GetRegistry(),
	}
}

// List handles GET /api/v1/protocols
func (h *ProtocolHandler) List(w http.ResponseWriter, r *http.Request) {
	protoList := h.registry.ListProtocols()

	response := protocols.ProtocolListResponse{
		Data: protoList,
	}

	sendJSON(w, http.StatusOK, response)
}
