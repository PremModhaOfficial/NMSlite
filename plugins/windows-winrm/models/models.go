package models

// PluginInput represents a single polling task received from the core via STDIN
type PluginInput struct {
	RequestID   string      `json:"request_id"`
	Target      string      `json:"target"`
	Port        int         `json:"port"`
	Credentials Credentials `json:"credentials"`
}

// Credentials holds authentication details for WinRM connection
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Domain   string `json:"domain,omitempty"`
}

// PluginOutput represents the result sent back to the core via STDOUT
type PluginOutput struct {
	RequestID string   `json:"request_id"`
	Status    string   `json:"status"` // "success" or "failed"
	Timestamp string   `json:"timestamp,omitempty"`
	Metrics   []Metric `json:"metrics,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// Metric represents a single metric data point in SNMP-style key-value format
type Metric struct {
	Name  string  `json:"name"` // Hierarchical: "system.cpu.usage"
	Value float64 `json:"value"`
	Type  string  `json:"type,omitempty"` // "gauge", "counter", "derive" - defaults to "gauge"
}
