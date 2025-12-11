package collector

import (
	"encoding/json"
	"fmt"

	"github.com/nmslite/plugins/windows-winrm/models"
	"github.com/nmslite/plugins/windows-winrm/winrm"
)

// CPUData represents the WMI output for processor performance
type CPUData struct {
	Name                 string `json:"Name"`
	PercentProcessorTime uint64 `json:"PercentProcessorTime"`
}

// CollectCPU queries Win32_PerfFormattedData_PerfOS_Processor and returns per-core CPU metrics
// Excludes the "_Total" aggregate entry
func CollectCPU(client *winrm.Client) ([]models.Metric, error) {
	// PowerShell command to get per-core CPU usage
	// Excludes _Total and outputs as JSON
	script := `Get-WmiObject Win32_PerfFormattedData_PerfOS_Processor | Where-Object { $_.Name -ne '_Total' } | Select-Object Name, PercentProcessorTime | ConvertTo-Json -Compress`

	output, err := client.RunPowerShell(script)
	if err != nil {
		return nil, fmt.Errorf("failed to collect CPU metrics: %w", err)
	}

	if output == "" {
		return nil, fmt.Errorf("no CPU data returned")
	}

	// Parse JSON output - can be single object or array
	var cpuDataList []CPUData

	// Try parsing as array first
	if err := json.Unmarshal([]byte(output), &cpuDataList); err != nil {
		// Try parsing as single object (single-core system)
		var singleCPU CPUData
		if err := json.Unmarshal([]byte(output), &singleCPU); err != nil {
			return nil, fmt.Errorf("failed to parse CPU data: %w, raw output: %s", err, output)
		}
		cpuDataList = []CPUData{singleCPU}
	}

	// Convert to metrics
	metrics := make([]models.Metric, 0, len(cpuDataList))
	totalVal := 100.0

	for _, cpu := range cpuDataList {
		metrics = append(metrics, models.Metric{
			MetricGroup: "host.cpu",
			Tags:        map[string]string{"core": cpu.Name},
			ValUsed:     float64(cpu.PercentProcessorTime),
			ValTotal:    models.Float64Ptr(totalVal),
		})
	}

	return metrics, nil
}
