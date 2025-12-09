package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
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

// GetSchema handles GET /api/v1/protocols/{id}/schema
func (h *ProtocolHandler) GetSchema(w http.ResponseWriter, r *http.Request) {
	protocolID := chi.URLParam(r, "id")
	if protocolID == "" {
		sendError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Protocol ID is required", nil)
		return
	}

	// Verify protocol exists
	_, err := h.registry.GetProtocol(protocolID)
	if err != nil {
		sendError(w, r, http.StatusNotFound, "NOT_FOUND", "Protocol not found", nil)
		return
	}

	// Get schema
	schema, err := h.registry.GetSchema(protocolID)
	if err != nil {
		sendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to retrieve schema", nil)
		return
	}

	// Unmarshal schema to validate and return as JSON object
	var schemaObj interface{}
	if err := json.Unmarshal(schema, &schemaObj); err != nil {
		sendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to parse schema", nil)
		return
	}

	response := map[string]interface{}{
		"protocol_id": protocolID,
		"schema":      schemaObj,
	}

	sendJSON(w, http.StatusOK, response)
}
