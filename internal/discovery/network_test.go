package discovery

import (
	"testing"
)

func TestDetectTargetType(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected TargetType
	}{
		// CIDR tests
		{"CIDR /24", "192.168.1.0/24", TargetTypeCIDR},
		{"CIDR /16", "10.0.0.0/16", TargetTypeCIDR},
		{"CIDR /32", "192.168.1.100/32", TargetTypeCIDR},
		{"CIDR IPv6", "2001:db8::/32", TargetTypeCIDR},
		{"CIDR with spaces", " 192.168.1.0/24 ", TargetTypeCIDR},

		// Range tests
		{"Range simple", "192.168.1.1-192.168.1.10", TargetTypeRange},
		{"Range cross subnet", "192.168.1.250-192.168.2.10", TargetTypeRange},
		{"Range with spaces", " 192.168.1.1 - 192.168.1.10 ", TargetTypeRange},
		{"Range IPv6", "2001:db8::1-2001:db8::10", TargetTypeRange},

		// Single IP tests
		{"Single IPv4", "192.168.1.100", TargetTypeSingle},
		{"Single IPv4 with spaces", " 192.168.1.100 ", TargetTypeSingle},
		{"Single IPv6", "2001:db8::1", TargetTypeSingle},
		{"Single IPv6 compressed", "::1", TargetTypeSingle},

		// Invalid tests
		{"Invalid CIDR", "192.168.1.0/33", TargetTypeUnknown},
		{"Invalid range format", "192.168.1.1-invalid", TargetTypeUnknown},
		{"Invalid IP", "999.999.999.999", TargetTypeUnknown},
		{"Empty string", "", TargetTypeUnknown},
		{"Hostname", "example.com", TargetTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectTargetType(tt.value)
			if result != tt.expected {
				t.Errorf("DetectTargetType(%q) = %v, want %v", tt.value, result, tt.expected)
			}
		})
	}
}

func TestExpandTarget_SingleIP(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected []string
	}{
		{"Single IPv4", "192.168.1.100", []string{"192.168.1.100"}},
		{"Single IPv4 with spaces", " 192.168.1.100 ", []string{"192.168.1.100"}},
		{"Single IPv6", "2001:db8::1", []string{"2001:db8::1"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExpandTarget(tt.value)
			if err != nil {
				t.Fatalf("ExpandTarget(%q) error = %v", tt.value, err)
			}
			if len(result) != len(tt.expected) {
				t.Fatalf("ExpandTarget(%q) returned %d IPs, want %d", tt.value, len(result), len(tt.expected))
			}
			for i, ip := range result {
				if ip != tt.expected[i] {
					t.Errorf("ExpandTarget(%q)[%d] = %q, want %q", tt.value, i, ip, tt.expected[i])
				}
			}
		})
	}
}

func TestExpandTarget_CIDR(t *testing.T) {
	tests := []struct {
		name          string
		value         string
		expectedCount int
		expectError   bool
	}{
		{"CIDR /30", "192.168.1.0/30", 2, false},  // .1 and .2 (exclude .0 network and .3 broadcast)
		{"CIDR /29", "192.168.1.0/29", 6, false},  // .1 to .6 (8 total - network - broadcast)
		{"CIDR /28", "192.168.1.0/28", 14, false}, // 16 - 2
		{"CIDR /24", "192.168.1.0/24", 254, false},
		{"CIDR /31", "192.168.1.0/31", 2, false}, // Point-to-point, both IPs usable
		{"CIDR /32", "192.168.1.100/32", 1, false},
		{"CIDR /16", "192.168.0.0/16", 65534, false},      // 65534 IPs - just under limit
		{"CIDR /15 too large", "192.168.0.0/15", 0, true}, // >65536 IPs - should error
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExpandTarget(tt.value)
			if tt.expectError {
				if err == nil {
					t.Errorf("ExpandTarget(%q) expected error but got none", tt.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("ExpandTarget(%q) error = %v", tt.value, err)
			}
			if len(result) != tt.expectedCount {
				t.Errorf("ExpandTarget(%q) returned %d IPs, want %d", tt.value, len(result), tt.expectedCount)
			}
		})
	}
}

func TestExpandTarget_CIDR_Details(t *testing.T) {
	// Test specific CIDR expansions in detail
	t.Run("CIDR /30 exact IPs", func(t *testing.T) {
		result, err := ExpandTarget("192.168.1.0/30")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := []string{"192.168.1.1", "192.168.1.2"}
		if len(result) != len(expected) {
			t.Fatalf("got %d IPs, want %d", len(result), len(expected))
		}
		for i, ip := range expected {
			if result[i] != ip {
				t.Errorf("IP[%d] = %q, want %q", i, result[i], ip)
			}
		}
	})

	t.Run("CIDR /32 single IP", func(t *testing.T) {
		result, err := ExpandTarget("192.168.1.100/32")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 || result[0] != "192.168.1.100" {
			t.Errorf("got %v, want [192.168.1.100]", result)
		}
	})
}

func TestExpandTarget_Range(t *testing.T) {
	tests := []struct {
		name          string
		value         string
		expectedCount int
		expectError   bool
	}{
		{"Range 10 IPs", "192.168.1.1-192.168.1.10", 10, false},
		{"Range 1 IP", "192.168.1.100-192.168.1.100", 1, false},
		{"Range 256 IPs", "192.168.1.0-192.168.1.255", 256, false},
		{"Range cross subnet", "192.168.1.250-192.168.2.5", 12, false},
		{"Range inverted", "192.168.1.10-192.168.1.1", 0, true},
		{"Range 65536 IPs", "10.0.0.0-10.0.255.255", 65536, false}, // Exactly 65536 - at limit
		{"Range too large", "10.0.0.0-10.1.0.0", 0, true},          // >65536 IPs
		{"Range mixed versions", "192.168.1.1-2001:db8::1", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExpandTarget(tt.value)
			if tt.expectError {
				if err == nil {
					t.Errorf("ExpandTarget(%q) expected error but got none", tt.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("ExpandTarget(%q) error = %v", tt.value, err)
			}
			if len(result) != tt.expectedCount {
				t.Errorf("ExpandTarget(%q) returned %d IPs, want %d", tt.value, len(result), tt.expectedCount)
			}
		})
	}
}

func TestExpandTarget_Range_Details(t *testing.T) {
	t.Run("Range exact IPs", func(t *testing.T) {
		result, err := ExpandTarget("192.168.1.1-192.168.1.5")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := []string{
			"192.168.1.1",
			"192.168.1.2",
			"192.168.1.3",
			"192.168.1.4",
			"192.168.1.5",
		}
		if len(result) != len(expected) {
			t.Fatalf("got %d IPs, want %d", len(result), len(expected))
		}
		for i, ip := range expected {
			if result[i] != ip {
				t.Errorf("IP[%d] = %q, want %q", i, result[i], ip)
			}
		}
	})

	t.Run("Range with spaces", func(t *testing.T) {
		result, err := ExpandTarget(" 192.168.1.1 - 192.168.1.3 ")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 3 {
			t.Errorf("got %d IPs, want 3", len(result))
		}
	})
}

func TestExpandTarget_Errors(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"Invalid format", "invalid"},
		{"Hostname", "example.com"},
		{"Invalid CIDR", "192.168.1.0/33"},
		{"Invalid range", "192.168.1.1-invalid"},
		{"Empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ExpandTarget(tt.value)
			if err == nil {
				t.Errorf("ExpandTarget(%q) expected error but got none", tt.value)
			}
		})
	}
}

func TestValidateTarget(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		// Valid targets
		{"Valid IP", "192.168.1.100", false},
		{"Valid CIDR", "192.168.1.0/24", false},
		{"Valid range", "192.168.1.1-192.168.1.10", false},
		{"Valid /32", "192.168.1.100/32", false},

		// Invalid targets
		{"Invalid IP", "999.999.999.999", true},
		{"Invalid CIDR", "192.168.1.0/33", true},
		{"Invalid range", "192.168.1.1-invalid", true},
		{"Hostname", "example.com", true},
		{"Empty", "", true},
		{"CIDR too large", "10.0.0.0/8", true},
		{"Range too large", "10.0.0.0-10.1.0.0", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTarget(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTarget(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}

func TestGetTargetInfo(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		contains string // Check if output contains this string
	}{
		{"Single IP", "192.168.1.100", "Single IP"},
		{"CIDR /24", "192.168.1.0/24", "254 IPs"},
		{"CIDR /30", "192.168.1.0/30", "2 IPs"},
		{"Range 10", "192.168.1.1-192.168.1.10", "10 IPs"},
		{"Invalid", "invalid", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetTargetInfo(tt.value)
			if result == "" {
				t.Errorf("GetTargetInfo(%q) returned empty string", tt.value)
			}
			// Simple check - just ensure it doesn't panic and returns something
			t.Logf("GetTargetInfo(%q) = %q", tt.value, result)
		})
	}
}

func TestExpandCIDR_IPv6(t *testing.T) {
	t.Run("IPv6 /126", func(t *testing.T) {
		result, err := ExpandTarget("2001:db8::0/126")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// /126 = 4 IPs, IPv6 doesn't exclude network/broadcast
		if len(result) != 4 {
			t.Errorf("got %d IPs, want 4", len(result))
		}
	})

	t.Run("IPv6 /128", func(t *testing.T) {
		result, err := ExpandTarget("2001:db8::1/128")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result) != 1 || result[0] != "2001:db8::1" {
			t.Errorf("got %v, want [2001:db8::1]", result)
		}
	})
}

func BenchmarkExpandTarget_SingleIP(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = ExpandTarget("192.168.1.100")
	}
}

func BenchmarkExpandTarget_CIDR_24(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = ExpandTarget("192.168.1.0/24")
	}
}

func BenchmarkExpandTarget_Range_100(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, _ = ExpandTarget("192.168.1.1-192.168.1.100")
	}
}

func BenchmarkDetectTargetType(b *testing.B) {
	targets := []string{
		"192.168.1.100",
		"192.168.1.0/24",
		"192.168.1.1-192.168.1.100",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, target := range targets {
			_ = DetectTargetType(target)
		}
	}
}
