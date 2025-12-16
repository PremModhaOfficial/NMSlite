package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nmslite/nmslite/internal/api/common"
	"github.com/nmslite/nmslite/internal/api/handlers"
	"github.com/nmslite/nmslite/internal/auth"
	"github.com/nmslite/nmslite/internal/channels"
	"github.com/nmslite/nmslite/internal/database/dbgen"
	"github.com/nmslite/nmslite/internal/globals"
	"github.com/nmslite/nmslite/internal/middleware"
	"github.com/nmslite/nmslite/internal/protocols"
)

// NewRouter NewRouter creates and configures the API router
func NewRouter(authService *auth.Service, db *pgxpool.Pool, events *channels.EventChannels) http.Handler {
	cfg := globals.GetConfig()
	logger := slog.Default()
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

	// Initialize dependencies
	queries := dbgen.New(db)
	deps := &common.Dependencies{
		Q:        queries,
		Auth:     authService,
		Events:   events,
		Registry: protocols.GetRegistry(),
		Logger:   logger,
	}

	// Initialize handlers
	healthHandler := NewHealthHandler()
	systemHandler := handlers.NewSystemHandler(deps)
	credentialHandler := handlers.NewCredentialHandler(deps)
	discoveryHandler := handlers.NewDiscoveryHandler(deps)
	monitorHandler := handlers.NewMonitorHandler(deps)

	// Public routes (no auth required)
	r.Get("/health", healthHandler.Health)
	r.Get("/ready", healthHandler.Ready)

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// Public auth endpoint
		r.Post("/login", systemHandler.Login)

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
			})

			// Monitors (Devices)
			r.Route("/monitors", func(r chi.Router) {
				r.Get("/", monitorHandler.List)
				r.Post("/", monitorHandler.Create)
				r.Get("/{id}", monitorHandler.Get)
				r.Patch("/{id}", monitorHandler.Update)
				r.Delete("/{id}", monitorHandler.Delete)
				r.Patch("/{id}/restore", monitorHandler.Restore)
			})

			// Metrics queries (batch)
			r.Post("/metrics/query", monitorHandler.QueryMetrics)

			// Protocols
			r.Route("/protocols", func(r chi.Router) {
				r.Get("/", systemHandler.ListProtocols)
			})
		})
	})

	return r
}
