package main

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"time"

	"github.com/nmslite/plugins/windows-winrm/collector"
	"github.com/nmslite/plugins/windows-winrm/models"
	"github.com/nmslite/plugins/windows-winrm/winrm"
)

const (
	// DefaultTimeout for WinRM connections
	DefaultTimeout = 30 * time.Second
)

func main() {
	// Disable log output to stdout (we use stdout for JSON output)
	// Logs go to stderr
	log.SetOutput(os.Stderr)
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// Read all input from STDIN
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		log.Fatalf("Failed to read STDIN: %v", err)
	}

	// Handle empty input
	if len(input) == 0 {
		log.Fatal("No input received on STDIN")
	}

	// Parse input as JSON array of polling tasks
	var tasks []models.PluginInput
	if err := json.Unmarshal(input, &tasks); err != nil {
		log.Fatalf("Failed to parse input JSON: %v", err)
	}

	// Process each task
	outputs := make([]models.PluginOutput, len(tasks))
	for i, task := range tasks {
		outputs[i] = processTask(task)
	}

	// Write JSON array to STDOUT
	encoder := json.NewEncoder(os.Stdout)
	if err := encoder.Encode(outputs); err != nil {
		log.Fatalf("Failed to write output JSON: %v", err)
	}
}

// processTask handles a single polling task
func processTask(task models.PluginInput) models.PluginOutput {
	// Default port if not specified
	port := task.Port
	if port == 0 {
		if task.Credentials.UseHTTPS {
			port = 5986
		} else {
			port = 5985
		}
	}

	// Create WinRM client
	client, err := winrm.NewClient(task.Target, port, task.Credentials, DefaultTimeout)
	if err != nil {
		return models.PluginOutput{
			RequestID: task.RequestID,
			Status:    "failed",
			Error:     "WinRM connection failed: " + err.Error(),
		}
	}
	defer client.Close()

	// Collect all metrics
	metrics, err := collector.Collect(client)
	if err != nil {
		return models.PluginOutput{
			RequestID: task.RequestID,
			Status:    "failed",
			Error:     "Metric collection failed: " + err.Error(),
		}
	}

	// Return success with metrics
	return models.PluginOutput{
		RequestID: task.RequestID,
		Status:    "success",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Metrics:   metrics,
	}
}
