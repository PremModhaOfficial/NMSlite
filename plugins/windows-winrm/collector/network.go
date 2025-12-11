package collector

import (
	"encoding/json"
	"fmt"

	"github.com/nmslite/plugins/windows-winrm/models"
	"github.com/nmslite/plugins/windows-winrm/winrm"
)

// NetworkData represents the WMI output for network interface performance
type NetworkData struct {
	Name                string `json:"Name"`
	BytesReceivedPersec uint64 `json:"BytesReceivedPersec"`
	BytesSentPersec     uint64 `json:"BytesSentPersec"`
	CurrentBandwidth    uint64 `json:"CurrentBandwidth"` // bits per second
}

// CollectNetwork queries Win32_PerfFormattedData_Tcpip_NetworkInterface
// and returns per-interface network metrics with separate in/out direction entries
func CollectNetwork(client *winrm.Client) ([]models.Metric, error) {
	// PowerShell command to get network interface stats
	script := `Get-WmiObject Win32_PerfFormattedData_Tcpip_NetworkInterface | Select-Object Name, BytesReceivedPersec, BytesSentPersec, CurrentBandwidth | ConvertTo-Json -Compress`

	output, err := client.RunPowerShell(script)
	if err != nil {
		return nil, fmt.Errorf("failed to collect network metrics: %w", err)
	}

	if output == "" {
		return nil, fmt.Errorf("no network data returned")
	}

	// Parse JSON output - can be single object or array
	var netDataList []NetworkData

	// Try parsing as array first
	if err := json.Unmarshal([]byte(output), &netDataList); err != nil {
		// Try parsing as single object (single interface system)
		var singleNet NetworkData
		if err := json.Unmarshal([]byte(output), &singleNet); err != nil {
			return nil, fmt.Errorf("failed to parse network data: %w, raw output: %s", err, output)
		}
		netDataList = []NetworkData{singleNet}
	}

	// Convert to metrics - TWO entries per interface (in + out)
	metrics := make([]models.Metric, 0, len(netDataList)*2)

	for _, net := range netDataList {
		// Calculate link speed in bytes/sec (CurrentBandwidth is in bits/sec)
		// Use nil if bandwidth is 0 (unknown)
		var linkSpeedBytes *float64
		if net.CurrentBandwidth > 0 {
			speed := float64(net.CurrentBandwidth) / 8
			linkSpeedBytes = &speed
		}

		// Inbound metric
		metrics = append(metrics, models.Metric{
			MetricGroup: "net.interface",
			Tags: map[string]string{
				"interface": net.Name,
				"direction": "in",
			},
			ValUsed:  float64(net.BytesReceivedPersec),
			ValTotal: linkSpeedBytes,
		})

		// Outbound metric
		metrics = append(metrics, models.Metric{
			MetricGroup: "net.interface",
			Tags: map[string]string{
				"interface": net.Name,
				"direction": "out",
			},
			ValUsed:  float64(net.BytesSentPersec),
			ValTotal: linkSpeedBytes,
		})
	}

	return metrics, nil
}
