package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nmslite/nmslite/internal/api"
	"github.com/nmslite/nmslite/internal/auth"
	"github.com/nmslite/nmslite/internal/channels"
	"github.com/nmslite/nmslite/internal/config"
	"github.com/nmslite/nmslite/internal/credentials"
	"github.com/nmslite/nmslite/internal/database"
	"github.com/nmslite/nmslite/internal/database/db_gen"
	"github.com/nmslite/nmslite/internal/discovery"
	"github.com/nmslite/nmslite/internal/plugins"
	"github.com/nmslite/nmslite/internal/poller"
)

func main() {
	// Load configuration
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize structured logger
	logger := initLogger(cfg.Logging)
	logger.Info("Starting NMS Lite Server",
		"version", "1.0.0",
		"host", cfg.Server.Host,
		"port", cfg.Server.Port,
	)
	// TODO: Initialize poller
	// TODO: Initialize plugin manager

	// Initialize database connection
	db, err := database.InitDB(cfg)
	if err != nil {
		log.Fatalf("DB init failed: %v", err)
	}
	defer database.Close()

	// Run embedded migrations (compiled into the binary)
	err = database.RunMigrations()
	if err != nil {
		log.Fatalf("Migrations failed: %v", err)
	}

	// Initialize authentication service
	authService, err := auth.NewService(
		cfg.Auth.JWTSecret,
		cfg.Auth.EncryptionKey,
		cfg.Auth.AdminUsername,
		cfg.Auth.AdminPassword,
		cfg.Auth.GetJWTExpiry(),
	)
	if err != nil {
		log.Fatalf("Failed to initialize auth service: %v", err)
	}

	// Initialize EventChannels (replaces EventBus)
	eventBusSize := cfg.EventBus.DiscoveryEventsChannelSize
	if eventBusSize <= 0 {
		eventBusSize = 50 // default buffer size
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	channelsCfg := channels.EventChannelsConfig{
		DiscoveryBufferSize:    eventBusSize,
		MonitorStateBufferSize: cfg.EventBus.StateSignalChannelSize,
		PluginBufferSize:       100,
		CacheBufferSize:        cfg.EventBus.CacheEventsChannelSize,
	}
	events := channels.NewEventChannels(ctx, channelsCfg)
	defer events.Close()
	logger.Info("EventChannels initialized",
		"discovery_buffer", channelsCfg.DiscoveryBufferSize,
		"monitor_state_buffer", channelsCfg.MonitorStateBufferSize,
	)

	// Start discovery completion logger
	channels.StartDiscoveryCompletionLogger(ctx, events, logger)

	// Initialize and start StateHandler
	stateHandler := poller.NewStateHandler(events, db, logger)
	go func() {
		if err := stateHandler.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("State handler error", "error", err)
		}
	}()

	// Load active monitors into cache
	if err := stateHandler.LoadActiveMonitors(ctx); err != nil {
		logger.Error("Failed to load active monitors", "error", err)
	}

	// Initialize Plugin Registry
	pluginRegistry := plugins.NewRegistry(cfg.Plugins.Directory, logger)
	if err := pluginRegistry.Scan(); err != nil {
		logger.Error("Failed to scan plugins", "error", err)
	} else {
		pluginList := pluginRegistry.List()
		logger.Info("Plugins loaded", "count", len(pluginList))
		for _, p := range pluginList {
			logger.Info("  Plugin registered",
				"id", p.Manifest.ID,
				"name", p.Manifest.Name,
				"version", p.Manifest.Version,
				"port", p.Manifest.DefaultPort,
			)
		}
	}

	// Initialize Plugin Executor
	pluginExecutor := plugins.NewExecutor(
		pluginRegistry,
		time.Duration(cfg.Poller.PluginTimeoutMS)*time.Millisecond,
		logger,
	)

	// Initialize Credential Service
	credentialService := credentials.NewService(authService, db_gen.New(db))

	// Initialize Discovery Worker (with plugin support and channels)
	discoveryWorker := discovery.NewWorker(
		events,
		db_gen.New(db),
		pluginRegistry,
		pluginExecutor,
		credentialService,
		logger,
	)

	// Start Discovery Worker
	go func() {
		if err := discoveryWorker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("Discovery worker error", "error", err)
		}
	}()

	// Create API router with EventChannels
	router := api.NewRouter(cfg, authService, logger, db, events)

	// Create HTTP server
	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.GetReadTimeout(),
		WriteTimeout: cfg.Server.GetWriteTimeout(),
	}

	// Start server in goroutine
	go func() {
		logger.Info("HTTP server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("Server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Cancel the main context to signal all workers to stop
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server forced to shutdown", "error", err)
	}

	logger.Info("Server stopped gracefully")
}

func initLogger(cfg config.LoggingConfig) *slog.Logger {
	var handler slog.Handler

	// Set log level
	var level slog.Level
	switch cfg.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	// Set format
	if cfg.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}
