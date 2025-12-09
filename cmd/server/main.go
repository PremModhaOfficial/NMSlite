package main

import (
	"context"
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
	"github.com/nmslite/nmslite/internal/config"
	"github.com/nmslite/nmslite/internal/database"
)

func main() {
	ctx := context.Background()

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

	// Initialize database connection
	_, err = database.InitDB(cfg)
	if err != nil {
		log.Fatalf("DB init failed: %v", err)
	}
	defer database.Close()

	// Run embedded migrations (compiled into the binary)
	err = database.RunMigrations()
	if err != nil {
		log.Fatalf("Migrations failed: %v", err)
	}

	// TODO: Initialize poller
	// TODO: Initialize plugin manager

	// Create API router
	router := api.NewRouter(cfg, authService, logger)

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
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
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
