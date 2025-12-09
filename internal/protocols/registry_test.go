package protocols

import (
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
		"winrm":     false,
		"ssh":       false,
		"snmp-v2c":  false,
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

func TestGetSchema(t *testing.T) {
	registry := GetRegistry()

	testCases := []struct {
		name        string
		protocolID  string
		shouldExist bool
	}{
		{"WinRM Schema", "winrm", true},
		{"SSH Schema", "ssh", true},
		{"SNMP Schema", "snmp-v2c", true},
		{"Invalid Schema", "invalid", false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			schema, err := registry.GetSchema(tc.protocolID)

			if tc.shouldExist {
				if err != nil {
					t.Errorf("Expected schema for %s to exist, got error: %v", tc.protocolID, err)
				}
				if len(schema) == 0 {
					t.Error("Schema should not be empty")
				}
			} else {
				if err == nil {
					t.Errorf("Expected error for schema %s, but got none", tc.protocolID)
				}
			}
		})
	}
}

func TestValidateWinRMCredentials(t *testing.T) {
	testCases := []struct {
		name      string
		creds     map[string]interface{}
		shouldErr bool
	}{
		{
			"Valid WinRM",
			map[string]interface{}{"username": "admin", "password": "pass123"},
			false,
		},
		{
			"Missing username",
			map[string]interface{}{"password": "pass123"},
			true,
		},
		{
			"Missing password",
			map[string]interface{}{"username": "admin"},
			true,
		},
		{
			"Empty username",
			map[string]interface{}{"username": "", "password": "pass123"},
			true,
		},
	}

	registry := GetRegistry()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := registry.ValidateCredentials("winrm", tc.creds)

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
		creds     map[string]interface{}
		shouldErr bool
	}{
		{
			"Valid SSH with password",
			map[string]interface{}{"username": "ubuntu", "password": "pass123"},
			false,
		},
		{
			"Valid SSH with key",
			map[string]interface{}{"username": "ubuntu", "private_key": "-----BEGIN RSA PRIVATE KEY-----"},
			false,
		},
		{
			"Missing username",
			map[string]interface{}{"password": "pass123"},
			true,
		},
		{
			"Missing both password and key",
			map[string]interface{}{"username": "ubuntu"},
			true,
		},
	}

	registry := GetRegistry()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := registry.ValidateCredentials("ssh", tc.creds)

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
		creds     map[string]interface{}
		shouldErr bool
	}{
		{
			"Valid SNMP",
			map[string]interface{}{"community": "public"},
			false,
		},
		{
			"Missing community",
			map[string]interface{}{},
			true,
		},
		{
			"Empty community",
			map[string]interface{}{"community": ""},
			true,
		},
	}

	registry := GetRegistry()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := registry.ValidateCredentials("snmp-v2c", tc.creds)

			if tc.shouldErr && err == nil {
				t.Error("Expected validation error, but got none")
			}
			if !tc.shouldErr && err != nil {
				t.Errorf("Expected no error, but got: %v", err)
			}
		})
	}
}
