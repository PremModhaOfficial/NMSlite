// Package database provides PostgreSQL connection pooling using pgx/v5.
// It maintains a single connection pool for all database operations.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/nmslite/nmslite/internal/globals"
	"github.com/pressly/goose/v3"
)

var (
	// pool handles all database operations
	pool *pgxpool.Pool

	// initOnce ensures the pool is initialized only once
	initOnce sync.Once

	// initErr stores any initialization error
	initErr error

	// closeMu protects concurrent Close() calls
	closeMu sync.Mutex
)

// GetPool returns the connection pool for database operations.
// Returns nil if InitDB has not been called successfully.
func GetPool() *pgxpool.Pool {
	return pool
}

// InitDB initializes the database connection pool.
// This function is safe to call multiple times - only the first call will initialize the pool.
//
// The pool is configured for:
//   - All database operations (API, metrics, etc.)
//   - Optimized for mixed workloads
//
// InitDB initializes the database connection pool.
// This function is safe to call multiple times - only the first call will initialize the pool.
//
// The pool is configured for:
//   - All database operations (API, metrics, etc.)
//   - Optimized for mixed workloads
func InitDB(ctx context.Context) error {
	initOnce.Do(func() {
		// Create pool config
		poolConfig, err := createPoolConfig()
		if err != nil {
			initErr = fmt.Errorf("failed to create pool config: %w", err)
			return
		}

		// Initialize pool
		pool, err = pgxpool.NewWithConfig(ctx, poolConfig)
		if err != nil {
			initErr = fmt.Errorf("failed to create pool: %w", err)
			return
		}

		// Verify pool connectivity
		if err = pool.Ping(ctx); err != nil {
			pool.Close()
			pool = nil
			initErr = fmt.Errorf("failed to ping pool: %w", err)
			return
		}

		initErr = nil
	})

	return initErr
}

// createPoolConfig creates a pgxpool configuration based on the database config.
func createPoolConfig() (*pgxpool.Config, error) {
	cfg := globals.GetConfig()

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

	// Configure connection pool with unified settings
	poolConfig.MaxConns = int32(cfg.Database.Pool.MaxConns)
	poolConfig.MinConns = int32(cfg.Database.Pool.MinConns)
	poolConfig.MaxConnLifetime = time.Duration(cfg.Database.Pool.MaxConnLifetimeMinutes) * time.Minute
	poolConfig.MaxConnIdleTime = time.Duration(cfg.Database.Pool.MaxConnIdleTimeMinutes) * time.Minute
	poolConfig.HealthCheckPeriod = time.Duration(cfg.Database.Pool.HealthCheckPeriodSeconds) * time.Second

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
func RunMigrations(ctx context.Context) error {
	cfg := globals.GetConfig()

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

// Close gracefully closes the connection pool.
// This function is safe to call multiple times and from multiple goroutines.
// It waits for all active connections to be returned to the pool before closing.
func Close() {
	closeMu.Lock()
	defer closeMu.Unlock()

	if pool != nil {
		pool.Close()
		pool = nil
	}
}

// Stats returns statistics for both connection pools.
// Useful for monitoring and debugging connection pool health.
