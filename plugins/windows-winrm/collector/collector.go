package collector

import (
	"fmt"
	"log"

	"github.com/nmslite/plugins/windows-winrm/models"
	"github.com/nmslite/plugins/windows-winrm/winrm"
)

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
