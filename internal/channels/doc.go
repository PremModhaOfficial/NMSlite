// Package channels implements a typed channel-based event-driven architecture
// for the NMSlite monitoring system.
//
// This package replaces the generic eventbus pattern with compile-time type-safe
// Go channels, providing:
//
//   - Type safety: No runtime type assertions needed
//   - Clear contracts: Each event type has its own channel
//   - Better debugging: Channel traces show exact event types
//   - Zero dependencies: Pure Go channels, no external libraries
//
// # Architecture
//
// The package provides two primary abstractions:
//
// 1. EventChannels: For broadcasting system events (discovery, monitor state changes, etc.)
// 2. PollingPipeline: For work queues in the polling subsystem
//
// # Migration from EventBus
//
// Old pattern (eventbus):
//
//	eb.Publish(ctx, eventbus.TopicDiscovery, eventbus.DiscoveryEvent{...})
//	eventChan := eb.Subscribe(eventbus.TopicDiscovery)
//	for event := range eventChan {
//	    if evt, ok := event.Payload.(eventbus.DiscoveryEvent); ok {
//	        // handle event
//	    }
//	}
//
// New pattern (channels):
//
//	select {
//	case events.DiscoveryStarted <- DiscoveryStartedEvent{...}:
//	case <-ctx.Done():
//	}
//
//	for event := range events.DiscoveryStarted {
//	    // handle event - already typed!
//	}
//
// # Graceful Shutdown
//
// All channel hubs support context-based cancellation and graceful shutdown:
//
//	events := NewEventChannels(ctx, config)
//	defer events.Close()
//
//	select {
//	case <-events.Done():
//	    // shutdown initiated
//	}
package channels
