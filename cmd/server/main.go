package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nmslite/nmslite/internal/api"
	"github.com/nmslite/nmslite/internal/auth"
	"github.com/nmslite/nmslite/internal/channels"
	"github.com/nmslite/nmslite/internal/database"
	"github.com/nmslite/nmslite/internal/database/dbgen"
	"github.com/nmslite/nmslite/internal/discovery"
	"github.com/nmslite/nmslite/internal/globals"
	"github.com/nmslite/nmslite/internal/plugins"
	"github.com/nmslite/nmslite/internal/poller"
)

func main() {
	// Parse command-line flags
	dumpConfig := flag.Bool("dump-config", false, "Dump example configuration to stdout and exit")
	flag.Parse()

	// Handle dump-config flag
	if *dumpConfig {
		if err := globals.DumpExampleConfig(os.Stdout); err != nil {
			log.Fatalf("Failed to dump example config: %v", err)
		}
		os.Exit(0)
	}

	// Load configuration and logger
	cfg := globals.InitGlobal() // Initialize global config singleton
	logger := initLogger()
	logger.Info("Starting NMS Lite Server",
		"version", "1.0.0",
		"host", cfg.Server.Host,
		"port", cfg.Server.Port,
	)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize database with single pool
	pool := initDatabase(ctx)
	defer database.Close()

	authService := initAuthService()
	events := initEventChannels(ctx)
	defer events.Close()

	// Initialize BatchWriter for metrics
	batchWriter := initBatchWriter(ctx, pool)

	// Initialize and start workers
	pluginRegistry, pluginExecutor, credService := startDiscoveryWorker(ctx, pool, events, authService)
	startScheduler(ctx, pool, pluginExecutor, pluginRegistry, credService, events, batchWriter)

	// Initialize Websocket Hub
	hub := discovery.NewHub()
	go hub.Run()

	// Initialize Provisioner
	provisioner := discovery.NewProvisioner(dbgen.New(pool), events, pluginRegistry, logger)

	// Start Discovery Handlers (now in discovery package, using Hub)
	discovery.StartProvisionHandler(ctx, events, dbgen.New(pool), hub, logger, provisioner)
	discovery.StartDiscoveryCompletionLogger(ctx, events, hub, slog.Default())

	// Start HTTP server
	srv := initHTTPServer(authService, pool, events, hub, provisioner)
	go startServer(srv)

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Graceful shutdown
	shutdownServer(cancel, srv)
}

func initDatabase(ctx context.Context) *pgxpool.Pool {
	if err := database.InitDB(ctx); err != nil {
		log.Fatalf("DB init failed: %v", err)
	}

	if err := database.RunMigrations(ctx); err != nil {
		log.Fatalf("Migrations failed: %v", err)
	}

	pool := database.GetPool()

	slog.Info("Database pool initialized",
		"max_conns", globals.GetConfig().Database.Pool.MaxConns,
	)

	return pool
}

func initAuthService() *auth.Service {
	cfg := globals.GetConfig()
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
	return authService
}

func initEventChannels(ctx context.Context) *channels.EventChannels {
	// Refactored to use config.Get() internally in NewEventChannels
	events := channels.NewEventChannels()

	// Log what was configured (need to access config solely for logging)
	cfg := globals.GetConfig()
	eventBusSize := cfg.Channel.DiscoveryEventsChannelSize
	if eventBusSize <= 0 {
		eventBusSize = 50
	}

	slog.Info("EventChannels initialized",
		"discovery_buffer", eventBusSize,
		"monitor_state_buffer", cfg.Channel.StateSignalChannelSize,
	)

	// Note: Discovery Completion Logger is now started later with the Hub
	return events
}

func initBatchWriter(ctx context.Context, pool *pgxpool.Pool) *poller.BatchWriter {
	batchWriter := poller.NewBatchWriter(pool)

	go func() {
		if err := batchWriter.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("BatchWriter error", "error", err)
		}
	}()

	cfg := globals.GetConfig().Metrics
	slog.Info("BatchWriter started",
		"batch_size", cfg.BatchSize,
		"flush_interval_ms", cfg.FlushIntervalMS,
		"max_buffer_size", cfg.MaxBufferSize,
	)

	return batchWriter
}

func startDiscoveryWorker(ctx context.Context, db *pgxpool.Pool, events *channels.EventChannels, authService *auth.Service) (*plugins.Registry, *plugins.Executor, *auth.CredentialService) {
	cfg := globals.GetConfig()
	logger := slog.Default()

	// Initialize Plugin Registry
	pluginRegistry := plugins.NewRegistry(cfg.Plugins.Directory)
	if err := pluginRegistry.Scan(); err != nil {
		logger.Error("Failed to scan pluginManager", "error", err)
	} else {
		pluginList := pluginRegistry.List()
		logger.Info("Plugins loaded", "count", len(pluginList))
		for _, p := range pluginList {
			logger.Info("Plugin registered",
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
	)

	// Initialize services
	credentialService := auth.NewCredentialService(authService, dbgen.New(db))
	discoveryWorker := discovery.NewWorker(
		events,
		dbgen.New(db),
		pluginRegistry,
		pluginExecutor,
		credentialService,
		authService,
		logger,
	)

	// Start Discovery Worker
	go func() {
		if err := discoveryWorker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("Discovery worker error", "error", err)
		}
	}()

	return pluginRegistry, pluginExecutor, credentialService
}

func startScheduler(
	ctx context.Context,
	db *pgxpool.Pool,
	pluginExecutor *plugins.Executor,
	pluginRegistry *plugins.Registry,
	credService *auth.CredentialService,
	events *channels.EventChannels,
	batchWriter *poller.BatchWriter,
) {
	resultWriter := poller.NewResultWriter(batchWriter)

	scheduler := poller.NewSchedulerImpl(
		dbgen.New(db), // Wrap pool with sqlc querier - pool is still shared
		events,
		pluginExecutor,
		pluginRegistry,
		credService,
		resultWriter,
	)

	go func() {
		if err := scheduler.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("Scheduler error", "error", err)
		}
	}()

	cfg := globals.GetConfig().Scheduler
	slog.Info("Scheduler started",
		"tick_interval_ms", cfg.TickIntervalMS,
		"liveness_workers", cfg.LivenessWorkers,
		"plugin_workers", cfg.PluginWorkers,
	)
}

func initHTTPServer(authService *auth.Service, db *pgxpool.Pool, events *channels.EventChannels, hub *discovery.Hub, provisioner *discovery.Provisioner) *http.Server {
	cfg := globals.GetConfig()
	router := api.NewRouter(authService, db, events, hub, provisioner)
	return &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.GetReadTimeout(),
		WriteTimeout: cfg.Server.GetWriteTimeout(),
	}
}

func startServer(srv *http.Server) {
	cfg := globals.GetConfig()
	if cfg.TLS.Enabled {
		slog.Info("HTTPS server listening", "addr", srv.Addr)
		if err := srv.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTPS server failed", "error", err)
			os.Exit(1)
		}
	} else {
		slog.Info("HTTP server listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}
}

func shutdownServer(cancel context.CancelFunc, srv *http.Server) {
	slog.Info("Shutting down server...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
	}

	slog.Info("Server stopped gracefully")
}

func initLogger() *slog.Logger {
	return globals.InitLogger(globals.GetConfig().Logging)
}
