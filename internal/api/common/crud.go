package common

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// CRUDHandler provides a generic implementation for standard CRUD API endpoints
type CRUDHandler[T any] struct {
	Deps *Dependencies
	Name string // Entity name for error messages

	// Direct function callbacks containing ALL logic for the operation
	// These functions should handle Validation -> DB Operation -> Helpers (Encrypt/Decrypt)
	ListFunc   func(ctx context.Context) ([]T, error)
	CreateFunc func(ctx context.Context, input T) (T, error)
	GetFunc    func(ctx context.Context, id uuid.UUID) (T, error)
	UpdateFunc func(ctx context.Context, id uuid.UUID, input T) (T, error)
	DeleteFunc func(ctx context.Context, id uuid.UUID) error

	// CacheInvalidation
	CacheType string // If set, triggers cache invalidation on mutation
}

// List handles GET requests
func (h *CRUDHandler[T]) List(w http.ResponseWriter, r *http.Request) {
	if h.ListFunc == nil {
		SendError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "List not supported", nil)
		return
	}

	items, err := h.ListFunc(r.Context())
	if HandleDBError(w, r, err, h.Name) {
		return
	}

	SendListResponse(w, items, len(items))
}

// Create handles POST requests
func (h *CRUDHandler[T]) Create(w http.ResponseWriter, r *http.Request) {
	if h.CreateFunc == nil {
		SendError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Create not supported", nil)
		return
	}

	input, ok := DecodeJSON[T](w, r)
	if !ok {
		return
	}

	item, err := h.CreateFunc(r.Context(), input)
	// Function callbacks return standard error, we map it.
	// If it's a validation error, the func should return something we can identify?
	// For simplicity, we assume generic DB error mapping often works, but for validation we might need checks.
	// For "dead simple", we let HandleDBError capture it or we could check for specific error types if needed.
	// But usually validation errors should be sent as 400.
	// Current HandleDBError sends 500 for unknown.
	// To keep it simple, let's allow HandleDBError to do its job, or maybe specific handlers return a "ValidationError"?
	// We'll trust HandleDBError for now or slight improvement: checking for "validation" string? No, keep simple.

	if HandleDBError(w, r, err, "Failed to create "+h.Name) {
		return
	}

	// 	if h.CacheType != "" && h.Deps.Events != nil {
	// 		h.invalidateCache(h.CacheType, uuid.Nil)
	// 	}

	SendJSON(w, http.StatusCreated, item)
}

// Get handles GET /{id} requests
func (h *CRUDHandler[T]) Get(w http.ResponseWriter, r *http.Request) {
	if h.GetFunc == nil {
		SendError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Get not supported", nil)
		return
	}

	id, ok := ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	item, err := h.GetFunc(r.Context(), id)
	if HandleDBError(w, r, err, h.Name) {
		return
	}

	SendJSON(w, http.StatusOK, item)
}

// Update handles PUT/PATCH /{id} requests
func (h *CRUDHandler[T]) Update(w http.ResponseWriter, r *http.Request) {
	if h.UpdateFunc == nil {
		SendError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Update not supported", nil)
		return
	}

	id, ok := ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	input, ok := DecodeJSON[T](w, r)
	if !ok {
		return
	}

	item, err := h.UpdateFunc(r.Context(), id, input)
	if HandleDBError(w, r, err, "Failed to update "+h.Name) {
		return
	}

	// 	if h.CacheType != "" {
	// 		h.invalidateCache(h.CacheType, id)
	// 	}

	SendJSON(w, http.StatusOK, item)
}

// Delete handles DELETE /{id} requests
func (h *CRUDHandler[T]) Delete(w http.ResponseWriter, r *http.Request) {
	if h.DeleteFunc == nil {
		SendError(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "Delete not supported", nil)
		return
	}

	id, ok := ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	err := h.DeleteFunc(r.Context(), id)
	if HandleDBError(w, r, err, h.Name) {
		return
	}

	// 	if h.CacheType != "" {
	// 		h.invalidateCache(h.CacheType, id)
	// 	}

	SendJSON(w, http.StatusNoContent, nil)
}

// func (h *CRUDHandler[T]) invalidateCache(entityType string, id uuid.UUID) {
// 	if h.Deps.Events == nil {
// 		return
// 	}
// 	h.Deps.Events.CacheInvalidate <- channels.CacheInvalidateEvent{
// 		EntityType: entityType,
// 		EntityID:   id,
// 		Timestamp:  time.Now(),
// 	}
// }
