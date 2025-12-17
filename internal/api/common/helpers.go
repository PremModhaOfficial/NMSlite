package common

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/nmslite/nmslite/internal/api/auth"
)

// SendJSON sends a JSON response
func SendJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

// SendError sends a standardized error response
func SendError(w http.ResponseWriter, r *http.Request, status int, code, message string, details interface{}) {
	requestID, _ := r.Context().Value(auth.RequestIDKey).(string)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	response := auth.ErrorResponse{
		Error: auth.ErrorDetail{
			Code:      code,
			Message:   message,
			Details:   details,
			RequestID: requestID,
		},
	}

	json.NewEncoder(w).Encode(response)
}

// ParseIDParam extracts and validates an int64 ID from URL params
func ParseIDParam(w http.ResponseWriter, r *http.Request, param string) (int64, bool) {
	idStr := chi.URLParam(r, param)
	if idStr == "" {
		SendError(w, r, http.StatusBadRequest, "MISSING_ID", "Missing ID parameter", nil)
		return 0, false
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		SendError(w, r, http.StatusBadRequest, "INVALID_ID", "Invalid ID format", err)
		return 0, false
	}
	return id, true
}

// DecodeJSON decodes request body with error handling
func DecodeJSON[T any](w http.ResponseWriter, r *http.Request) (T, bool) {
	var input T
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		SendError(w, r, http.StatusBadRequest, "INVALID_BODY", "Invalid JSON body", err)
		return input, false
	}
	return input, true
}

// HandleDBError sends appropriate error response for DB errors
func HandleDBError(w http.ResponseWriter, r *http.Request, err error, entityName string) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, pgx.ErrNoRows) {
		SendError(w, r, http.StatusNotFound, "NOT_FOUND", entityName+" not found", nil)
	} else {
		SendError(w, r, http.StatusInternalServerError, "DB_ERROR", "Database error", err)
	}
	return true
}

// SendListResponse sends a standardized list response
func SendListResponse(w http.ResponseWriter, data interface{}, total int) {
	SendJSON(w, http.StatusOK, map[string]interface{}{
		"data":  data,
		"total": total,
	})
}
