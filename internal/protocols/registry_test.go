package protocols

import (
	"encoding/json"
	"testing"
)

func TestRegistryInitialization(t *testing.T) {
	registry := GetRegistry()

	// Test that registry is initialized
	if registry == nil {
		t.Error("Registry should not be nil")
	}

	// Test that protocols are registered
	protocols := registry.ListProtocols()
	if len(protocols) == 0 {
		t.Error("Registry should have protocols registered")
	}

	expected := map[string]bool{
		"winrm":    false,
		"ssh":      false,
		"snmp-v2c": false,
	}

	for _, p := range protocols {
		if _, exists := expected[p.ID]; !exists {
			t.Errorf("Unexpected protocol: %s", p.ID)
		}
		expected[p.ID] = true
	}

	for protocolID, found := range expected {
		if !found {
			t.Errorf("Expected protocol not found: %s", protocolID)
		}
	}
}

func TestGetProtocol(t *testing.T) {
	registry := GetRegistry()

	testCases := []struct {
		name        string
		protocolID  string
		shouldExist bool
	}{
		{"WinRM", "winrm", true},
		{"SSH", "ssh", true},
		{"SNMP", "snmp-v2c", true},
		{"Invalid", "invalid-protocol", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			protocol, err := registry.GetProtocol(tc.protocolID)

			if tc.shouldExist {
				if err != nil {
					t.Errorf("Expected protocol %s to exist, got error: %v", tc.protocolID, err)
				}
				if protocol == nil {
					t.Error("Protocol should not be nil")
				}
				if protocol.ID != tc.protocolID {
					t.Errorf("Expected ID %s, got %s", tc.protocolID, protocol.ID)
				}
			} else {
				if err == nil {
					t.Errorf("Expected error for protocol %s, but got none", tc.protocolID)
				}
			}
		})
	}
}

func TestGetCredentialType(t *testing.T) {
	registry := GetRegistry()

	testCases := []struct {
		name        string
		protocolID  string
		shouldExist bool
	}{
		{"WinRM Credential Type", "winrm", true},
		{"SSH Credential Type", "ssh", true},
		{"SNMP Credential Type", "snmp-v2c", true},
		{"Invalid Credential Type", "invalid", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			credType, err := registry.GetCredentialType(tc.protocolID)

			if tc.shouldExist {
				if err != nil {
					t.Errorf("Expected credential type for %s to exist, got error: %v", tc.protocolID, err)
				}
				if credType == nil {
					t.Error("Credential type should not be nil")
				}
			} else {
				if err == nil {
					t.Errorf("Expected error for credential type %s, but got none", tc.protocolID)
				}
			}
		})
	}
}

// Helper function to create JSON from map
func toJSON(t *testing.T, data map[string]any) json.RawMessage {
	t.Helper()
	jsonData, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}
	return jsonData
}

func TestValidateWinRMCredentials(t *testing.T) {
	testCases := []struct {
		name      string
		creds     map[string]any
		shouldErr bool
	}{
		{
			"Valid WinRM",
			map[string]any{"username": "admin", "password": "pass123"},
			false,
		},
		{
			"Missing username",
			map[string]any{"password": "pass123"},
			true,
		},
		{
			"Missing password",
			map[string]any{"username": "admin"},
			true,
		},
		{
			"Empty username",
			map[string]any{"username": "", "password": "pass123"},
			true,
		},
	}

	registry := GetRegistry()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := registry.ValidateCredentials("winrm", toJSON(t, tc.creds))

			if tc.shouldErr && err == nil {
				t.Error("Expected validation error, but got none")
			}
			if !tc.shouldErr && err != nil {
				t.Errorf("Expected no error, but got: %v", err)
			}
		})
	}
}

func TestValidateSSHCredentials(t *testing.T) {
	testCases := []struct {
		name      string
		creds     map[string]any
		shouldErr bool
	}{
		{
			"Valid SSH with password",
			map[string]any{"username": "ubuntu", "password": "pass123"},
			false,
		},
		{
			"Valid SSH with key",
			map[string]any{"username": "ubuntu", "private_key": "-----BEGIN RSA PRIVATE KEY-----"},
			false,
		},
		{
			"Missing username",
			map[string]any{"password": "pass123"},
			true,
		},
		{
			"Missing both password and key",
			map[string]any{"username": "ubuntu"},
			true,
		},
	}

	registry := GetRegistry()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := registry.ValidateCredentials("ssh", toJSON(t, tc.creds))

			if tc.shouldErr && err == nil {
				t.Error("Expected validation error, but got none")
			}
			if !tc.shouldErr && err != nil {
				t.Errorf("Expected no error, but got: %v", err)
			}
		})
	}
}

func TestValidateSNMPCredentials(t *testing.T) {
	testCases := []struct {
		name      string
		creds     map[string]any
		shouldErr bool
	}{
		{
			"Valid SNMP",
			map[string]any{"community": "public"},
			false,
		},
		{
			"Missing community",
			map[string]any{},
			true,
		},
		{
			"Empty community",
			map[string]any{"community": ""},
			true,
		},
	}

	registry := GetRegistry()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := registry.ValidateCredentials("snmp-v2c", toJSON(t, tc.creds))

			if tc.shouldErr && err == nil {
				t.Error("Expected validation error, but got none")
			}
			if !tc.shouldErr && err != nil {
				t.Errorf("Expected no error, but got: %v", err)
			}
		})
	}
}

func TestValidationErrorFormat(t *testing.T) {
	registry := GetRegistry()

	// Test with invalid WinRM credentials (missing required fields)
	_, err := registry.ValidateCredentials("winrm", toJSON(t, map[string]any{}))
	if err == nil {
		t.Fatal("Expected validation error, but got none")
	}

	// Check if error is a ValidationErrors type
	validationErrs, ok := err.(*ValidationErrors)
	if !ok {
		t.Fatalf("Expected *ValidationErrors, got %T", err)
	}

	// Should have errors for username and password
	if len(validationErrs.Errors) != 2 {
		t.Errorf("Expected 2 validation errors, got %d", len(validationErrs.Errors))
	}

	// Check error message format
	errMsg := validationErrs.Error()
	if errMsg == "" {
		t.Error("Error message should not be empty")
	}
}

func TestInvalidProtocolValidation(t *testing.T) {
	registry := GetRegistry()

	_, err := registry.ValidateCredentials("invalid-protocol", toJSON(t, map[string]any{}))
	if err == nil {
		t.Error("Expected error for invalid protocol, but got none")
	}
}
