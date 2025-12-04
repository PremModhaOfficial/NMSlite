package model

import "time"

// User represents a system user
type User struct {
	ID        int       `json:"id"`
	Username  string    `json:"username"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Claims represents JWT claims
type Claims struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

// TokenPair represents access and refresh tokens
type TokenPair struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Credential represents authentication credentials for Windows devices
type Credential struct {
	ID             int       `json:"id"`
	Name           string    `json:"name"`
	CredentialType string    `json:"credential_type"` // wmi, winrm_basic, winrm_ntlm
	Username       string    `json:"username"`
	Domain         string    `json:"domain,omitempty"`
	Port           int       `json:"port,omitempty"`
	UseSSL         bool      `json:"use_ssl"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	// Password is never returned in API responses
}

// Device represents a Windows machine being monitored
type Device struct {
	ID              int       `json:"id"`
	IP              string    `json:"ip"`
	Hostname        string    `json:"hostname"`
	OS              string    `json:"os"`
	Status          string    `json:"status"` // discovered, provisioned, monitoring, unreachable
	PollingInterval int       `json:"polling_interval"`
	LastSeen        time.Time `json:"last_seen"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// DeviceMetrics represents collected metrics from a device
type DeviceMetrics struct {
	DeviceID     int            `json:"device_id"`
	Timestamp    time.Time      `json:"timestamp"`
	CPU          CPUMetrics     `json:"cpu"`
	Memory       MemoryMetrics  `json:"memory"`
	Disk         DiskMetrics    `json:"disk"`
	Network      NetworkMetrics `json:"network"`
	ProcessCount int            `json:"process_count"`
}

// CPUMetrics represents CPU usage
type CPUMetrics struct {
	UsagePercent float64 `json:"usage_percent"`
}

// MemoryMetrics represents memory usage
type MemoryMetrics struct {
	TotalBytes   int64   `json:"total_bytes"`
	UsedBytes    int64   `json:"used_bytes"`
	UsagePercent float64 `json:"usage_percent"`
}

// DiskMetrics represents disk usage
type DiskMetrics struct {
	TotalBytes   int64   `json:"total_bytes"`
	UsedBytes    int64   `json:"used_bytes"`
	UsagePercent float64 `json:"usage_percent"`
}

// NetworkMetrics represents network usage
type NetworkMetrics struct {
	BytesSentPerSec    int64   `json:"bytes_sent_per_sec"`
	BytesRecvPerSec    int64   `json:"bytes_recv_per_sec"`
	UtilizationPercent float64 `json:"utilization_percent"`
	PacketsSent        int64   `json:"packets_sent"`
	PacketsRecv        int64   `json:"packets_recv"`
	Errors             int64   `json:"errors"`
	Dropped            int64   `json:"dropped"`
}
