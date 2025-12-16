package auth

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/nmslite/nmslite/internal/database/dbgen"
	// "github.com/nmslite/nmslite/internal/plugins" - REMOVED
)

// Credentials for authentication (Moved from plugins)
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Domain   string `json:"domain,omitempty"`

	// SSH specific
	PrivateKey string `json:"private_key,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`

	// SNMP v2c
	Community string `json:"community,omitempty"`

	// SNMP v3 (USM)
	SecurityName  string `json:"security_name,omitempty"`
	SecurityLevel string `json:"security_level,omitempty"`
	AuthProtocol  string `json:"auth_protocol,omitempty"`
	AuthPassword  string `json:"auth_password,omitempty"`
	PrivProtocol  string `json:"priv_protocol,omitempty"`
	PrivPassword  string `json:"priv_password,omitempty"`
}

// CredentialService handles credential decryption operations
type CredentialService struct {
	authService *Service
	querier     dbgen.Querier
}

// NewCredentialService creates a new credential service
func NewCredentialService(authService *Service, querier dbgen.Querier) *CredentialService {
	return &CredentialService{
		authService: authService,
		querier:     querier,
	}
}

// GetDecrypted fetches and decrypts a credential profile
func (s *CredentialService) GetDecrypted(ctx context.Context, profileID uuid.UUID) (*Credentials, error) {
	// Fetch credential profile
	profile, err := s.querier.GetCredentialProfile(ctx, profileID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch credential profile: %w", err)
	}

	// Delegate to shared decryption logic
	return s.DecryptContainer(profile.Payload)
}

// DecryptContainer decrypts the raw payload JSON blob (which contains an encrypted string)
func (s *CredentialService) DecryptContainer(container []byte) (*Credentials, error) {
	// Decrypt payload (expecting a JSON string containing the encrypted payload)
	var encryptedStr string
	if err := json.Unmarshal(container, &encryptedStr); err != nil {
		// Fallback: try using the raw data as string (legacy/unencrypted support)
		encryptedStr = string(container)
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
	creds := &Credentials{}

	if username, ok := credMap["username"].(string); ok {
		creds.Username = username
	}
	if password, ok := credMap["password"].(string); ok {
		creds.Password = password
	}
	if domain, ok := credMap["domain"].(string); ok {
		creds.Domain = domain
	}

	// SSH
	if pk, ok := credMap["private_key"].(string); ok {
		creds.PrivateKey = pk
	}
	if pp, ok := credMap["passphrase"].(string); ok {
		creds.Passphrase = pp
	}

	// SNMP v2c
	if comm, ok := credMap["community"].(string); ok {
		creds.Community = comm
	}

	// SNMP v3
	if sn, ok := credMap["security_name"].(string); ok {
		creds.SecurityName = sn
	}
	if sl, ok := credMap["security_level"].(string); ok {
		creds.SecurityLevel = sl
	}
	if ap, ok := credMap["auth_protocol"].(string); ok {
		creds.AuthProtocol = ap
	}
	if authPass, ok := credMap["auth_password"].(string); ok {
		creds.AuthPassword = authPass
	}
	if pp, ok := credMap["priv_protocol"].(string); ok {
		creds.PrivProtocol = pp
	}
	if priv, ok := credMap["priv_password"].(string); ok {
		creds.PrivPassword = priv
	}

	return creds, nil
}
