package database

import "embed"

// EmbeddedMigrations contains all SQL migration files embedded into the binary.
// The migrations are loaded at compile time from the migrations/ subdirectory.
//
// This allows the application to run database migrations without requiring
// external SQL files to be present on the filesystem at runtime.
//
//go:embed migrations/*.sql
var EmbeddedMigrations embed.FS
