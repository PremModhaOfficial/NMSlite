package protocols

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStrictValidation(t *testing.T) {
	registry := GetRegistry()

	tests := []struct {
		name        string
		protocol    string
		payload     string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Valid SSH",
			protocol:    "ssh",
			payload:     `{"username": "admin", "password": "password"}`,
			expectError: false,
		},
		{
			name:        "SSH with Port (Should Fail)",
			protocol:    "ssh",
			payload:     `{"username": "admin", "password": "password", "port": 22}`,
			expectError: true,
			errorMsg:    "contains forbidden field",
		},
		{
			name:        "SSH with Unknown Field (Should Fail)",
			protocol:    "ssh",
			payload:     `{"username": "admin", "password": "password", "extra": "value"}`,
			expectError: true,
			errorMsg:    "contains forbidden field",
		},
		{
			name:        "Valid WinRM",
			protocol:    "winrm",
			payload:     `{"username": "admin", "password": "password"}`,
			expectError: false,
		},
		{
			name:        "WinRM with Port (Should Fail)",
			protocol:    "winrm",
			payload:     `{"username": "admin", "password": "password", "port": 5985}`,
			expectError: true,
			errorMsg:    "contains forbidden field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := registry.ValidateCredentials(tt.protocol, json.RawMessage(tt.payload))
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errorMsg)
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
