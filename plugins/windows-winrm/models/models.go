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
	UseHTTPS bool   `json:"use_https"`
}

// PluginOutput represents the result sent back to the core via STDOUT
type PluginOutput struct {
	RequestID string   `json:"request_id"`
	Status    string   `json:"status"` // "success" or "failed"
	Timestamp string   `json:"timestamp,omitempty"`
	Metrics   []Metric `json:"metrics,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// Metric represents a single metric data point
type Metric struct {
	MetricGroup string            `json:"metric_group"`
	Tags        map[string]string `json:"tags"`
	ValUsed     float64           `json:"val_used"`
	ValTotal    *float64          `json:"val_total"` // Pointer allows JSON null for metrics without limits
}

// Helper function to create a pointer to a float64 value
func Float64Ptr(v float64) *float64 {
	return &v
}

// Float64PtrOrNil returns nil if value is 0, otherwise returns pointer to value
func Float64PtrOrNil(v float64) *float64 {
	if v == 0 {
		return nil
	}
	return &v
}
