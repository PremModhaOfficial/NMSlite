package common

import (
	"log/slog"

	"github.com/nmslite/nmslite/internal/auth"
	"github.com/nmslite/nmslite/internal/channels"
	"github.com/nmslite/nmslite/internal/database/dbgen"
	"github.com/nmslite/nmslite/internal/protocols"
)

// Dependencies holds common dependencies for API handlers
type Dependencies struct {
	Q        dbgen.Querier
	Auth     *auth.Service
	Registry *protocols.Registry
	Events   *channels.EventChannels
	Logger   *slog.Logger
}

// Encrypt is a helper to encrypt data using the Auth service
func (d *Dependencies) Encrypt(data []byte) (string, error) {
	if d.Auth == nil {
		return string(data), nil // Return as-is if no auth service (shouldn't happen in prod)
	}
	return d.Auth.Encrypt(data)
}

// Decrypt is a helper to decrypt data using the Auth service
func (d *Dependencies) Decrypt(encrypted string) ([]byte, error) {
	if d.Auth == nil {
		return []byte(encrypted), nil
	}
	return d.Auth.Decrypt(encrypted)
}
