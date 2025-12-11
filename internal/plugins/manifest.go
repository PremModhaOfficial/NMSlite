package plugins

// PluginManifest represents manifest.json structure
type PluginManifest struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Version          string   `json:"version"`
	Description      string   `json:"description"`
	Protocol         string   `json:"protocol"`
	DefaultPort      int      `json:"default_port"`
	SupportedMetrics []string `json:"supported_metrics"`
	TimeoutMs        int      `json:"timeout_ms"`
}

// PluginInfo combines manifest with runtime info
type PluginInfo struct {
	Manifest   PluginManifest
	BinaryPath string
	ConfigDir  string
}
