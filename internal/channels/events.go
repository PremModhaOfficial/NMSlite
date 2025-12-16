// Package channels provides typed Go channels for event-driven architecture,
// replacing the generic eventbus pattern with compile-time type safety.
package channels

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/nmslite/nmslite/internal/database/dbgen"
	"github.com/nmslite/nmslite/internal/globals"
	"github.com/nmslite/nmslite/internal/pluginManager"
)

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
	Plugin            *pluginManager.PluginInfo
	IP                string
	Port              int
}

// MonitorStateEvent is published when a monitor state changes
type MonitorStateEvent struct {
	MonitorID uuid.UUID
	IP        string
	EventType string // "down", "recovered"
	Failures  int    // only used when EventType == "down"
	Timestamp time.Time
}

// PluginEvent is published when a plugin execution encounters issues
type PluginEvent struct {
	PluginID  string
	MonitorID uuid.UUID
	EventType string        // "timeout", "error"
	Error     string        // only used when EventType == "error"
	Timeout   time.Duration // only used when EventType == "timeout"
	Timestamp time.Time
}

// CacheInvalidateEvent signals cache entries need refresh
type CacheInvalidateEvent struct {
	EntityType string // "credential", "monitor", "discovery"
	EntityID   uuid.UUID
	Timestamp  time.Time
}

// EventChannelsConfig configures buffer sizes for event channels
type EventChannelsConfig struct {
	DiscoveryBufferSize    int
	MonitorStateBufferSize int
	PluginBufferSize       int
	CacheBufferSize        int
}

// EventChannels provides typed channels for all system events
type EventChannels struct {
	// Discovery events
	DiscoveryRequest chan DiscoveryRequestEvent
	DiscoveryStatus  chan DiscoveryStatusEvent
	DeviceValidated  chan DeviceValidatedEvent

	// Monitor state events
	MonitorState chan MonitorStateEvent

	// Plugin events
	PluginEvent chan PluginEvent

	// Cache events
	CacheInvalidate chan CacheInvalidateEvent

	// Graceful shutdown
	done chan struct{}
}

// NewEventChannels creates a new EventChannels hub with configured buffer sizes
func NewEventChannels(ctx context.Context) *EventChannels {
	cfg := globals.GetConfig().Channel

	discoverySize := cfg.DiscoveryEventsChannelSize
	if discoverySize <= 0 {
		discoverySize = 50
	}

	return &EventChannels{
		DiscoveryRequest: make(chan DiscoveryRequestEvent, discoverySize),
		DiscoveryStatus:  make(chan DiscoveryStatusEvent, discoverySize),
		DeviceValidated:  make(chan DeviceValidatedEvent, discoverySize),
		MonitorState:     make(chan MonitorStateEvent, cfg.StateSignalChannelSize),
		PluginEvent:      make(chan PluginEvent, 100), // Hardcoded default
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
	close(ec.PluginEvent)
	close(ec.CacheInvalidate)

	return nil
}

// Done returns a channel that's closed when the EventChannels is shutting down
func (ec *EventChannels) Done() <-chan struct{} {
	return ec.done
}
