package plugins

// Credentials for authentication
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Domain   string `json:"domain,omitempty"`
	UseHTTPS bool   `json:"use_https"`
}

// PollTask represents a single polling task
type PollTask struct {
	RequestID   string      `json:"request_id"`
	Target      string      `json:"target"`
	Port        int         `json:"port"`
	Credentials Credentials `json:"credentials"`
}

// PollResult represents polling result
type PollResult struct {
	RequestID string        `json:"request_id"`
	Status    string        `json:"status"`
	Timestamp string        `json:"timestamp,omitempty"`
	Metrics   []interface{} `json:"metrics,omitempty"`
	Error     string        `json:"error,omitempty"`
}
