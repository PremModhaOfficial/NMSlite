package api

import (
	"database/sql"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/nmslite/nmslite/internal/auth"
	"github.com/nmslite/nmslite/internal/config"
	"github.com/nmslite/nmslite/internal/database/db_gen"
	"github.com/nmslite/nmslite/internal/middleware"
)

// Router creates and configures the API router
func NewRouter(cfg *config.Config, authService *auth.Service, logger *slog.Logger, db *sql.DB) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.Recovery(logger))
	r.Use(middleware.Logger(logger))

	// CORS (if enabled)
	if cfg.CORS.Enabled {
		r.Use(middleware.CORS(
			cfg.CORS.AllowedOrigins,
			cfg.CORS.AllowedMethods,
			cfg.CORS.AllowedHeaders,
			cfg.CORS.MaxAgeSeconds,
		))
	}

	// Initialize handlers
	healthHandler := NewHealthHandler()
	authHandler := NewAuthHandler(authService)
	credentialHandler := NewCredentialHandler(db_gen.New(db), authService)
	discoveryHandler := NewDiscoveryHandler(db_gen.New(db), authService)
	monitorHandler := NewMonitorHandler(db_gen.New(db))
	protocolHandler := NewProtocolHandler()

	// Public routes (no auth required)
	r.Get("/health", healthHandler.Health)
	r.Get("/ready", healthHandler.Ready)

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// Public auth endpoint
		r.Post("/login", authHandler.Login)

		// Protected routes (require JWT)
		r.Group(func(r chi.Router) {
			r.Use(middleware.JWTAuth(authService))

			// Credential Profiles
			r.Route("/credentials", func(r chi.Router) {
				r.Get("/", credentialHandler.List)
				r.Post("/", credentialHandler.Create)
				r.Get("/{id}", credentialHandler.Get)
				r.Put("/{id}", credentialHandler.Update)
				r.Delete("/{id}", credentialHandler.Delete)
			})

			// Discovery Profiles
			r.Route("/discoveries", func(r chi.Router) {
				r.Get("/", discoveryHandler.List)
				r.Post("/", discoveryHandler.Create)
				r.Get("/{id}", discoveryHandler.Get)
				r.Put("/{id}", discoveryHandler.Update)
				r.Delete("/{id}", discoveryHandler.Delete)
				r.Post("/{id}/run", discoveryHandler.Run)
				r.Get("/{id}/results", discoveryHandler.GetResults)
				r.Get("/{id}/jobs/{job_id}", discoveryHandler.GetJob)
			})

			// Monitors (Devices)
			r.Route("/monitors", func(r chi.Router) {
				r.Get("/", monitorHandler.List)
				// r.Post("/", monitorHandler.Create) // Monitors are created via discovery
				r.Get("/{id}", monitorHandler.Get)
				r.Patch("/{id}", monitorHandler.Update)
				r.Delete("/{id}", monitorHandler.Delete)
				r.Patch("/{id}/restore", monitorHandler.Restore)
				r.Get("/{id}/metrics", monitorHandler.GetMetrics)
			})

			// Protocols
			r.Route("/protocols", func(r chi.Router) {
				r.Get("/", protocolHandler.List)
				r.Get("/{id}/schema", protocolHandler.GetSchema)
			})
		})
	})

	return r
}
