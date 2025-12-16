package protocols

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// Registry holds all protocol definitions and their credential types
type Registry struct {
	protocols       map[string]*Protocol
	credentialTypes map[string]reflect.Type
	mu              sync.RWMutex
}

// Protocol represents a protocol definition
type Protocol struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	DefaultPort int    `json:"default_port"`
	Version     string `json:"version"`
}

// ProtocolListResponse represents the response for listing protocols
type ProtocolListResponse struct {
	Data []*Protocol `json:"data"`
}

var (
	globalRegistry *Registry
	registryOnce   sync.Once
)

// GetRegistry returns the singleton Protocol Registry
func GetRegistry() *Registry {
	registryOnce.Do(func() {
		globalRegistry = newRegistry()
		globalRegistry.initializeProtocols()
	})
	return globalRegistry
}

// newRegistry creates a new protocol registry
func newRegistry() *Registry {
	return &Registry{
		protocols:       make(map[string]*Protocol),
		credentialTypes: make(map[string]reflect.Type),
	}
}

// initializeProtocols registers all supported protocols and their credential types
func (r *Registry) initializeProtocols() {
	// WinRM Protocol
	r.registerProtocol(&Protocol{
		ID:          "winrm",
		Name:        "Windows Server (WinRM)",
		Description: "Collects metrics from Windows servers via WinRM",
		DefaultPort: 5985,
		Version:     "1.0.0",
	}, reflect.TypeOf(WinRMCredentials{}))

	// SSH Protocol
	r.registerProtocol(&Protocol{
		ID:          "ssh",
		Name:        "Linux/Unix (SSH)",
		Description: "Collects metrics from Linux/Unix servers via SSH",
		DefaultPort: 22,
		Version:     "1.0.0",
	}, reflect.TypeOf(SSHCredentials{}))

	// SNMP v2c Protocol
	r.registerProtocol(&Protocol{
		ID:          "snmp-v2c",
		Name:        "SNMP v2c",
		Description: "Collects metrics via SNMP v2c",
		DefaultPort: 161,
		Version:     "1.0.0",
	}, reflect.TypeOf(SNMPCredentials{}))

	// SNMP v3 Protocol
	r.registerProtocol(&Protocol{
		ID:          "snmp-v3",
		Name:        "SNMP v3",
		Description: "Collects metrics via SNMP v3 (USM)",
		DefaultPort: 161,
		Version:     "1.0.0",
	}, reflect.TypeOf(SNMPv3Credentials{}))
}

// registerProtocol registers a protocol with its credential type
func (r *Registry) registerProtocol(protocol *Protocol, credType reflect.Type) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.protocols[protocol.ID] = protocol
	r.credentialTypes[protocol.ID] = credType
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

// ValidateCredentials validates credential data against a protocol's expected struct
// and returns the typed credential struct if validation passes
func (r *Registry) ValidateCredentials(protocolID string, data json.RawMessage) (any, error) {
	r.mu.RLock()
	credType, exists := r.credentialTypes[protocolID]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unknown protocol: %s", protocolID)
	}

	// Create new instance of the credential struct
	creds := reflect.New(credType).Interface()

	// Unmarshal JSON into struct with strict validation
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(creds); err != nil {
		if strings.Contains(err.Error(), "unknown field") {
			return nil, &ValidationErrors{
				Errors: []ValidationError{{
					Field:   "_json",
					Message: fmt.Sprintf("contains forbidden field: %v", err),
				}},
			}
		}
		return nil, &ValidationErrors{
			Errors: []ValidationError{{
				Field:   "_json",
				Message: fmt.Sprintf("invalid JSON format: %v", err),
			}},
		}
	}

	// Validate using validator
	if err := ValidateCredentialStruct(creds); err != nil {
		return nil, err
	}

	return creds, nil
}

// GetCredentialType returns the reflect.Type for a protocol's credentials
// This can be useful for advanced use cases like generating documentation
func (r *Registry) GetCredentialType(protocolID string) (reflect.Type, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	credType, exists := r.credentialTypes[protocolID]
	if !exists {
		return nil, fmt.Errorf("credential type not found for protocol: %s", protocolID)
	}
	return credType, nil
}
