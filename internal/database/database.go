// Package database
package database

import (
	"database/sql"
	"fmt"
	"sync"

	_ "github.com/lib/pq"
	"github.com/nmslite/nmslite/internal/config"
	"github.com/pressly/goose/v3"
)

var (
	instance *sql.DB
	once     sync.Once
)

func GetDB() *sql.DB {
	return instance
}

func InitDB(cfg *config.Config) (*sql.DB, error) {
	var err error
	once.Do(func() {
		conStr := cfg.Database.GetDSN()
		instance, err = sql.Open("postgres", conStr)
		if err != nil {
			return
		}

		if err = instance.Ping(); err != nil {
			return
		}
	})

	return instance, err
}

// RunMigrations runs all pending database migrations using embedded SQL files.
// The migrations are compiled into the binary and don't require external files.
func RunMigrations() error {
	if instance == nil {
		return fmt.Errorf("database not initialized: call InitDB first")
	}

	// Configure goose to use the embedded filesystem
	goose.SetBaseFS(EmbeddedMigrations)

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("failed to set goose dialect: %w", err)
	}

	// Run migrations from the embedded "migrations" directory
	if err := goose.Up(instance, "migrations"); err != nil {
		return fmt.Errorf("failed to run goose migrations: %w", err)
	}

	return nil
}

func Close() error {
	if instance != nil {
		return instance.Close()
	}
	return nil
}
