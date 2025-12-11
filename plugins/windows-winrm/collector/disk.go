package collector

import (
	"encoding/json"
	"fmt"

	"github.com/nmslite/plugins/windows-winrm/models"
	"github.com/nmslite/plugins/windows-winrm/winrm"
)

// DiskData represents the WMI output for logical disk info
type DiskData struct {
	DeviceID  string `json:"DeviceID"`
	Size      uint64 `json:"Size"`
	FreeSpace uint64 `json:"FreeSpace"`
}

// CollectDisk queries Win32_LogicalDisk and returns per-mount disk usage metrics
// Only fixed drives (DriveType=3) are included
func CollectDisk(client *winrm.Client) ([]models.Metric, error) {
	// PowerShell command to get fixed disk info
	// DriveType=3 means fixed local disk
	script := `Get-WmiObject Win32_LogicalDisk -Filter "DriveType=3" | Select-Object DeviceID, Size, FreeSpace | ConvertTo-Json -Compress`

	output, err := client.RunPowerShell(script)
	if err != nil {
		return nil, fmt.Errorf("failed to collect disk metrics: %w", err)
	}

	if output == "" {
		return nil, fmt.Errorf("no disk data returned")
	}

	// Parse JSON output - can be single object or array
	var diskDataList []DiskData

	// Try parsing as array first
	if err := json.Unmarshal([]byte(output), &diskDataList); err != nil {
		// Try parsing as single object (single disk system)
		var singleDisk DiskData
		if err := json.Unmarshal([]byte(output), &singleDisk); err != nil {
			return nil, fmt.Errorf("failed to parse disk data: %w, raw output: %s", err, output)
		}
		diskDataList = []DiskData{singleDisk}
	}

	// Convert to metrics
	metrics := make([]models.Metric, 0, len(diskDataList))

	for i, disk := range diskDataList {
		// Skip disks with no size (can happen with unmounted volumes)
		if disk.Size == 0 {
			continue
		}

		totalBytes := float64(disk.Size)
		freeBytes := float64(disk.FreeSpace)
		usedBytes := totalBytes - freeBytes

		// Device name derived from index (disk0, disk1, etc.)
		deviceName := fmt.Sprintf("disk%d", i)

		metrics = append(metrics, models.Metric{
			MetricGroup: "host.storage",
			Tags: map[string]string{
				"mount":  disk.DeviceID,
				"device": deviceName,
			},
			ValUsed:  usedBytes,
			ValTotal: models.Float64Ptr(totalBytes),
		})
	}

	return metrics, nil
}
