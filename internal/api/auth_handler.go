package api

import (
	"encoding/json"
	"net/http"

	"github.com/nmslite/nmslite/internal/auth"
	"github.com/nmslite/nmslite/internal/middleware"
)

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	authService *auth.Service
}

// NewAuthHandler creates a new authentication handler
func NewAuthHandler(authService *auth.Service) *AuthHandler {
	return &AuthHandler{
		authService: authService,
	}
}

// Login handles POST /api/v1/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req auth.LoginRequest

	// Parse request body
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON payload", nil)
		return
	}

	// Validate input
	if req.Username == "" || req.Password == "" {
		sendError(w, r, http.StatusBadRequest, "VALIDATION_ERROR", "Username and password are required", nil)
		return
	}

	// Authenticate
	response, err := h.authService.Login(req.Username, req.Password)
	if err != nil {
		sendError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid credentials", nil)
		return
	}

	// Send response
	sendJSON(w, http.StatusOK, response)
}

// sendJSON sends a JSON response
func sendJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// sendError sends a standardized error response
func sendError(w http.ResponseWriter, r *http.Request, status int, code, message string, details interface{}) {
	requestID, _ := r.Context().Value(middleware.RequestIDKey).(string)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	response := middleware.ErrorResponse{
		Error: middleware.ErrorDetail{
			Code:      code,
			Message:   message,
			Details:   details,
			RequestID: requestID,
		},
	}

	json.NewEncoder(w).Encode(response)
}
