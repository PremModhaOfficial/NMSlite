package plugins

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// Registry scans plugin_bins/ and maintains in-memory plugin index
type Registry struct {
	pluginDir string
	plugins   map[string]*PluginInfo // keyed by plugin ID
	byPort    map[int][]*PluginInfo  // index by default_port
	mu        sync.RWMutex
	logger    *slog.Logger
}

// NewRegistry creates a new plugin registry
func NewRegistry(pluginDir string, logger *slog.Logger) *Registry {
	return &Registry{
		pluginDir: pluginDir,
		plugins:   make(map[string]*PluginInfo),
		byPort:    make(map[int][]*PluginInfo),
		logger:    logger,
	}
}

// Scan scans the plugin directory and loads all plugins
func (r *Registry) Scan() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Clear existing plugins
	r.plugins = make(map[string]*PluginInfo)
	r.byPort = make(map[int][]*PluginInfo)

	// Read plugin directory
	entries, err := os.ReadDir(r.pluginDir)
	if err != nil {
		return fmt.Errorf("failed to read plugin directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pluginName := entry.Name()
		pluginPath := filepath.Join(r.pluginDir, pluginName)

		// Load manifest
		manifestPath := filepath.Join(pluginPath, "manifest.json")
		manifestData, err := os.ReadFile(manifestPath)
		if err != nil {
			r.logger.Warn("Failed to read manifest", "plugin", pluginName, "error", err)
			continue
		}

		var manifest PluginManifest
		if err := json.Unmarshal(manifestData, &manifest); err != nil {
			r.logger.Warn("Failed to parse manifest", "plugin", pluginName, "error", err)
			continue
		}

		// Find binary (assume it has the same name as the directory)
		binaryPath := filepath.Join(pluginPath, pluginName)
		if _, err := os.Stat(binaryPath); err != nil {
			r.logger.Warn("Binary not found", "plugin", pluginName, "path", binaryPath)
			continue
		}

		// Register plugin
		info := &PluginInfo{
			Manifest:   manifest,
			BinaryPath: binaryPath,
			ConfigDir:  pluginPath,
		}

		r.plugins[manifest.ID] = info

		// Index by port
		if manifest.DefaultPort > 0 {
			r.byPort[manifest.DefaultPort] = append(r.byPort[manifest.DefaultPort], info)
		}

		r.logger.Info("Loaded plugin",
			"id", manifest.ID,
			"name", manifest.Name,
			"version", manifest.Version,
			"port", manifest.DefaultPort,
		)
	}

	return nil
}

// GetByID retrieves a plugin by its ID
func (r *Registry) GetByID(id string) (*PluginInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugin, ok := r.plugins[id]
	return plugin, ok
}

// GetByPort retrieves plugins that handle a specific port
func (r *Registry) GetByPort(port int) []*PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	plugins := r.byPort[port]
	// Return a copy to avoid race conditions
	result := make([]*PluginInfo, len(plugins))
	copy(result, plugins)
	return result
}

// GetByProtocol retrieves the plugin that handles a specific protocol
// Returns error if no plugin or multiple plugins are found for the protocol
func (r *Registry) GetByProtocol(protocol string) (*PluginInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var found []*PluginInfo
	for _, plugin := range r.plugins {
		if plugin.Manifest.Protocol == protocol {
			found = append(found, plugin)
		}
	}

	if len(found) == 0 {
		return nil, fmt.Errorf("no plugin found for protocol: %s", protocol)
	}

	if len(found) > 1 {
		return nil, fmt.Errorf("multiple plugins found for protocol %s (expected exactly one)", protocol)
	}

	return found[0], nil
}

// List returns all registered plugins
func (r *Registry) List() []*PluginInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*PluginInfo, 0, len(r.plugins))
	for _, plugin := range r.plugins {
		result = append(result, plugin)
	}
	return result
}
