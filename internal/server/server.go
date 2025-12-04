package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/nmslite/nmslite/internal/handler"
	"github.com/nmslite/nmslite/internal/store"
)

// Server represents the HTTP server
type Server struct {
	router *chi.Mux
	port   string
}

// NewServer creates and configures the HTTP server
func NewServer(port string) *Server {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.StripSlashes)

	// Initialize store
	mockStore := store.NewMockStore()

	// Initialize handlers
	authHandler := handler.NewAuthHandler(mockStore, "mock-jwt-secret")
	credHandler := handler.NewCredentialHandler(mockStore)
	devHandler := handler.NewDeviceHandler(mockStore)
	metricsHandler := handler.NewMetricsHandler(mockStore)
	healthHandler := handler.NewHealthHandler()

	// Health check endpoint (no auth required)
	r.Get("/health", healthHandler.HealthCheck)

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// Authentication endpoints
		r.Route("/auth", func(r chi.Router) {
			r.Post("/login", authHandler.Login)
			r.Post("/refresh", authHandler.RefreshToken)
		})

		// Protected endpoints
		r.Group(func(r chi.Router) {
			// Apply JWT middleware here when needed
			// r.Use(jwtMiddleware)

			// Credential endpoints
			r.Route("/credentials", func(r chi.Router) {
				r.Get("/", credHandler.ListCredentials)
				r.Post("/", credHandler.CreateCredential)
				r.Get("/{id}", credHandler.GetCredential)
				r.Put("/{id}", credHandler.UpdateCredential)
				r.Delete("/{id}", credHandler.DeleteCredential)
			})

			// Device endpoints
			r.Route("/devices", func(r chi.Router) {
				r.Get("/", devHandler.ListDevices)
				r.Post("/", devHandler.CreateDevice)
				r.Post("/discover", devHandler.DiscoverDevices)
				r.Get("/{id}", devHandler.GetDevice)
				r.Put("/{id}", devHandler.UpdateDevice)
				r.Delete("/{id}", devHandler.DeleteDevice)
				r.Post("/{id}/provision", devHandler.ProvisionDevice)
				r.Post("/{id}/deprovision", devHandler.DeprovisionDevice)

				// Metrics endpoints
				r.Get("/{id}/metrics", metricsHandler.GetLatestMetrics)
				r.Post("/{id}/metrics/history", metricsHandler.GetMetricsHistory)
			})
		})
	})

	return &Server{
		router: r,
		port:   port,
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	return http.ListenAndServe(":"+s.port, s.router)
}

// Router returns the chi router
func (s *Server) Router() *chi.Mux {
	return s.router
}
