package plugins

// Credentials for authentication
type Credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Domain   string `json:"domain,omitempty"`

	// SSH specific
	PrivateKey string `json:"private_key,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`

	// SNMP v2c
	Community string `json:"community,omitempty"`

	// SNMP v3 (USM)
	SecurityName  string `json:"security_name,omitempty"`
	SecurityLevel string `json:"security_level,omitempty"`
	AuthProtocol  string `json:"auth_protocol,omitempty"`
	AuthPassword  string `json:"auth_password,omitempty"`
	PrivProtocol  string `json:"priv_protocol,omitempty"`
	PrivPassword  string `json:"priv_password,omitempty"`
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
