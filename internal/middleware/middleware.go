package middleware

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/nmslite/nmslite/internal/auth"
)

type contextKey string

const (
	RequestIDKey contextKey = "request_id"
	UsernameKey  contextKey = "username"
)

// ErrorResponse represents a standard error response
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code      string      `json:"code"`
	Message   string      `json:"message"`
	Details   interface{} `json:"details,omitempty"`
	RequestID string      `json:"request_id"`
}

// RequestID middleware adds a unique request ID to each request
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := uuid.New().String()
		ctx := context.WithValue(r.Context(), RequestIDKey, requestID)
		w.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Logger middleware logs HTTP requests
func Logger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(wrapped, r)

			duration := time.Since(start)

			requestID, _ := r.Context().Value(RequestIDKey).(string)
			username, _ := r.Context().Value(UsernameKey).(string)

			logger.Info("Request completed",
				"request_id", requestID,
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.statusCode,
				"duration_ms", duration.Milliseconds(),
				"user", username,
				"ip", r.RemoteAddr,
			)
		})
	}
}

// CORS middleware handles CORS headers
func CORS(allowedOrigins, allowedMethods, allowedHeaders []string, maxAge int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Check if origin is allowed
			allowed := false
			for _, o := range allowedOrigins {
				if o == "*" || o == origin {
					allowed = true
					break
				}
			}

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", strings.Join(allowedMethods, ", "))
				w.Header().Set("Access-Control-Allow-Headers", strings.Join(allowedHeaders, ", "))
				w.Header().Set("Access-Control-Max-Age", string(rune(maxAge)))
			}

			// Handle preflight
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// JWTAuth middleware validates JWT tokens
func JWTAuth(authService *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				sendError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Missing authorization header", nil)
				return
			}

			// Check Bearer prefix
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				sendError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid authorization header format", nil)
				return
			}

			tokenString := parts[1]

			// Validate token
			claims, err := authService.ValidateToken(tokenString)
			if err != nil {
				sendError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid or expired token", nil)
				return
			}

			// Add username to context
			ctx := context.WithValue(r.Context(), UsernameKey, claims.Username)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Recovery middleware recovers from panics
func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					requestID, _ := r.Context().Value(RequestIDKey).(string)
					logger.Error("Panic recovered",
						"request_id", requestID,
						"error", err,
						"path", r.URL.Path,
					)

					sendError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred", nil)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// sendError sends a standardized error response
func sendError(w http.ResponseWriter, r *http.Request, status int, code, message string, details interface{}) {
	requestID, _ := r.Context().Value(RequestIDKey).(string)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	response := ErrorResponse{
		Error: ErrorDetail{
			Code:      code,
			Message:   message,
			Details:   details,
			RequestID: requestID,
		},
	}

	json.NewEncoder(w).Encode(response)
}
