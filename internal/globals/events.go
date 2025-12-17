// Package globals  provides typed Go channels for event-driven architecture,
package globals

import (
	"time"

	"github.com/google/uuid"
	"github.com/nmslite/nmslite/internal/api/auth"
	"github.com/nmslite/nmslite/internal/database/dbgen"
	// "github.com/nmslite/nmslite/internal/poller" - REMOVED
)

// PluginInfo represents a loaded plugin with its metadata and runtime path
type PluginInfo struct {
	Name       string `json:"name"`
	Protocol   string `json:"protocol"`
	BinaryPath string `json:"-"`
}

// PollTask represents a single polling task
type PollTask struct {
	RequestID   string           `json:"request_id"`
	Target      string           `json:"target"`
	Port        int              `json:"port"`
	Credentials auth.Credentials `json:"credentials"`
}

// PollResult represents polling result
type PollResult struct {
	RequestID string        `json:"request_id"`
	Status    string        `json:"status"`
	Timestamp string        `json:"timestamp,omitempty"`
	Metrics   []interface{} `json:"metrics,omitempty"`
	Error     string        `json:"error,omitempty"`
}

// DiscoveryRequestEvent is published when a discovery begins execution
type DiscoveryRequestEvent struct {
	ProfileID uuid.UUID
	StartedAt time.Time
}

// DiscoveryStatusEvent is published when a discovery finishes
type DiscoveryStatusEvent struct {
	ProfileID    uuid.UUID
	Status       string // "success", "partial", "failed"
	DevicesFound int
	StartedAt    time.Time
	CompletedAt  time.Time
}

// DeviceValidatedEvent - published when protocol handshake succeeds
type DeviceValidatedEvent struct {
	DiscoveryProfile  dbgen.DiscoveryProfile
	CredentialProfile dbgen.CredentialProfile
	Plugin            *PluginInfo
	IP                string
	Port              int
	Hostname          string
}

// MonitorStateEvent is published when a monitor state changes
type MonitorStateEvent struct {
	MonitorID uuid.UUID
	IP        string
	EventType string // "down", "recovered"
	Failures  int    // only used when EventType == "down"
	Timestamp time.Time
}

// CacheInvalidateEvent signals cache entries need refresh
// CacheInvalidateEvent signals cache entries need refresh
type CacheInvalidateEvent struct {
	UpdateType string                               // "update", "delete"
	Monitors   []dbgen.GetMonitorWithCredentialsRow // For "update"
	MonitorIDs []uuid.UUID                          // For "delete"
}

// EventChannels provides typed channels for all system events
type EventChannels struct {
	// Discovery events
	DiscoveryRequest chan DiscoveryRequestEvent
	DiscoveryStatus  chan DiscoveryStatusEvent
	DeviceValidated  chan DeviceValidatedEvent

	// Monitor state events
	MonitorState chan MonitorStateEvent

	// Cache events
	CacheInvalidate chan CacheInvalidateEvent

	// Graceful shutdown
	done chan struct{}
}

// NewEventChannels creates a new EventChannels hub with configured buffer sizes
func NewEventChannels() *EventChannels {
	cfg := GetConfig().Channel

	discoverySize := cfg.DiscoveryEventsChannelSize
	if discoverySize <= 0 {
		discoverySize = 50
	}

	return &EventChannels{
		DiscoveryRequest: make(chan DiscoveryRequestEvent, discoverySize),
		DiscoveryStatus:  make(chan DiscoveryStatusEvent, discoverySize),
		DeviceValidated:  make(chan DeviceValidatedEvent, discoverySize),
		MonitorState:     make(chan MonitorStateEvent, cfg.StateSignalChannelSize),
		CacheInvalidate:  make(chan CacheInvalidateEvent, cfg.CacheEventsChannelSize),
		done:             make(chan struct{}),
	}
}

// Close gracefully shuts down all channels
func (ec *EventChannels) Close() error {
	close(ec.done)

	// Close all channels to signal consumers to exit
	close(ec.DiscoveryRequest)
	close(ec.DiscoveryStatus)
	close(ec.DeviceValidated)
	close(ec.MonitorState)
	close(ec.CacheInvalidate)

	return nil
}

// Done returns a channel that's closed when the EventChannels is shutting down
func (ec *EventChannels) Done() <-chan struct{} {
	return ec.done
}
