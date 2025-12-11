package channels

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// MonitorTask represents a single monitor polling task
type MonitorTask struct {
	MonitorID           uuid.UUID
	IP                  string
	Port                int
	PluginID            string
	CredentialProfileID uuid.UUID
	PollingInterval     time.Duration
}

// PollResult represents the outcome of a plugin poll
type PollResult struct {
	MonitorID uuid.UUID
	Success   bool
	Timestamp time.Time
	Metrics   interface{} // Flexible for different metric types
	Error     string
	PluginID  string
}

// StateTransition represents a monitor state change
type StateTransition struct {
	MonitorID uuid.UUID
	OldState  string
	NewState  string // "active", "down", "maintenance"
	Timestamp time.Time
	Failures  int
}

// PollingPipeline manages work queues for the polling subsystem
type PollingPipeline struct {
	// Input: batched liveness checks
	LivenessQueue chan []MonitorTask

	// Input: individual plugin execution tasks
	PluginQueue chan MonitorTask

	// Output: poll results for metrics storage
	ResultQueue chan PollResult

	// Output: state changes for StateHandler
	StateQueue chan StateTransition

	// Context for graceful shutdown
	ctx  context.Context
	done chan struct{}
}

// NewPollingPipeline creates a new polling pipeline with configured buffer sizes
func NewPollingPipeline(ctx context.Context, cfg PollingPipelineConfig) *PollingPipeline {
	return &PollingPipeline{
		LivenessQueue: make(chan []MonitorTask, cfg.LivenessQueueSize),
		PluginQueue:   make(chan MonitorTask, cfg.PluginQueueSize),
		ResultQueue:   make(chan PollResult, cfg.ResultQueueSize),
		StateQueue:    make(chan StateTransition, cfg.StateQueueSize),
		ctx:           ctx,
		done:          make(chan struct{}),
	}
}

// Close gracefully shuts down all pipeline channels
func (pp *PollingPipeline) Close() error {
	close(pp.done)

	close(pp.LivenessQueue)
	close(pp.PluginQueue)
	close(pp.ResultQueue)
	close(pp.StateQueue)

	return nil
}

// Done returns a channel that's closed when the pipeline is shutting down
func (pp *PollingPipeline) Done() <-chan struct{} {
	return pp.done
}

// Context returns the context associated with this pipeline
func (pp *PollingPipeline) Context() context.Context {
	return pp.ctx
}
