// Package handler
package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/nmslite/nmslite/internal/model"
	"github.com/nmslite/nmslite/internal/store"
)

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	store     *store.MockStore
	jwtSecret string
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(s *store.MockStore, secret string) *AuthHandler {
	return &AuthHandler{
		store:     s,
		jwtSecret: secret,
	}
}

// Login handles user login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	// Simple mock authentication - in real app, verify password hash
	user := h.store.GetUserByUsername(req.Username)
	if user == nil || req.Password != "secret" {
		respondError(w, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid username or password")
		return
	}

	// Generate tokens
	accessToken := h.generateAccessToken(user)
	refreshToken := generateRefreshToken()
	expiresAt := time.Now().Add(15 * time.Minute)

	response := map[string]interface{}{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"expires_at":    expiresAt,
	}

	respondSuccess(w, http.StatusOK, response)
}

// RefreshToken handles token refresh
func (h *AuthHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
		return
	}

	// In mock mode, just generate new tokens
	user := h.store.GetUser(1) // Default admin user
	accessToken := h.generateAccessToken(user)
	refreshToken := generateRefreshToken()
	expiresAt := time.Now().Add(15 * time.Minute)

	response := map[string]interface{}{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"expires_at":    expiresAt,
	}

	respondSuccess(w, http.StatusOK, response)
}

// generateAccessToken creates a simple JWT-like access token
func (h *AuthHandler) generateAccessToken(user *model.User) string {
	// Simple token generation (for mock purposes)
	// In production, use actual JWT library
	payload := fmt.Sprintf("%d:%s:%s:%d", user.ID, user.Username, user.Role, time.Now().Unix())
	hash := sha256.Sum256([]byte(payload + h.jwtSecret))
	token := payload + ":" + hex.EncodeToString(hash[:])
	return token
}

// generateRefreshToken creates a simple refresh token
func generateRefreshToken() string {
	hash := sha256.Sum256([]byte(time.Now().String()))
	return "refresh_" + hex.EncodeToString(hash[:16])
}
