package poller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/nmslite/nmslite/internal/globals"
)

// PluginManager manages plugin loading and execution
type PluginManager struct {
	pluginDir string
	plugins   map[string]*globals.PluginInfo // keyed by Protocol (e.g. "ssh", "winrm")
	mu        sync.RWMutex
	logger    *slog.Logger
	timeout   time.Duration
}

// NewPluginManager creates a new plugin manager
func NewPluginManager(pluginDir string, timeout time.Duration) *PluginManager {
	return &PluginManager{
		pluginDir: pluginDir,
		plugins:   make(map[string]*globals.PluginInfo),
		logger:    slog.Default().With("component", "plugin_manager"),
		timeout:   timeout,
	}
}

// Scan scans the plugin directory and loads all plugins, indexed by Protocol
func (m *PluginManager) Scan() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Clear existing plugins
	m.plugins = make(map[string]*globals.PluginInfo)

	// Read plugin directory
	entries, err := os.ReadDir(m.pluginDir)
	if err != nil {
		if os.IsNotExist(err) {
			m.logger.Info("Plugin directory does not exist, skipping scan", "dir", m.pluginDir)
			return nil
		}
		return fmt.Errorf("failed to read plugin directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginName := entry.Name()
		pluginPath := filepath.Join(m.pluginDir, pluginName)

		// Load manifest
		manifestPath := filepath.Join(pluginPath, "manifest.json")
		manifestData, err := os.ReadFile(manifestPath)
		if err != nil {
			m.logger.Warn("Failed to read manifest", "plugin", pluginName, "error", err)
			continue
		}

		var pluginMeta struct {
			Name     string `json:"name"`
			Protocol string `json:"protocol"`
		}
		if err := json.Unmarshal(manifestData, &pluginMeta); err != nil {
			m.logger.Warn("Failed to parse manifest", "plugin", pluginName, "error", err)
			continue
		}

		// Find binary (assume it has the same name as the directory)
		binaryPath := filepath.Join(pluginPath, pluginName)
		if _, err := os.Stat(binaryPath); err != nil {
			m.logger.Warn("Binary not found", "plugin", pluginName, "path", binaryPath)
			continue
		}

		// Register plugin
		info := &globals.PluginInfo{
			Name:       pluginMeta.Name,
			Protocol:   pluginMeta.Protocol,
			BinaryPath: binaryPath,
		}

		// Enforce 1:1 Protocol mapping (last one wins if duplicate, or error? User said 1:1)
		// We'll log a warning if overwriting.
		if existing, exists := m.plugins[pluginMeta.Protocol]; exists {
			m.logger.Warn("Duplicate plugin for protocol found, overwriting",
				"protocol", pluginMeta.Protocol,
				"old_plugin", existing.Name,
				"new_plugin", pluginMeta.Name,
			)
		}

		m.plugins[pluginMeta.Protocol] = info

		m.logger.Info("Loaded plugin",
			"protocol", pluginMeta.Protocol,
			"name", pluginMeta.Name,
		)
	}

	return nil
}

// Get retrieves a plugin by its Protocol
func (m *PluginManager) Get(protocol string) (*globals.PluginInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	plugin, ok := m.plugins[protocol]
	return plugin, ok
}

// List returns all registered plugins
func (m *PluginManager) List() []*globals.PluginInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*globals.PluginInfo, 0, len(m.plugins))
	for _, plugin := range m.plugins {
		result = append(result, plugin)
	}
	return result
}

// Poll executes a batch of tasks using the plugin associated with the given protocol
func (m *PluginManager) Poll(ctx context.Context, protocol string, tasks []globals.PollTask) ([]globals.PollResult, error) {
	plugin, ok := m.Get(protocol)
	if !ok {
		return nil, fmt.Errorf("no plugin found for protocol: %s", protocol)
	}

	// Marshal tasks to JSON
	inputData, err := json.Marshal(tasks)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tasks: %w", err)
	}

	// Prepare command
	cmd := exec.CommandContext(ctx, plugin.BinaryPath)
	cmd.Dir = filepath.Dir(plugin.BinaryPath) // Run in plugin directory

	// Pipe input
	cmd.Stdin = bytes.NewReader(inputData)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	m.logger.Debug("Executing plugin", "protocol", protocol, "task_count", len(tasks))

	// Execute
	start := time.Now()
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("plugin execution failed: %w, stderr: %s", err, stderr.String())
	}
	duration := time.Since(start)

	m.logger.Debug("Plugin execution completed",
		"protocol", protocol,
		"duration", duration,
		"stderr_len", stderr.Len(),
	)

	// Unmarshal output
	var results []globals.PollResult
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		return nil, fmt.Errorf("failed to parse plugin output: %w, output: %s", err, stdout.String())
	}

	return results, nil
}
