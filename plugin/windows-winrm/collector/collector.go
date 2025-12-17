package collector

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/nmslite/plugins/windows-winrm/models"
	"github.com/nmslite/plugins/windows-winrm/winrm"
)

// executeWMIQuery runs a PowerShell script and parses JSON output into a slice of T.
// If singleFallback is true, it will try parsing as a single object if array parsing fails.
// This handles the common WMI pattern where single results return an object, not an array.
func executeWMIQuery[T any](client *winrm.Client, script string, singleFallback bool) ([]T, error) {
	output, err := client.RunPowerShell(script)
	if err != nil {
		return nil, fmt.Errorf("WMI query failed: %w", err)
	}
	if output == "" {
		return nil, fmt.Errorf("no data returned")
	}

	var results []T
	if err := json.Unmarshal([]byte(output), &results); err != nil {
		if singleFallback {
			var single T
			if err := json.Unmarshal([]byte(output), &single); err != nil {
				return nil, fmt.Errorf("parse failed: %w, raw: %s", err, output)
			}
			return []T{single}, nil
		}
		return nil, fmt.Errorf("parse failed: %w", err)
	}
	return results, nil
}

// Collect runs all metric collectors and returns combined results
// Uses partial success strategy - if one collector fails, others continue
func Collect(client *winrm.Client) ([]models.Metric, error) {
	var allMetrics []models.Metric
	var errors []string

	// Collect CPU metrics
	cpuMetrics, err := CollectCPU(client)
	if err != nil {
		log.Printf("[WARN] CPU collection failed for %s: %v", client.Target(), err)
		errors = append(errors, fmt.Sprintf("cpu: %v", err))
	} else {
		allMetrics = append(allMetrics, cpuMetrics...)
	}

	// Collect Memory metrics
	memMetrics, err := CollectMemory(client)
	if err != nil {
		log.Printf("[WARN] Memory collection failed for %s: %v", client.Target(), err)
		errors = append(errors, fmt.Sprintf("memory: %v", err))
	} else {
		allMetrics = append(allMetrics, memMetrics...)
	}

	// Collect Disk metrics
	diskMetrics, err := CollectDisk(client)
	if err != nil {
		log.Printf("[WARN] Disk collection failed for %s: %v", client.Target(), err)
		errors = append(errors, fmt.Sprintf("disk: %v", err))
	} else {
		allMetrics = append(allMetrics, diskMetrics...)
	}

	// Collect Network metrics
	netMetrics, err := CollectNetwork(client)
	if err != nil {
		log.Printf("[WARN] Network collection failed for %s: %v", client.Target(), err)
		errors = append(errors, fmt.Sprintf("network: %v", err))
	} else {
		allMetrics = append(allMetrics, netMetrics...)
	}

	// If all collectors failed, return error
	if len(allMetrics) == 0 && len(errors) > 0 {
		return nil, fmt.Errorf("all collectors failed: %v", errors)
	}

	// If we got some metrics, return them (partial success)
	return allMetrics, nil
}

// -------------------------------------------------------------------------
// CPU Collector
// -------------------------------------------------------------------------

// CPUData represents the WMI output for processor performance
type CPUData struct {
	Name                 string `json:"Name"`
	PercentProcessorTime uint64 `json:"PercentProcessorTime"`
}

// CollectCPU queries Win32_PerfFormattedData_PerfOS_Processor and returns per-core CPU metrics
// Includes both per-core metrics and the aggregate total
func CollectCPU(client *winrm.Client) ([]models.Metric, error) {
	script := `Get-WmiObject Win32_PerfFormattedData_PerfOS_Processor | Select-Object Name, PercentProcessorTime | ConvertTo-Json -Compress`

	cpuData, err := executeWMIQuery[CPUData](client, script, true)
	if err != nil {
		return nil, fmt.Errorf("CPU collection failed: %w", err)
	}

	metrics := make([]models.Metric, 0, len(cpuData)+1)
	for _, cpu := range cpuData {
		if cpu.Name == "_Total" {
			// Aggregate CPU usage
			metrics = append(metrics, models.Metric{
				Name:  "system.cpu.usage",
				Value: float64(cpu.PercentProcessorTime),
				Type:  "gauge",
			})
		} else {
			// Per-core CPU usage
			metrics = append(metrics, models.Metric{
				Name:  fmt.Sprintf("system.cpu.%s.usage", cpu.Name),
				Value: float64(cpu.PercentProcessorTime),
				Type:  "gauge",
			})
		}
	}
	return metrics, nil
}

// -------------------------------------------------------------------------
// Memory Collector
// -------------------------------------------------------------------------

// MemoryData represents the WMI output for operating system memory info
// Note: WMI returns values in KB
type MemoryData struct {
	TotalVisibleMemorySize uint64 `json:"TotalVisibleMemorySize"`
	FreePhysicalMemory     uint64 `json:"FreePhysicalMemory"`
}

// CollectMemory queries Win32_OperatingSystem and returns memory usage metrics
// Values are converted from KB to bytes
func CollectMemory(client *winrm.Client) ([]models.Metric, error) {
	script := `Get-WmiObject Win32_OperatingSystem | Select-Object TotalVisibleMemorySize, FreePhysicalMemory | ConvertTo-Json -Compress`

	memData, err := executeWMIQuery[MemoryData](client, script, true)
	if err != nil {
		return nil, fmt.Errorf("memory collection failed: %w", err)
	}
	if len(memData) == 0 {
		return nil, fmt.Errorf("no memory data returned")
	}

	mem := memData[0]
	totalBytes := float64(mem.TotalVisibleMemorySize) * 1024
	freeBytes := float64(mem.FreePhysicalMemory) * 1024
	usedBytes := totalBytes - freeBytes
	usagePercent := (usedBytes / totalBytes) * 100

	return []models.Metric{
		{Name: "system.memory.total_bytes", Value: totalBytes, Type: "gauge"},
		{Name: "system.memory.used_bytes", Value: usedBytes, Type: "gauge"},
		{Name: "system.memory.free_bytes", Value: freeBytes, Type: "gauge"},
		{Name: "system.memory.usage_percent", Value: usagePercent, Type: "gauge"},
	}, nil
}

// -------------------------------------------------------------------------
// Disk Collector
// -------------------------------------------------------------------------

// DiskData represents the WMI output for logical disk info
type DiskData struct {
	DeviceID  string `json:"DeviceID"`
	Size      uint64 `json:"Size"`
	FreeSpace uint64 `json:"FreeSpace"`
}

// CollectDisk queries Win32_LogicalDisk and returns per-mount disk usage metrics
// Only fixed drives (DriveType=3) are included. Also returns aggregate totals.
func CollectDisk(client *winrm.Client) ([]models.Metric, error) {
	script := `Get-WmiObject Win32_LogicalDisk -Filter "DriveType=3" | Select-Object DeviceID, Size, FreeSpace | ConvertTo-Json -Compress`

	diskData, err := executeWMIQuery[DiskData](client, script, true)
	if err != nil {
		return nil, fmt.Errorf("disk collection failed: %w", err)
	}

	var metrics []models.Metric
	var aggTotal, aggFree float64

	for _, disk := range diskData {
		if disk.Size == 0 {
			continue
		}

		totalBytes := float64(disk.Size)
		freeBytes := float64(disk.FreeSpace)
		usedBytes := totalBytes - freeBytes
		usagePercent := (usedBytes / totalBytes) * 100
		deviceName := strings.ToLower(strings.TrimSuffix(disk.DeviceID, ":"))

		// Per-disk metrics
		metrics = append(metrics,
			models.Metric{Name: fmt.Sprintf("system.disk.%s.total_bytes", deviceName), Value: totalBytes, Type: "gauge"},
			models.Metric{Name: fmt.Sprintf("system.disk.%s.used_bytes", deviceName), Value: usedBytes, Type: "gauge"},
			models.Metric{Name: fmt.Sprintf("system.disk.%s.free_bytes", deviceName), Value: freeBytes, Type: "gauge"},
			models.Metric{Name: fmt.Sprintf("system.disk.%s.usage_percent", deviceName), Value: usagePercent, Type: "gauge"},
		)

		// Accumulate for aggregates
		aggTotal += totalBytes
		aggFree += freeBytes
	}

	// Aggregate disk metrics
	if aggTotal > 0 {
		aggUsed := aggTotal - aggFree
		aggUsagePercent := (aggUsed / aggTotal) * 100
		metrics = append(metrics,
			models.Metric{Name: "system.disk.total_bytes", Value: aggTotal, Type: "gauge"},
			models.Metric{Name: "system.disk.used_bytes", Value: aggUsed, Type: "gauge"},
			models.Metric{Name: "system.disk.free_bytes", Value: aggFree, Type: "gauge"},
			models.Metric{Name: "system.disk.usage_percent", Value: aggUsagePercent, Type: "gauge"},
		)
	}

	return metrics, nil
}

// -------------------------------------------------------------------------
// Network Collector
// -------------------------------------------------------------------------

// NetworkData represents the WMI output for network interface performance
type NetworkData struct {
	Name                string `json:"Name"`
	BytesReceivedPersec uint64 `json:"BytesReceivedPersec"`
	BytesSentPersec     uint64 `json:"BytesSentPersec"`
	CurrentBandwidth    uint64 `json:"CurrentBandwidth"` // bits per second
}

// CollectNetwork queries Win32_PerfFormattedData_Tcpip_NetworkInterface
// and returns per-interface network metrics with separate in/out direction entries.
// Also returns aggregate totals across all interfaces.
func CollectNetwork(client *winrm.Client) ([]models.Metric, error) {
	script := `Get-WmiObject Win32_PerfFormattedData_Tcpip_NetworkInterface | Select-Object Name, BytesReceivedPersec, BytesSentPersec, CurrentBandwidth | ConvertTo-Json -Compress`

	netData, err := executeWMIQuery[NetworkData](client, script, true)
	if err != nil {
		return nil, fmt.Errorf("network collection failed: %w", err)
	}

	var metrics []models.Metric
	var aggRecv, aggSent, aggBandwidth float64

	for _, net := range netData {
		ifaceName := sanitizeInterfaceName(net.Name)
		linkSpeedBytes := float64(net.CurrentBandwidth) / 8

		// Per-interface metrics
		metrics = append(metrics,
			models.Metric{Name: fmt.Sprintf("network.%s.bytes_recv_per_sec", ifaceName), Value: float64(net.BytesReceivedPersec), Type: "gauge"},
			models.Metric{Name: fmt.Sprintf("network.%s.bytes_sent_per_sec", ifaceName), Value: float64(net.BytesSentPersec), Type: "gauge"},
		)
		if linkSpeedBytes > 0 {
			metrics = append(metrics,
				models.Metric{Name: fmt.Sprintf("network.%s.bandwidth_bytes", ifaceName), Value: linkSpeedBytes, Type: "gauge"},
			)
		}

		// Accumulate for aggregates
		aggRecv += float64(net.BytesReceivedPersec)
		aggSent += float64(net.BytesSentPersec)
		aggBandwidth += linkSpeedBytes
	}

	// Aggregate network metrics
	metrics = append(metrics,
		models.Metric{Name: "network.bytes_recv_per_sec", Value: aggRecv, Type: "gauge"},
		models.Metric{Name: "network.bytes_sent_per_sec", Value: aggSent, Type: "gauge"},
	)
	if aggBandwidth > 0 {
		metrics = append(metrics,
			models.Metric{Name: "network.bandwidth_bytes", Value: aggBandwidth, Type: "gauge"},
		)
	}

	return metrics, nil
}

// sanitizeInterfaceName converts interface names to safe metric names
func sanitizeInterfaceName(name string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	result := re.ReplaceAllString(name, "_")
	result = strings.Trim(result, "_")
	return strings.ToLower(result)
}
