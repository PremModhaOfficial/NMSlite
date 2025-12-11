package collector

import (
	"encoding/json"
	"fmt"

	"github.com/nmslite/plugins/windows-winrm/models"
	"github.com/nmslite/plugins/windows-winrm/winrm"
)

// MemoryData represents the WMI output for operating system memory info
// Note: WMI returns values in KB
type MemoryData struct {
	TotalVisibleMemorySize uint64 `json:"TotalVisibleMemorySize"`
	FreePhysicalMemory     uint64 `json:"FreePhysicalMemory"`
}

// CollectMemory queries Win32_OperatingSystem and returns memory usage metrics
// Values are converted from KB to bytes
func CollectMemory(client *winrm.Client) ([]models.Metric, error) {
	// PowerShell command to get memory info
	script := `Get-WmiObject Win32_OperatingSystem | Select-Object TotalVisibleMemorySize, FreePhysicalMemory | ConvertTo-Json -Compress`

	output, err := client.RunPowerShell(script)
	if err != nil {
		return nil, fmt.Errorf("failed to collect memory metrics: %w", err)
	}

	if output == "" {
		return nil, fmt.Errorf("no memory data returned")
	}

	// Parse JSON output
	var memData MemoryData
	if err := json.Unmarshal([]byte(output), &memData); err != nil {
		return nil, fmt.Errorf("failed to parse memory data: %w, raw output: %s", err, output)
	}

	// Convert KB to bytes
	totalBytes := float64(memData.TotalVisibleMemorySize) * 1024
	freeBytes := float64(memData.FreePhysicalMemory) * 1024
	usedBytes := totalBytes - freeBytes

	metrics := []models.Metric{
		{
			MetricGroup: "host.memory",
			Tags:        map[string]string{},
			ValUsed:     usedBytes,
			ValTotal:    models.Float64Ptr(totalBytes),
		},
	}

	return metrics, nil
}
