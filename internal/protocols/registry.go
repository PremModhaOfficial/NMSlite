package protocols

import (
	"encoding/json"
	"fmt"
	"sync"
)

// Registry holds all protocol definitions and schemas
type Registry struct {
	protocols map[string]*Protocol
	schemas   map[string]json.RawMessage
	mu        sync.RWMutex
}

// Protocol represents a protocol definition
type Protocol struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	DefaultPort  int    `json:"default_port"`
	Version      string `json:"version"`
}

var (
	globalRegistry *Registry
	registryOnce   sync.Once
)

// GetRegistry returns the singleton Protocol Registry
func GetRegistry() *Registry {
	registryOnce.Do(func() {
		globalRegistry = NewRegistry()
		globalRegistry.initializeProtocols()
	})
	return globalRegistry
}

// NewRegistry creates a new protocol registry
func NewRegistry() *Registry {
	return &Registry{
		protocols: make(map[string]*Protocol),
		schemas:   make(map[string]json.RawMessage),
	}
}

// initializeProtocols registers all supported protocols and their schemas
func (r *Registry) initializeProtocols() {
	// WinRM Protocol
	r.registerProtocol(&Protocol{
		ID:          "winrm",
		Name:        "Windows Server (WinRM)",
		Description: "Collects metrics from Windows servers via WinRM",
		DefaultPort: 5985,
		Version:     "1.0.0",
	}, winrmSchema)

	// SSH Protocol
	r.registerProtocol(&Protocol{
		ID:          "ssh",
		Name:        "Linux/Unix (SSH)",
		Description: "Collects metrics from Linux/Unix servers via SSH",
		DefaultPort: 22,
		Version:     "1.0.0",
	}, sshSchema)

	// SNMP v2c Protocol
	r.registerProtocol(&Protocol{
		ID:          "snmp-v2c",
		Name:        "SNMP v2c",
		Description: "Collects metrics via SNMP v2c",
		DefaultPort: 161,
		Version:     "1.0.0",
	}, snmpSchema)
}

// registerProtocol registers a protocol with its schema
func (r *Registry) registerProtocol(protocol *Protocol, schema json.RawMessage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.protocols[protocol.ID] = protocol
	r.schemas[protocol.ID] = schema
}

// GetProtocol returns a protocol by ID
func (r *Registry) GetProtocol(id string) (*Protocol, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	protocol, exists := r.protocols[id]
	if !exists {
		return nil, fmt.Errorf("protocol not found: %s", id)
	}
	return protocol, nil
}

// ListProtocols returns all registered protocols
func (r *Registry) ListProtocols() []*Protocol {
	r.mu.RLock()
	defer r.mu.RUnlock()

	protocols := make([]*Protocol, 0, len(r.protocols))
	for _, p := range r.protocols {
		protocols = append(protocols, p)
	}
	return protocols
}

// GetSchema returns the JSON schema for a protocol
func (r *Registry) GetSchema(protocolID string) (json.RawMessage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	schema, exists := r.schemas[protocolID]
	if !exists {
		return nil, fmt.Errorf("schema not found for protocol: %s", protocolID)
	}
	return schema, nil
}

// ValidateCredentials validates credential data against a protocol's schema
func (r *Registry) ValidateCredentials(protocolID string, credentials map[string]interface{}) error {
	// TODO: Implement JSON schema validation using a library like jsonschema
	// For now, basic protocol-specific validation
	switch protocolID {
	case "winrm":
		return validateWinRMCredentials(credentials)
	case "ssh":
		return validateSSHCredentials(credentials)
	case "snmp-v2c":
		return validateSNMPCredentials(credentials)
	default:
		return fmt.Errorf("unknown protocol: %s", protocolID)
	}
}

// validateWinRMCredentials validates WinRM credentials
func validateWinRMCredentials(creds map[string]interface{}) error {
	username, ok := creds["username"].(string)
	if !ok || username == "" {
		return fmt.Errorf("username is required for WinRM")
	}

	password, ok := creds["password"].(string)
	if !ok || password == "" {
		return fmt.Errorf("password is required for WinRM")
	}

	return nil
}

// validateSSHCredentials validates SSH credentials
func validateSSHCredentials(creds map[string]interface{}) error {
	username, ok := creds["username"].(string)
	if !ok || username == "" {
		return fmt.Errorf("username is required for SSH")
	}

	// Either password or private_key must be provided
	password, hasPassword := creds["password"].(string)
	privateKey, hasPrivateKey := creds["private_key"].(string)

	if !hasPassword && !hasPrivateKey {
		return fmt.Errorf("either password or private_key is required for SSH")
	}

	if hasPassword && password == "" && !hasPrivateKey {
		return fmt.Errorf("password cannot be empty if private_key is not provided")
	}

	if hasPrivateKey && privateKey == "" && !hasPassword {
		return fmt.Errorf("private_key cannot be empty if password is not provided")
	}

	return nil
}

// validateSNMPCredentials validates SNMP credentials
func validateSNMPCredentials(creds map[string]interface{}) error {
	community, ok := creds["community"].(string)
	if !ok || community == "" {
		return fmt.Errorf("community is required for SNMP v2c")
	}

	return nil
}

// JSON Schema Definitions

// WinRM schema - username and password required
var winrmSchema = json.RawMessage(`{
	"$schema": "http://json-schema.org/draft-07/schema#",
	"type": "object",
	"title": "WinRM Credentials",
	"description": "Credentials for Windows Remote Management",
	"required": ["username", "password"],
	"properties": {
		"username": {
			"type": "string",
			"title": "Username",
			"description": "Windows user account",
			"minLength": 1
		},
		"password": {
			"type": "string",
			"title": "Password",
			"description": "Windows user password",
			"minLength": 1
		},
		"domain": {
			"type": "string",
			"title": "Domain",
			"description": "Windows domain (optional)",
			"default": ""
		},
		"use_https": {
			"type": "boolean",
			"title": "Use HTTPS",
			"description": "Use HTTPS instead of HTTP",
			"default": false
		}
	}
}`)

// SSH schema - username required, password or private_key required
var sshSchema = json.RawMessage(`{
	"$schema": "http://json-schema.org/draft-07/schema#",
	"type": "object",
	"title": "SSH Credentials",
	"description": "Credentials for SSH access",
	"required": ["username"],
	"properties": {
		"username": {
			"type": "string",
			"title": "Username",
			"description": "SSH user account",
			"minLength": 1
		},
		"password": {
			"type": "string",
			"title": "Password",
			"description": "SSH user password (mutually exclusive with private_key)"
		},
		"private_key": {
			"type": "string",
			"title": "Private Key",
			"description": "SSH private key in PEM format (mutually exclusive with password)"
		},
		"passphrase": {
			"type": "string",
			"title": "Passphrase",
			"description": "Passphrase for encrypted private key (optional)"
		},
		"port": {
			"type": "integer",
			"title": "Port",
			"description": "SSH port (default: 22)",
			"default": 22
		}
	}
}`)

// SNMP schema - community string required
var snmpSchema = json.RawMessage(`{
	"$schema": "http://json-schema.org/draft-07/schema#",
	"type": "object",
	"title": "SNMP v2c Credentials",
	"description": "Credentials for SNMP v2c access",
	"required": ["community"],
	"properties": {
		"community": {
			"type": "string",
			"title": "Community String",
			"description": "SNMP community string for read access",
			"minLength": 1,
			"default": "public"
		}
	}
}`)
