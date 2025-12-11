// Package database provides PostgreSQL connection pooling using pgx/v5.
// It maintains separate connection pools for API operations and metrics writes
// to prevent metrics from blocking API queries under high load.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nmslite/nmslite/internal/config"
	"github.com/pressly/goose/v3"
)

var (
	// apiPool handles all API queries and transactional operations
	apiPool *pgxpool.Pool

	// metricsPool handles high-volume metric writes
	metricsPool *pgxpool.Pool

	// initOnce ensures pools are initialized only once
	initOnce sync.Once

	// initErr stores any initialization error
	initErr error

	// closeMu protects concurrent Close() calls
	closeMu sync.Mutex
)

// GetAPIPool returns the connection pool for API operations.
// Returns nil if InitDB has not been called successfully.
func GetAPIPool() *pgxpool.Pool {
	return apiPool
}

// GetMetricsPool returns the connection pool for metrics operations.
// Returns nil if InitDB has not been called successfully.
func GetMetricsPool() *pgxpool.Pool {
	return metricsPool
}

// InitDB initializes both API and metrics connection pools.
// This function is safe to call multiple times - only the first call will initialize the pools.
//
// The API pool is configured for:
//   - General CRUD operations
//   - Transactional queries
//   - Lower concurrency (MaxConns = MaxOpenConns from config)
//
// The metrics pool is configured for:
//   - High-volume metric inserts
//   - Batch writes
//   - Higher concurrency (MaxConns = MaxOpenConns * 2)
//   - Shorter connection lifetime
func InitDB(ctx context.Context, cfg *config.Config) error {
	initOnce.Do(func() {
		// Create base pool config
		apiPoolConfig, err := createPoolConfig(ctx, cfg, false)
		if err != nil {
			initErr = fmt.Errorf("failed to create API pool config: %w", err)
			return
		}

		metricsPoolConfig, err := createPoolConfig(ctx, cfg, true)
		if err != nil {
			initErr = fmt.Errorf("failed to create metrics pool config: %w", err)
			return
		}

		// Initialize API pool
		apiPool, err = pgxpool.NewWithConfig(ctx, apiPoolConfig)
		if err != nil {
			initErr = fmt.Errorf("failed to create API pool: %w", err)
			return
		}

		// Verify API pool connectivity
		if err = apiPool.Ping(ctx); err != nil {
			apiPool.Close()
			apiPool = nil
			initErr = fmt.Errorf("failed to ping API pool: %w", err)
			return
		}

		// Initialize metrics pool
		metricsPool, err = pgxpool.NewWithConfig(ctx, metricsPoolConfig)
		if err != nil {
			apiPool.Close()
			apiPool = nil
			initErr = fmt.Errorf("failed to create metrics pool: %w", err)
			return
		}

		// Verify metrics pool connectivity
		if err = metricsPool.Ping(ctx); err != nil {
			apiPool.Close()
			metricsPool.Close()
			apiPool = nil
			metricsPool = nil
			initErr = fmt.Errorf("failed to ping metrics pool: %w", err)
			return
		}

		initErr = nil
	})

	return initErr
}

// createPoolConfig creates a pgxpool configuration based on the database config.
// If isMetrics is true, the pool is optimized for high-volume writes.
func createPoolConfig(ctx context.Context, cfg *config.Config, isMetrics bool) (*pgxpool.Config, error) {
	// Build connection string
	connString := fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.DBName,
		cfg.Database.SSLMode,
	)

	// Parse the connection string
	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse connection string: %w", err)
	}

	// Configure connection pool
	if isMetrics {
		// Metrics pool: optimized for high-volume writes
		poolConfig.MaxConns = int32(cfg.Database.MetricsPool.MaxConns)
		poolConfig.MinConns = int32(cfg.Database.MetricsPool.MinConns)
		poolConfig.MaxConnLifetime = time.Duration(cfg.Database.MetricsPool.MaxConnLifetimeMinutes) * time.Minute
		poolConfig.MaxConnIdleTime = time.Duration(cfg.Database.MetricsPool.MaxConnIdleTimeMinutes) * time.Minute
		poolConfig.HealthCheckPeriod = time.Duration(cfg.Database.MetricsPool.HealthCheckPeriodSeconds) * time.Second
	} else {
		// API pool: balanced for general operations
		poolConfig.MaxConns = int32(cfg.Database.APIPool.MaxConns)
		poolConfig.MinConns = int32(cfg.Database.APIPool.MinConns)
		poolConfig.MaxConnLifetime = time.Duration(cfg.Database.APIPool.MaxConnLifetimeMinutes) * time.Minute
		poolConfig.MaxConnIdleTime = time.Duration(cfg.Database.APIPool.MaxConnIdleTimeMinutes) * time.Minute
		poolConfig.HealthCheckPeriod = time.Duration(cfg.Database.APIPool.HealthCheckPeriodSeconds) * time.Second
	}

	// Set connection timeout
	poolConfig.ConnConfig.ConnectTimeout = 10 * time.Second

	return poolConfig, nil
}

// RunMigrations runs all pending database migrations using embedded SQL files.
// The migrations are compiled into the binary and don't require external files.
//
// This function uses the pgx/v5/stdlib driver to provide a database/sql compatible
// connection for goose, which requires the standard library interface.
//
// Context timeout is set to 5 minutes to allow for long-running migrations.
func RunMigrations(ctx context.Context, cfg *config.Config) error {
	// Create a context with timeout for migrations
	migrateCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Build connection string for stdlib driver
	connString := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Database.Host,
		cfg.Database.Port,
		cfg.Database.User,
		cfg.Database.Password,
		cfg.Database.DBName,
		cfg.Database.SSLMode,
	)

	// Open a connection using the stdlib driver for goose compatibility
	db, err := sql.Open("pgx", connString)
	if err != nil {
		return fmt.Errorf("failed to open migration connection: %w", err)
	}
	defer db.Close()

	// Verify connectivity
	if err = db.PingContext(migrateCtx); err != nil {
		return fmt.Errorf("failed to ping database for migrations: %w", err)
	}

	// Configure goose to use the embedded filesystem
	goose.SetBaseFS(EmbeddedMigrations)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	// Run migrations from the embedded "migrations" directory
	if err := goose.UpContext(migrateCtx, db, "migrations"); err != nil {
		return fmt.Errorf("failed to run goose migrations: %w", err)
	}

	return nil
}

// Close gracefully closes both connection pools.
// This function is safe to call multiple times and from multiple goroutines.
// It waits for all active connections to be returned to the pool before closing.
func Close() {
	closeMu.Lock()
	defer closeMu.Unlock()

	if apiPool != nil {
		apiPool.Close()
		apiPool = nil
	}

	if metricsPool != nil {
		metricsPool.Close()
		metricsPool = nil
	}
}

// Stats returns statistics for both connection pools.
// Useful for monitoring and debugging connection pool health.
func Stats() (apiStats, metricsStats *pgxpool.Stat) {
	if apiPool != nil {
		stats := apiPool.Stat()
		apiStats = stats
	}

	if metricsPool != nil {
		stats := metricsPool.Stat()
		metricsStats = stats
	}

	return apiStats, metricsStats
}
