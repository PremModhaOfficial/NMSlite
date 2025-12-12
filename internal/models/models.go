package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// CredentialProfile represents encrypted credentials
type CredentialProfile struct {
	ID             uuid.UUID     `db:"id"`
	Name           string        `db:"name"`
	Description    string        `db:"description"`
	Protocol       string        `db:"protocol"`        // e.g., 'winrm', 'ssh', 'snmp-v2c'
	CredentialData EncryptedData `db:"credential_data"` // Encrypted JSON
	CreatedAt      time.Time     `db:"created_at"`
	UpdatedAt      time.Time     `db:"updated_at"`
	DeletedAt      *time.Time    `db:"deleted_at"` // Soft delete
}

// DiscoveryProfile represents a discovery scanning job definition
type DiscoveryProfile struct {
	ID                   uuid.UUID  `db:"id"`
	Name                 string     `db:"name"`
	TargetValue          string     `db:"target_value"` // e.g., "192.168.1.0/24" - type auto-detected
	Ports                JSONArray  `db:"ports"`        // [22, 5985, 443]
	PortScanTimeoutMs    int        `db:"port_scan_timeout_ms"`
	CredentialProfileIDs JSONArray  `db:"credential_profile_ids"` // Array of UUIDs
	LastRunAt            *time.Time `db:"last_run_at"`
	LastRunStatus        *string    `db:"last_run_status"` // 'success', 'partial', 'failed'
	DevicesDiscovered    int        `db:"devices_discovered"`
	CreatedAt            time.Time  `db:"created_at"`
	UpdatedAt            time.Time  `db:"updated_at"`
	DeletedAt            *time.Time `db:"deleted_at"` // Soft delete
}

// Monitor represents a provisioned device
type Monitor struct {
	ID                     uuid.UUID  `db:"id"`
	DisplayName            string     `db:"display_name"`
	Hostname               string     `db:"hostname"`
	IPAddress              string     `db:"ip_address"` // INET type
	PluginID               string     `db:"plugin_id"`
	CredentialProfileID    uuid.UUID  `db:"credential_profile_id"`
	DiscoveryProfileID     uuid.UUID  `db:"discovery_profile_id"`
	PollingIntervalSeconds int        `db:"polling_interval_seconds"`
	Status                 string     `db:"status"` // 'active', 'maintenance', 'down'
	ConsecutiveFailures    int        `db:"consecutive_failures"`
	LastPollAt             *time.Time `db:"last_poll_at"`
	LastSuccessfulPollAt   *time.Time `db:"last_successful_poll_at"`
	CreatedAt              time.Time  `db:"created_at"`
	UpdatedAt              time.Time  `db:"updated_at"`
	DeletedAt              *time.Time `db:"deleted_at"` // Soft delete
}

// Metric represents a single metric data point
type Metric struct {
	Timestamp   time.Time  `db:"timestamp"`
	MetricGroup string     `db:"metric_group"` // e.g., 'host.cpu', 'host.memory'
	DeviceID    uuid.UUID  `db:"device_id"`
	Tags        JSONObject `db:"tags"` // e.g., {"core": "0"}
	ValUsed     *float64   `db:"val_used"`
	ValTotal    *float64   `db:"val_total"`
}

// EncryptedData is a wrapper for encrypted JSONB data
type EncryptedData []byte

func (e EncryptedData) Value() (driver.Value, error) {
	return []byte(e), nil
}

// JSONArray wraps a JSON array
type JSONArray []interface{}

func (ja JSONArray) Value() (driver.Value, error) {
	return json.Marshal(ja)
}

// JSONObject wraps a JSON object
type JSONObject map[string]interface{}

func (jo JSONObject) Value() (driver.Value, error) {
	return json.Marshal(jo)
}
