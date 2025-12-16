package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/nmslite/nmslite/internal/middleware"
)

// sendJSON sends a JSON response
func sendJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
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

// parseUUIDParam extracts and validates a UUID from URL params
func parseUUIDParam(w http.ResponseWriter, r *http.Request, param string) (uuid.UUID, bool) {
	idStr := chi.URLParam(r, param)
	id, err := uuid.Parse(idStr)
	if err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid UUID format", err)
		return uuid.Nil, false
	}
	return id, true
}

// decodeJSON decodes request body with error handling
func decodeJSON[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var input T
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		sendError(w, r, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body", err)
		return input, false
	}
	return input, true
}

// handleDBError sends appropriate error response for DB errors
func handleDBError(w http.ResponseWriter, r *http.Request, err error, entityName string) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, pgx.ErrNoRows) {
		sendError(w, r, http.StatusNotFound, "NOT_FOUND", entityName+" not found", nil)
	} else {
		sendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Database error", err)
	}
	return true
}
