package database

import (
	"fmt"
	"net/netip"
)

// StringToInet converts an IP address string to netip.Addr
func StringToInet(ipStr string) (netip.Addr, error) {
	if ipStr == "" {
		return netip.Addr{}, fmt.Errorf("IP address string cannot be empty")
	}

	// Parse the IP address
	addr, err := netip.ParseAddr(ipStr)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("invalid IP address: %s", ipStr)
	}

	return addr, nil
}

// InetToString converts netip.Addr to an IP address string
func InetToString(addr netip.Addr) string {
	if !addr.IsValid() {
		return ""
	}

	return addr.String()
}
