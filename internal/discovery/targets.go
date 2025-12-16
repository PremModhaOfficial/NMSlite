// Package discovery provides network target expansion and auto-detection utilities.
package discovery

import (
	"fmt"
	"net/netip"
	"strings"
)

// TargetType represents the type of network target
type TargetType string

const (
	TargetTypeCIDR    TargetType = "cidr"
	TargetTypeRange   TargetType = "range"
	TargetTypeSingle  TargetType = "ip"
	TargetTypeUnknown TargetType = "unknown"
)

// DetectTargetType automatically detects the type of target from its value.
// It checks for CIDR notation, IP range, or single IP address.
//
// Examples:
//   - "192.168.1.0/24" -> "cidr"
//   - "192.168.1.1-192.168.1.50" -> "range"
//   - "192.168.1.100" -> "ip"
//   - "invalid" -> "unknown"
func DetectTargetType(value string) TargetType {
	value = strings.TrimSpace(value)

	// Check for CIDR notation (contains "/")
	if strings.Contains(value, "/") {
		if _, err := netip.ParsePrefix(value); err == nil {
			return TargetTypeCIDR
		}
	}

	// Check for IP range (contains "-")
	if strings.Contains(value, "-") {
		// Basic validation: try to parse both ends as IPs
		parts := strings.Split(value, "-")
		if len(parts) == 2 {
			startIP := strings.TrimSpace(parts[0])
			endIP := strings.TrimSpace(parts[1])
			if _, err := netip.ParseAddr(startIP); err == nil {
				if _, err := netip.ParseAddr(endIP); err == nil {
					return TargetTypeRange
				}
			}
		}
	}

	// Check for single IP address
	if _, err := netip.ParseAddr(value); err == nil {
		return TargetTypeSingle
	}

	return TargetTypeUnknown
}

// ExpandTarget expands a network target into a list of individual IP addresses.
// It supports CIDR notation, IP ranges, and single IPs.
//
// For CIDR blocks, it expands all usable hosts (excluding network and broadcast addresses for IPv4).
// For ranges, it expands all IPs between start and end (inclusive).
// For single IPs, it returns a slice with just that IP.
//
// Returns an error if the target format is invalid or if the range is too large (>65536 IPs).
func ExpandTarget(value string) ([]string, error) {
	targetType := DetectTargetType(value)

	switch targetType {
	case TargetTypeCIDR:
		return expandCIDR(value)
	case TargetTypeRange:
		return expandRange(value)
	case TargetTypeSingle:
		return []string{strings.TrimSpace(value)}, nil
	default:
		return nil, fmt.Errorf("invalid target format: %s", value)
	}
}

// expandCIDR expands a CIDR block into individual IP addresses.
// For IPv4, it excludes the network address and broadcast address.
// For IPv6, it includes all addresses in the range.
func expandCIDR(cidr string) ([]string, error) {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR notation: %w", err)
	}

	// Calculate the number of IPs in the range
	bits := prefix.Bits()
	maxBits := 32
	if prefix.Addr().Is6() {
		maxBits = 128
	}
	hostBits := maxBits - bits

	// Prevent expansion of very large ranges
	if hostBits > 16 {
		return nil, fmt.Errorf("CIDR block too large (>65536 hosts): %s", cidr)
	}

	var ips []string
	addr := prefix.Masked().Addr()

	// For IPv4 /31 and /32, include all addresses
	// For other IPv4, skip network address (first) and broadcast (last)
	skipFirst := prefix.Addr().Is4() && bits < 31
	skipLast := prefix.Addr().Is4() && bits < 31

	if skipFirst {
		addr = addr.Next()
	}

	// Iterate through all IPs in the range
	for prefix.Contains(addr) {
		ips = append(ips, addr.String())
		addr = addr.Next()

		// Prevent infinite loops for large ranges
		if len(ips) > 65536 {
			return nil, fmt.Errorf("CIDR block expanded to more than 65536 hosts: %s", cidr)
		}
	}

	// Remove the last IP if it's the broadcast address for IPv4
	if skipLast && len(ips) > 0 {
		ips = ips[:len(ips)-1]
	}

	return ips, nil
}

// expandRange expands an IP range (e.g., "192.168.1.1-192.168.1.50") into individual IPs.
// Both start and end IPs are included in the result.
func expandRange(rangeStr string) ([]string, error) {
	parts := strings.Split(rangeStr, "-")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid IP range format (expected 'start-end'): %s", rangeStr)
	}

	startIP, err := netip.ParseAddr(strings.TrimSpace(parts[0]))
	if err != nil {
		return nil, fmt.Errorf("invalid start IP in range: %w", err)
	}

	endIP, err := netip.ParseAddr(strings.TrimSpace(parts[1]))
	if err != nil {
		return nil, fmt.Errorf("invalid end IP in range: %w", err)
	}

	// Ensure both IPs are the same version (IPv4 or IPv6)
	if startIP.Is4() != endIP.Is4() {
		return nil, fmt.Errorf("IP version mismatch: %s and %s", startIP, endIP)
	}

	// Ensure start <= end
	if startIP.Compare(endIP) > 0 {
		return nil, fmt.Errorf("start IP must be <= end IP: %s > %s", startIP, endIP)
	}

	var ips []string
	current := startIP

	// Iterate from start to end (inclusive)
	for {
		ips = append(ips, current.String())

		// Prevent expansion of very large ranges
		if len(ips) > 65536 {
			return nil, fmt.Errorf("IP range too large (>65536 hosts): %s", rangeStr)
		}

		// Break if we've reached the end
		if current.Compare(endIP) == 0 {
			break
		}

		current = current.Next()

		// Safety check: prevent infinite loops
		if !current.IsValid() {
			return nil, fmt.Errorf("IP overflow while expanding range: %s", rangeStr)
		}
	}

	return ips, nil
}

// ValidateTarget checks if a target value is valid.
// It returns nil if valid, or an error describing the issue.
func ValidateTarget(value string) error {
	targetType := DetectTargetType(value)
	if targetType == TargetTypeUnknown {
		return fmt.Errorf("invalid target format: must be a valid IP, CIDR block, or IP range")
	}

	// Try to expand to validate the format thoroughly
	_, err := ExpandTarget(value)
	return err
}

// countIPsInCIDR returns the number of usable IPs in a CIDR block
func countIPsInCIDR(cidr string) (int64, error) {
	prefix, err := netip.ParsePrefix(cidr)
	if err != nil {
		return 0, fmt.Errorf("invalid CIDR: %w", err)
	}

	bits := prefix.Bits()
	maxBits := 32
	if prefix.Addr().Is6() {
		maxBits = 128
	}
	hostBits := maxBits - bits

	count := int64(1) << hostBits

	// For IPv4, subtract network and broadcast addresses (except /31 and /32)
	if prefix.Addr().Is4() && bits < 31 {
		count -= 2
	}

	return count, nil
}

// countIPsInRange returns the number of IPs in a range
func countIPsInRange(rangeStr string) (int64, error) {
	parts := strings.Split(rangeStr, "-")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid IP range format")
	}

	startIP, err := netip.ParseAddr(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, fmt.Errorf("invalid start IP: %w", err)
	}

	endIP, err := netip.ParseAddr(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, fmt.Errorf("invalid end IP: %w", err)
	}

	if startIP.Is4() != endIP.Is4() {
		return 0, fmt.Errorf("IP version mismatch")
	}

	if startIP.Compare(endIP) > 0 {
		return 0, fmt.Errorf("start IP > end IP")
	}

	// For IPv4, we can calculate directly
	if startIP.Is4() {
		startBytes := startIP.As4()
		endBytes := endIP.As4()
		startNum := int64(startBytes[0])<<24 | int64(startBytes[1])<<16 | int64(startBytes[2])<<8 | int64(startBytes[3])
		endNum := int64(endBytes[0])<<24 | int64(endBytes[1])<<16 | int64(endBytes[2])<<8 | int64(endBytes[3])
		return endNum - startNum + 1, nil
	}

	// For IPv6, we need to iterate (less efficient but safer)
	count := int64(1)
	current := startIP
	for current.Compare(endIP) < 0 {
		current = current.Next()
		count++
		if count > 65536 {
			return count, fmt.Errorf("range too large")
		}
	}

	return count, nil
}

// GetTargetInfo returns human-readable information about a target
func GetTargetInfo(value string) string {
	targetType := DetectTargetType(value)

	switch targetType {
	case TargetTypeCIDR:
		count, err := countIPsInCIDR(value)
		if err != nil {
			return fmt.Sprintf("CIDR: %s (invalid: %v)", value, err)
		}
		return fmt.Sprintf("CIDR: %s (%d IPs)", value, count)
	case TargetTypeRange:
		count, err := countIPsInRange(value)
		if err != nil {
			return fmt.Sprintf("Range: %s (invalid: %v)", value, err)
		}
		return fmt.Sprintf("Range: %s (%d IPs)", value, count)
	case TargetTypeSingle:
		return fmt.Sprintf("Single IP: %s", value)
	default:
		return fmt.Sprintf("Unknown: %s", value)
	}
}
