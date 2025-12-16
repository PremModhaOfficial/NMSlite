package pluginManager

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"time"
)

// Executor runs plugin binaries via STDIN/STDOUT
type Executor struct {
	registry *Registry
	timeout  time.Duration
	logger   *slog.Logger
}

// NewExecutor creates a new plugin executor
func NewExecutor(registry *Registry, timeout time.Duration) *Executor {
	return &Executor{
		registry: registry,
		timeout:  timeout,
		logger:   slog.Default().With("component", "plugin_executor"),
	}
}

// Poll runs plugin with mode="poll" (batch mode)
func (e *Executor) Poll(ctx context.Context, pluginID string, tasks []PollTask) ([]PollResult, error) {
	// Get plugin info
	plugin, ok := e.registry.GetByID(pluginID)
	if !ok {
		return nil, fmt.Errorf("plugin not found: %s", pluginID)
	}

	// Marshal tasks to JSON (as array)
	inputJSON, err := json.Marshal(tasks)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal poll tasks: %w", err)
	}

	// Create context with timeout
	execCtx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()

	// Execute plugin
	cmd := exec.CommandContext(execCtx, plugin.BinaryPath)
	cmd.Stdin = bytes.NewReader(inputJSON)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	e.logger.Debug("Executing plugin poll",
		"plugin", pluginID,
		"task_count", len(tasks),
	)

	err = cmd.Run()

	// Check for timeout
	if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
		return nil, fmt.Errorf("plugin execution timed out after %v", e.timeout)
	}

	if err != nil {
		e.logger.Warn("Plugin poll execution failed",
			"plugin", pluginID,
			"error", err,
			"stderr", stderr.String(),
		)
		return nil, fmt.Errorf("plugin execution failed: %w", err)
	}

	// Parse response
	var results []PollResult
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		return nil, fmt.Errorf("failed to parse poll response: %w", err)
	}

	return results, nil
}
