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

	"github.com/jackc/pgx/v5/pgxpool"
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
	// Load configuration and logger
	cfg := loadConfig()
	logger := initLogger(cfg.Logging)
	logger.Info("Starting NMS Lite Server",
		"version", "1.0.0",
		"host", cfg.Server.Host,
		"port", cfg.Server.Port,
	)

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize database with dual pools
	apiPool, metricsPool := initDatabase(ctx, cfg, logger)
	defer database.Close()

	authService := initAuthService(cfg, logger)
	events := initEventChannels(ctx, cfg, logger)
	defer events.Close()

	// Initialize BatchWriter for metrics
	batchWriter := initBatchWriter(ctx, metricsPool, cfg, logger)

	// Initialize and start workers
	pluginRegistry, pluginExecutor, credService := startDiscoveryWorker(ctx, cfg, apiPool, events, authService, logger)
	startScheduler(ctx, cfg, apiPool, pluginExecutor, pluginRegistry, credService, events, batchWriter, logger)

	// Start HTTP server
	srv := initHTTPServer(cfg, authService, logger, apiPool, events)
	go startServer(srv, logger)

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Graceful shutdown
	shutdownServer(ctx, cancel, srv, logger)
}

func loadConfig() *config.Config {
	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	return cfg
}

func initDatabase(ctx context.Context, cfg *config.Config, logger *slog.Logger) (*pgxpool.Pool, *pgxpool.Pool) {
	if err := database.InitDB(ctx, cfg); err != nil {
		log.Fatalf("DB init failed: %v", err)
	}

	if err := database.RunMigrations(ctx, cfg); err != nil {
		log.Fatalf("Migrations failed: %v", err)
	}

	apiPool := database.GetAPIPool()
	metricsPool := database.GetMetricsPool()

	logger.Info("Database pools initialized",
		"api_pool_max_conns", cfg.Database.APIPool.MaxConns,
		"metrics_pool_max_conns", cfg.Database.MetricsPool.MaxConns,
	)

	return apiPool, metricsPool
}

func initAuthService(cfg *config.Config, logger *slog.Logger) *auth.Service {
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

func initEventChannels(ctx context.Context, cfg *config.Config, logger *slog.Logger) *channels.EventChannels {
	eventBusSize := cfg.Channel.DiscoveryEventsChannelSize
	if eventBusSize <= 0 {
		eventBusSize = 50
	}

	channelsCfg := channels.EventChannelsConfig{
		DiscoveryBufferSize:    eventBusSize,
		MonitorStateBufferSize: cfg.Channel.StateSignalChannelSize,
		PluginBufferSize:       100,
		CacheBufferSize:        cfg.Channel.CacheEventsChannelSize,
	}

	events := channels.NewEventChannels(ctx, channelsCfg)
	logger.Info("EventChannels initialized",
		"discovery_buffer", channelsCfg.DiscoveryBufferSize,
		"monitor_state_buffer", channelsCfg.MonitorStateBufferSize,
	)

	channels.StartDiscoveryCompletionLogger(ctx, events, logger)
	return events
}

func initBatchWriter(ctx context.Context, metricsPool *pgxpool.Pool, cfg *config.Config, logger *slog.Logger) *poller.BatchWriter {
	batchWriter := poller.NewBatchWriter(metricsPool, &cfg.Metrics, logger)

	go func() {
		if err := batchWriter.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("BatchWriter error", "error", err)
		}
	}()

	logger.Info("BatchWriter started",
		"batch_size", cfg.Metrics.BatchSize,
		"flush_interval_ms", cfg.Metrics.FlushIntervalMS,
		"max_buffer_size", cfg.Metrics.MaxBufferSize,
	)

	return batchWriter
}

func startDiscoveryWorker(ctx context.Context, cfg *config.Config, db *pgxpool.Pool, events *channels.EventChannels, authService *auth.Service, logger *slog.Logger) (*plugins.Registry, *plugins.Executor, *credentials.Service) {
	// Initialize Plugin Registry
	pluginRegistry := plugins.NewRegistry(cfg.Plugins.Directory, logger)
	if err := pluginRegistry.Scan(); err != nil {
		logger.Error("Failed to scan plugins", "error", err)
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
		logger,
	)

	// Initialize services
	credentialService := credentials.NewService(authService, db_gen.New(db))
	discoveryWorker := discovery.NewWorker(
		events,
		db_gen.New(db),
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
	cfg *config.Config,
	db *pgxpool.Pool,
	pluginExecutor *plugins.Executor,
	pluginRegistry *plugins.Registry,
	credService *credentials.Service,
	events *channels.EventChannels,
	batchWriter *poller.BatchWriter,
	logger *slog.Logger,
) {
	resultWriter := poller.NewResultWriter(logger, batchWriter)

	scheduler := poller.NewSchedulerImpl(
		db,
		events,
		pluginExecutor,
		pluginRegistry,
		credService,
		resultWriter,
		logger,
		cfg.Scheduler,
	)

	// Load active monitors at startup
	if err := scheduler.LoadActiveMonitors(ctx); err != nil {
		logger.Error("Failed to load active monitors", "error", err)
	}

	go func() {
		if err := scheduler.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("Scheduler error", "error", err)
		}
	}()

	logger.Info("Scheduler started",
		"tick_interval_ms", cfg.Scheduler.TickIntervalMS,
		"liveness_workers", cfg.Scheduler.LivenessWorkers,
		"plugin_workers", cfg.Scheduler.PluginWorkers,
	)
}

func initHTTPServer(cfg *config.Config, authService *auth.Service, logger *slog.Logger, db *pgxpool.Pool, events *channels.EventChannels) *http.Server {
	router := api.NewRouter(cfg, authService, logger, db, events)
	return &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.GetReadTimeout(),
		WriteTimeout: cfg.Server.GetWriteTimeout(),
	}
}

func startServer(srv *http.Server, logger *slog.Logger) {
	logger.Info("HTTP server listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("Server failed", "error", err)
		os.Exit(1)
	}
}

func shutdownServer(ctx context.Context, cancel context.CancelFunc, srv *http.Server, logger *slog.Logger) {
	logger.Info("Shutting down server...")
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
