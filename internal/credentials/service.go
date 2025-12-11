package credentials

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/nmslite/nmslite/internal/auth"
	"github.com/nmslite/nmslite/internal/database/db_gen"
	"github.com/nmslite/nmslite/internal/plugins"
)

// Service handles credential operations
type Service struct {
	authService *auth.Service
	querier     db_gen.Querier
}

// NewService creates a new credential service
func NewService(authService *auth.Service, querier db_gen.Querier) *Service {
	return &Service{
		authService: authService,
		querier:     querier,
	}
}

// GetDecrypted fetches and decrypts a credential profile
func (s *Service) GetDecrypted(ctx context.Context, profileID uuid.UUID) (*plugins.Credentials, error) {
	// Fetch credential profile
	profile, err := s.querier.GetCredentialProfile(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch credential profile: %w", err)
	}

	// Decrypt credential_data (expecting a JSON string containing the encrypted payload)
	var encryptedStr string
	if err := json.Unmarshal(profile.CredentialData, &encryptedStr); err != nil {
		// Fallback: try using the raw data as string (legacy/unencrypted support)
		encryptedStr = string(profile.CredentialData)
	}

	decryptedData, err := s.authService.Decrypt(encryptedStr)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt credentials: %w", err)
	}

	// Parse JSON into a map first to handle dynamic structure
	var credMap map[string]interface{}
	if err := json.Unmarshal(decryptedData, &credMap); err != nil {
		return nil, fmt.Errorf("failed to parse credentials: %w", err)
	}

	// Build Credentials struct
	creds := &plugins.Credentials{}

	if username, ok := credMap["username"].(string); ok {
		creds.Username = username
	}
	if password, ok := credMap["password"].(string); ok {
		creds.Password = password
	}
	if domain, ok := credMap["domain"].(string); ok {
		creds.Domain = domain
	}
	if useHTTPS, ok := credMap["use_https"].(bool); ok {
		creds.UseHTTPS = useHTTPS
	}

	return creds, nil
}
