// Package eventbus provides a thread-safe, non-blocking event bus implementation
// for the NMS Lite monitoring system. It supports pub-sub patterns with multiple
// topics and graceful shutdown.
package eventbus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Topic represents the name of an event topic
type Topic string

// Event Topic Constants
const (
	// Discovery-related topics
	TopicDiscoveryRun       Topic = "discovery.run"
	TopicDiscoveryCompleted Topic = "discovery.completed"

	// Monitor health topics
	TopicMonitorDown      Topic = "monitor.down"
	TopicMonitorRecovered Topic = "monitor.recovered"

	// Plugin execution topics
	TopicPluginTimeout Topic = "plugin.timeout"
	TopicPluginError   Topic = "plugin.error"

	// Cache management topic
	TopicCacheInvalidate Topic = "cache.invalidate"
)

// Event represents a generic event in the system
type Event struct {
	Topic     Topic
	Timestamp time.Time
	Payload   interface{}
}

// DiscoveryRunEvent is published when a discovery run starts
type DiscoveryRunEvent struct {
	JobID     uuid.UUID
	ProfileID uuid.UUID
	StartedAt time.Time
}

// DiscoveryCompletedEvent is published when a discovery run completes
type DiscoveryCompletedEvent struct {
	JobID        uuid.UUID
	ProfileID    uuid.UUID
	DevicesFound int
	Status       string // 'success', 'partial', 'failed'
}

// MonitorDownEvent is published when a monitor transitions to down state
type MonitorDownEvent struct {
	MonitorID uuid.UUID
	IP        string
	Failures  int
	Timestamp time.Time
}

// MonitorRecoveredEvent is published when a monitor recovers from down state
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
}

// PluginErrorEvent is published when a plugin execution fails
type PluginErrorEvent struct {
	PluginID  string
	MonitorID uuid.UUID
	Error     string
}

// CacheInvalidateEvent is published when cache entries should be invalidated
type CacheInvalidateEvent struct {
	EntityType string // e.g., 'monitor', 'discovery_profile', 'credential'
	EntityID   uuid.UUID
}

// EventBus provides a thread-safe, non-blocking publish-subscribe event bus.
// Subscribers receive events through channels with bounded buffers. Events are
// dropped if a subscriber's buffer is full (non-blocking behavior).
type EventBus struct {
	// mu protects subscribers map
	mu sync.RWMutex

	// subscribers maps topics to their subscriber channels
	subscribers map[Topic][]chan Event

	// bufferSize is the buffer size for each subscriber channel
	bufferSize int

	// done signals graceful shutdown
	done chan struct{}

	// wg tracks all goroutines for graceful shutdown
	wg sync.WaitGroup
}

// NewEventBus creates a new EventBus with the specified buffer size.
// The buffer size determines how many events can be queued per subscriber
// before old events are dropped (non-blocking publish).
//
// Parameters:
//   - bufferSize: the capacity of each subscriber's event channel (recommended: 10-100)
//
// Returns:
//   - *EventBus: a new event bus instance ready for use
//
// Example:
//
//	bus := NewEventBus(50)
//	ch := bus.Subscribe(TopicMonitorDown)
//	go func() {
//		for event := range ch {
//			// process event
//		}
//	}()
func NewEventBus(bufferSize int) *EventBus {
	if bufferSize < 1 {
		bufferSize = 10 // Default to 10 if invalid size provided
	}
	return &EventBus{
		subscribers: make(map[Topic][]chan Event),
		bufferSize:  bufferSize,
		done:        make(chan struct{}),
	}
}

// Subscribe registers a new subscriber for the given topic and returns a channel
// for receiving events. Each call to Subscribe creates a new independent channel
// with its own buffer. If the channel buffer fills up, new events will be silently
// dropped (non-blocking behavior).
//
// The returned channel will be closed when the EventBus is shut down via Close().
//
// Parameters:
//   - topic: the topic to subscribe to
//
// Returns:
//   - <-chan Event: a receive-only channel for events on the specified topic
//
// Example:
//
//	ch := bus.Subscribe(TopicDiscoveryCompleted)
//	for event := range ch {
//		if discoveryEvent, ok := event.Payload.(DiscoveryCompletedEvent); ok {
//			fmt.Printf("Discovery completed: %d devices found\n", discoveryEvent.DevicesFound)
//		}
//	}
func (eb *EventBus) Subscribe(topic Topic) <-chan Event {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	ch := make(chan Event, eb.bufferSize)
	eb.subscribers[topic] = append(eb.subscribers[topic], ch)

	return ch
}

// SubscribeMultiple registers a new subscriber for multiple topics and returns a single
// channel that receives events from any of the specified topics. This is useful when
// a component needs to monitor multiple event types without managing multiple channels.
//
// Like Subscribe(), if the buffer fills up, events are silently dropped.
// The returned channel will be closed when the EventBus is shut down.
//
// Parameters:
//   - topics: one or more topics to subscribe to
//
// Returns:
//   - <-chan Event: a receive-only channel for events on any of the specified topics
//
// Example:
//
//	ch := bus.SubscribeMultiple(TopicMonitorDown, TopicMonitorRecovered)
//	for event := range ch {
//		switch event.Topic {
//		case TopicMonitorDown:
//			// handle down event
//		case TopicMonitorRecovered:
//			// handle recovery event
//		}
//	}
func (eb *EventBus) SubscribeMultiple(topics ...Topic) <-chan Event {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	// Create a dedicated channel for multiplexing
	muxCh := make(chan Event, eb.bufferSize)

	// For each topic, create a subscriber and relay events to the mux channel
	for _, topic := range topics {
		ch := make(chan Event, eb.bufferSize)
		eb.subscribers[topic] = append(eb.subscribers[topic], ch)

		// Spawn a relay goroutine for this channel
		eb.wg.Add(1)
		go func(relayChain <-chan Event) {
			defer eb.wg.Done()
			for {
				select {
				case event, ok := <-relayChain:
					if !ok {
						return // relayChain closed
					}
					// Try to send to mux channel, drop if full
					select {
					case muxCh <- event:
					default:
						// Buffer full, drop event
					}
				case <-eb.done:
					return
				}
			}
		}(ch)
	}

	return muxCh
}

// Publish publishes an event to all subscribers of the event's topic.
// This method is non-blocking: if a subscriber's channel buffer is full,
// the event is silently dropped for that subscriber only.
//
// The context parameter allows for cancellation during the publish operation,
// though the method returns immediately regardless.
//
// Parameters:
//   - ctx: context for cancellation (currently unused but reserved for future use)
//   - topic: the topic to publish to
//   - payload: the event payload (will be type-asserted by subscribers)
//
// Returns:
//   - error: currently always returns nil, reserved for future error handling
//
// Example:
//
//	event := MonitorDownEvent{
//		MonitorID: monitorID,
//		IP: "192.168.1.100",
//		Failures: 3,
//		Timestamp: time.Now(),
//	}
//	bus.Publish(ctx, TopicMonitorDown, event)
func (eb *EventBus) Publish(ctx context.Context, topic Topic, payload interface{}) error {
	eb.mu.RLock()
	subscribers, exists := eb.subscribers[topic]
	// Make a copy to avoid holding the lock during send
	subscribersCopy := make([]chan Event, len(subscribers))
	copy(subscribersCopy, subscribers)
	eb.mu.RUnlock()

	if !exists || len(subscribersCopy) == 0 {
		return nil // No subscribers, nothing to do
	}

	event := Event{
		Topic:     topic,
		Timestamp: time.Now(),
		Payload:   payload,
	}

	// Send to all subscribers, non-blocking
	for _, ch := range subscribersCopy {
		select {
		case ch <- event:
			// Event sent successfully
		default:
			// Channel buffer full, silently drop the event for this subscriber
			// This prevents slow subscribers from blocking the publisher
		}
	}

	return nil
}

// Close gracefully shuts down the EventBus. All subscriber channels are closed,
// and all internal goroutines are allowed to finish. Once Close() is called,
// the EventBus should not be used anymore.
//
// This method blocks until all goroutines have finished. In typical usage,
// this should be called during application shutdown.
//
// Returns:
//   - error: currently always returns nil, reserved for future error handling
//
// Example:
//
//	defer func() {
//		if err := bus.Close(); err != nil {
//			log.Printf("failed to close event bus: %v", err)
//		}
//	}()
func (eb *EventBus) Close() error {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	// Signal all goroutines to stop
	close(eb.done)

	// Close all subscriber channels
	for _, subscribers := range eb.subscribers {
		for _, ch := range subscribers {
			// Prevent panic on already-closed channels
			// This works because we only close once and all sends are done
			select {
			case _, ok := <-ch:
				if ok {
					close(ch)
				}
			default:
				// Channel might be empty, still safe to close
				close(ch)
			}
		}
	}

	// Clear the map
	eb.subscribers = make(map[Topic][]chan Event)

	// Wait for all relay goroutines from SubscribeMultiple
	eb.wg.Wait()

	return nil
}

// TopicString returns the string representation of a Topic.
// This is useful for logging and debugging.
//
// Returns:
//   - string: the topic as a string
//
// Example:
//
//	fmt.Printf("Publishing to topic: %s\n", TopicMonitorDown.TopicString())
func (t Topic) String() string {
	return string(t)
}

// EventString returns a human-readable string representation of an Event.
// Useful for logging and debugging.
//
// Returns:
//   - string: formatted event information
func (e Event) String() string {
	return fmt.Sprintf("Event{Topic: %s, Timestamp: %s, Payload: %+v}",
		e.Topic.String(),
		e.Timestamp.Format(time.RFC3339Nano),
		e.Payload,
	)
}

// DiscoveryRunEventString returns a formatted string for a DiscoveryRunEvent.
func (d DiscoveryRunEvent) String() string {
	return fmt.Sprintf("DiscoveryRunEvent{JobID: %s, ProfileID: %s, StartedAt: %s}",
		d.JobID.String(),
		d.ProfileID.String(),
		d.StartedAt.Format(time.RFC3339),
	)
}

// DiscoveryCompletedEventString returns a formatted string for a DiscoveryCompletedEvent.
func (d DiscoveryCompletedEvent) String() string {
	return fmt.Sprintf("DiscoveryCompletedEvent{JobID: %s, ProfileID: %s, DevicesFound: %d, Status: %s}",
		d.JobID.String(),
		d.ProfileID.String(),
		d.DevicesFound,
		d.Status,
	)
}

// MonitorDownEventString returns a formatted string for a MonitorDownEvent.
func (m MonitorDownEvent) String() string {
	return fmt.Sprintf("MonitorDownEvent{MonitorID: %s, IP: %s, Failures: %d, Timestamp: %s}",
		m.MonitorID.String(),
		m.IP,
		m.Failures,
		m.Timestamp.Format(time.RFC3339),
	)
}

// MonitorRecoveredEventString returns a formatted string for a MonitorRecoveredEvent.
func (m MonitorRecoveredEvent) String() string {
	return fmt.Sprintf("MonitorRecoveredEvent{MonitorID: %s, IP: %s, Timestamp: %s}",
		m.MonitorID.String(),
		m.IP,
		m.Timestamp.Format(time.RFC3339),
	)
}

// PluginTimeoutEventString returns a formatted string for a PluginTimeoutEvent.
func (p PluginTimeoutEvent) String() string {
	return fmt.Sprintf("PluginTimeoutEvent{PluginID: %s, MonitorID: %s, Timeout: %v}",
		p.PluginID,
		p.MonitorID.String(),
		p.Timeout,
	)
}

// PluginErrorEventString returns a formatted string for a PluginErrorEvent.
func (p PluginErrorEvent) String() string {
	return fmt.Sprintf("PluginErrorEvent{PluginID: %s, MonitorID: %s, Error: %s}",
		p.PluginID,
		p.MonitorID.String(),
		p.Error,
	)
}

// CacheInvalidateEventString returns a formatted string for a CacheInvalidateEvent.
func (c CacheInvalidateEvent) String() string {
	return fmt.Sprintf("CacheInvalidateEvent{EntityType: %s, EntityID: %s}",
		c.EntityType,
		c.EntityID.String(),
	)
}

// DiscoveryJob represents the status and metadata of a discovery job in progress
type DiscoveryJob struct {
	JobID        uuid.UUID
	ProfileID    uuid.UUID
	Status       string // 'pending', 'running', 'completed', 'failed'
	Progress     int    // percentage 0-100
	StartedAt    time.Time
	CompletedAt  *time.Time
	DevicesFound int
	Error        string
}

// JobStore holds all in-flight discovery jobs
type JobStore struct {
	mu   sync.RWMutex
	jobs map[uuid.UUID]*DiscoveryJob
}

// NewJobStore creates a new job store
func NewJobStore() *JobStore {
	return &JobStore{
		jobs: make(map[uuid.UUID]*DiscoveryJob),
	}
}

// GetJob retrieves a job by its ID
func (js *JobStore) GetJob(jobID uuid.UUID) (*DiscoveryJob, bool) {
	js.mu.RLock()
	defer js.mu.RUnlock()
	job, exists := js.jobs[jobID]
	return job, exists
}

// SetJob stores or updates a job
func (js *JobStore) SetJob(job *DiscoveryJob) {
	js.mu.Lock()
	defer js.mu.Unlock()
	js.jobs[job.JobID] = job
}

// DeleteJob removes a completed job
func (js *JobStore) DeleteJob(jobID uuid.UUID) {
	js.mu.Lock()
	defer js.mu.Unlock()
	delete(js.jobs, jobID)
}

// Global job store - shared across handlers and workers
var globalJobStore = NewJobStore()

// GetGlobalJobStore returns the global job store instance
func GetGlobalJobStore() *JobStore {
	return globalJobStore
}
