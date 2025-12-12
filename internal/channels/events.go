// Package channels provides typed Go channels for event-driven architecture,
// replacing the generic eventbus pattern with compile-time type safety.
package channels

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/nmslite/nmslite/internal/database/dbgen"
	"github.com/nmslite/nmslite/internal/plugins"
)

// DiscoveryRequestEvent is published when a discovery begins execution
type DiscoveryRequestEvent struct {
	ProfileID uuid.UUID
	StartedAt time.Time
}

// DiscoveryCompletedEvent is published when a discovery finishes
type DiscoveryCompletedEvent struct {
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
	Plugin            *plugins.PluginInfo
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

// EventChannels provides typed channels for all system events
type EventChannels struct {
	// Discovery events
	DiscoveryRequest   chan DiscoveryRequestEvent
	DiscoveryCompleted chan DiscoveryCompletedEvent
	DeviceValidated    chan DeviceValidatedEvent

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
func NewEventChannels(ctx context.Context, cfg EventChannelsConfig) *EventChannels {
	return &EventChannels{
		DiscoveryRequest:   make(chan DiscoveryRequestEvent, cfg.DiscoveryBufferSize),
		DiscoveryCompleted: make(chan DiscoveryCompletedEvent, cfg.DiscoveryBufferSize),
		DeviceValidated:    make(chan DeviceValidatedEvent, cfg.DiscoveryBufferSize),
		MonitorState:       make(chan MonitorStateEvent, cfg.MonitorStateBufferSize),
		PluginEvent:        make(chan PluginEvent, cfg.PluginBufferSize),
		CacheInvalidate:    make(chan CacheInvalidateEvent, cfg.CacheBufferSize),
		done:               make(chan struct{}),
	}
}

// Close gracefully shuts down all channels
func (ec *EventChannels) Close() error {
	close(ec.done)

	// Close all channels to signal consumers to exit
	close(ec.DiscoveryRequest)
	close(ec.DiscoveryCompleted)
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
