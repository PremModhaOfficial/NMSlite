// Package channels provides typed Go channels for event-driven architecture,
// replacing the generic eventbus pattern with compile-time type safety.
package channels

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// DiscoveryStartedEvent is published when a discovery begins execution
type DiscoveryStartedEvent struct {
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

// MonitorDownEvent is published when a monitor fails health checks
type MonitorDownEvent struct {
	MonitorID uuid.UUID
	IP        string
	Failures  int
	Timestamp time.Time
}

// MonitorRecoveredEvent is published when a down monitor recovers
type MonitorRecoveredEvent struct {
	MonitorID uuid.UUID
	IP        string
	Timestamp time.Time
}

// PluginTimeoutEvent is published when a plugin execution times out
type PluginTimeoutEvent struct {
	PluginID  string
	MonitorID uuid.UUID
	Timeout   time.Duration
	Timestamp time.Time
}

// PluginErrorEvent is published when a plugin execution fails
type PluginErrorEvent struct {
	PluginID  string
	MonitorID uuid.UUID
	Error     string
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
	DiscoveryStarted   chan DiscoveryStartedEvent
	DiscoveryCompleted chan DiscoveryCompletedEvent

	// Monitor state events
	MonitorDown      chan MonitorDownEvent
	MonitorRecovered chan MonitorRecoveredEvent

	// Plugin events
	PluginTimeout chan PluginTimeoutEvent
	PluginError   chan PluginErrorEvent

	// Cache events
	CacheInvalidate chan CacheInvalidateEvent

	// Context for graceful shutdown
	ctx  context.Context
	done chan struct{}
}

// NewEventChannels creates a new EventChannels hub with configured buffer sizes
func NewEventChannels(ctx context.Context, cfg EventChannelsConfig) *EventChannels {
	return &EventChannels{
		DiscoveryStarted:   make(chan DiscoveryStartedEvent, cfg.DiscoveryBufferSize),
		DiscoveryCompleted: make(chan DiscoveryCompletedEvent, cfg.DiscoveryBufferSize),
		MonitorDown:        make(chan MonitorDownEvent, cfg.MonitorStateBufferSize),
		MonitorRecovered:   make(chan MonitorRecoveredEvent, cfg.MonitorStateBufferSize),
		PluginTimeout:      make(chan PluginTimeoutEvent, cfg.PluginBufferSize),
		PluginError:        make(chan PluginErrorEvent, cfg.PluginBufferSize),
		CacheInvalidate:    make(chan CacheInvalidateEvent, cfg.CacheBufferSize),
		ctx:                ctx,
		done:               make(chan struct{}),
	}
}

// Close gracefully shuts down all channels
func (ec *EventChannels) Close() error {
	close(ec.done)

	// Close all channels to signal consumers to exit
	close(ec.DiscoveryStarted)
	close(ec.DiscoveryCompleted)
	close(ec.MonitorDown)
	close(ec.MonitorRecovered)
	close(ec.PluginTimeout)
	close(ec.PluginError)
	close(ec.CacheInvalidate)

	return nil
}

// Done returns a channel that's closed when the EventChannels is shutting down
func (ec *EventChannels) Done() <-chan struct{} {
	return ec.done
}

// Context returns the context associated with this EventChannels
func (ec *EventChannels) Context() context.Context {
	return ec.ctx
}
