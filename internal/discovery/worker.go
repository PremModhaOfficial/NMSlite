// Package discovery provides discovery worker and event handling functionality.
package discovery

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nmslite/nmslite/internal/database/db_gen"
	"github.com/nmslite/nmslite/internal/eventbus"
	"github.com/sqlc-dev/pqtype"
)

// JobStatus represents the current status of a discovery job.
type JobStatus struct {
	// JobID is the unique identifier for this discovery job
	JobID string `json:"job_id"`
	// ProfileID is the UUID of the discovery profile being executed
	ProfileID uuid.UUID `json:"profile_id"`
	// Status is the current state: "running", "completed", "failed"
	Status string `json:"status"`
	// Progress percentage (0-100)
	Progress int `json:"progress"`
	// Result contains discovery results if completed
	Result *DiscoveryResult `json:"result,omitempty"`
	// Error message if job failed
	Error string `json:"error,omitempty"`
	// StartedAt is when the job started
	StartedAt time.Time `json:"started_at"`
	// CompletedAt is when the job completed (if finished)
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// DiscoveryResult contains the results of a completed discovery job.
type DiscoveryResult struct {
	// DevicesDiscovered is the count of devices found
	DevicesDiscovered int `json:"devices_discovered"`
	// LastRunStatus is the overall result: "success", "partial", "failed"
	LastRunStatus string `json:"last_run_status"`
	// DevicesFound is a list of discovered device details
	DevicesFound []DiscoveredDeviceInfo `json:"devices_found"`
}

// DiscoveredDeviceInfo contains information about a discovered device.
type DiscoveredDeviceInfo struct {
	IPAddress string `json:"ip_address"`
	Port      int32  `json:"port"`
	Status    string `json:"status"`
}

// DiscoveryWorker processes discovery events asynchronously and manages job lifecycle.
type DiscoveryWorker struct {
	eventBus *eventbus.EventBus
	querier  db_gen.Querier
	logger   *slog.Logger

	// jobsMu protects the jobs map
	jobsMu sync.RWMutex
	// jobs tracks in-progress and completed discovery jobs
	jobs map[string]*JobStatus
}

// NewDiscoveryWorker creates a new discovery worker instance.
//
// Parameters:
//   - eventBus: the event bus for publishing/subscribing to discovery events
//   - querier: database query interface for persistence
//   - logger: structured logger for logging
//
// Returns:
//   - *DiscoveryWorker: a new worker instance
func NewDiscoveryWorker(
	eventBus *eventbus.EventBus,
	querier db_gen.Querier,
	logger *slog.Logger,
) *DiscoveryWorker {
	return &DiscoveryWorker{
		eventBus: eventBus,
		querier:  querier,
		logger:   logger,
		jobs:     make(map[string]*JobStatus),
	}
}

// Run starts the discovery worker and begins processing discovery events.
//
// It subscribes to TopicDiscoveryRun events and processes them sequentially.
// The method runs until the context is cancelled.
//
// Parameters:
//   - ctx: context for cancellation and deadline
//
// Returns:
//   - error: context cancellation error if ctx is cancelled
func (w *DiscoveryWorker) Run(ctx context.Context) error {
	w.logger.InfoContext(ctx, "Discovery worker starting",
		slog.String("worker", "discovery"),
	)

	// Subscribe to discovery run events
	eventChan := w.eventBus.Subscribe(eventbus.TopicDiscoveryRun)

	for {
		select {
		case <-ctx.Done():
			w.logger.InfoContext(ctx, "Discovery worker shutting down",
				slog.String("reason", ctx.Err().Error()),
			)
			return ctx.Err()

		case event, ok := <-eventChan:
			if !ok {
				w.logger.WarnContext(ctx, "Event channel closed, exiting worker")
				return fmt.Errorf("discovery event channel closed")
			}

			// Handle the discovery run event
			if runEvent, ok := event.Payload.(eventbus.DiscoveryRunEvent); ok {
				w.handleDiscoveryRunEvent(ctx, runEvent)
			} else {
				w.logger.WarnContext(ctx, "Received invalid event type",
					slog.String("expected", "eventbus.DiscoveryRunEvent"),
					slog.String("received", fmt.Sprintf("%T", event.Payload)),
				)
			}
		}
	}
}

// handleDiscoveryRunEvent processes a single discovery run event.
func (w *DiscoveryWorker) handleDiscoveryRunEvent(ctx context.Context, event eventbus.DiscoveryRunEvent) {
	jobID := event.JobID.String()
	logger := w.logger.With(
		slog.String("job_id", jobID),
		slog.String("profile_id", event.ProfileID.String()),
		slog.String("started_at", event.StartedAt.Format(time.RFC3339)),
	)

	// Check if job is already running
	if w.isJobRunning(event.ProfileID) {
		logger.WarnContext(ctx, "Discovery already running for this profile, skipping duplicate")
		w.publishCompletedEvent(ctx, event, "failed", 0, "duplicate discovery run detected")
		return
	}

	// Validate discovery profile exists
	profile, err := w.querier.GetDiscoveryProfile(ctx, event.ProfileID)
	if err != nil {
		if err == sql.ErrNoRows {
			logger.ErrorContext(ctx, "Discovery profile not found",
				slog.String("error", err.Error()),
			)
			w.publishCompletedEvent(ctx, event, "failed", 0, "discovery profile not found")
		} else {
			logger.ErrorContext(ctx, "Failed to fetch discovery profile",
				slog.String("error", err.Error()),
			)
			w.publishCompletedEvent(ctx, event, "failed", 0, fmt.Sprintf("database error: %v", err))
		}
		return
	}

	// Create and register job
	jobStatus := &JobStatus{
		JobID:     jobID,
		ProfileID: event.ProfileID,
		Status:    "running",
		Progress:  0,
		StartedAt: time.Now(),
	}
	w.registerJob(jobStatus)
	defer w.unregisterJob(jobID)

	logger.InfoContext(ctx, "Starting discovery run",
		slog.String("profile_name", profile.Name),
		slog.String("target_type", profile.TargetType),
	)

	// Execute discovery
	result, jobErr := w.executeDiscovery(ctx, profile, jobStatus, logger)

	// Determine final status
	status := "success"
	if jobErr != nil {
		status = "failed"
		logger.ErrorContext(ctx, "Discovery execution failed",
			slog.String("error", jobErr.Error()),
		)
	}

	// Update job status
	jobStatus.Status = status
	jobStatus.Progress = 100
	jobStatus.Result = result
	if jobErr != nil {
		jobStatus.Error = jobErr.Error()
	}
	now := time.Now()
	jobStatus.CompletedAt = &now

	// Persist job status update
	w.updateJobStatus(jobStatus)

	// Publish completion event
	deviceCount := 0
	if result != nil {
		deviceCount = result.DevicesDiscovered
	}
	w.publishCompletedEvent(ctx, event, status, deviceCount, "")

	logger.InfoContext(ctx, "Discovery run completed",
		slog.String("status", status),
		slog.Int("devices_discovered", deviceCount),
	)
}

// executeDiscovery runs the actual discovery process for a profile.
//
// It performs the following steps:
// 1. Parses the ports from JSON configuration
// 2. Clears previous discovered devices for the profile
// 3. Scans each port on the target IP for connectivity
// 4. Creates database records for open ports
// 5. Updates the discovery profile with results
//
// Returns the discovery result or an error if the process fails.
func (w *DiscoveryWorker) executeDiscovery(
	ctx context.Context,
	profile db_gen.DiscoveryProfile,
	jobStatus *JobStatus,
	logger *slog.Logger,
) (*DiscoveryResult, error) {
	// Parse ports from JSON
	var ports []int
	if err := json.Unmarshal(profile.Ports, &ports); err != nil {
		return nil, fmt.Errorf("failed to parse ports: %w", err)
	}

	// Clear previous discovered devices for this profile
	err := w.querier.ClearDiscoveredDevices(ctx, uuid.NullUUID{
		UUID:  profile.ID,
		Valid: true,
	})
	if err != nil {
		logger.WarnContext(ctx, "Failed to clear previous discovered devices",
			slog.String("error", err.Error()),
		)
	}

	result := &DiscoveryResult{
		DevicesDiscovered: 0,
		LastRunStatus:     "success",
		DevicesFound:      []DiscoveredDeviceInfo{},
	}

	// Get port scan timeout, default to 1 second if not set
	timeout := time.Duration(1000) * time.Millisecond
	if profile.PortScanTimeoutMs.Valid && profile.PortScanTimeoutMs.Int32 > 0 {
		timeout = time.Duration(profile.PortScanTimeoutMs.Int32) * time.Millisecond
	}

	// Scan target for open ports
	discoveredDevices := 0
	totalAttempts := len(ports)

	for i, port := range ports {
		// Check context cancellation
		if err := ctx.Err(); err != nil {
			return result, fmt.Errorf("discovery cancelled: %w", err)
		}

		// Update progress
		jobStatus.Progress = int(float64(i) / float64(totalAttempts) * 100)

		// Create a context with timeout for this port scan
		scanCtx, cancel := context.WithTimeout(ctx, timeout)

		// Attempt to connect to the port
		address := fmt.Sprintf("%s:%d", profile.TargetValue, port)
		conn, err := (&net.Dialer{}).DialContext(scanCtx, "tcp", address)
		cancel()

		if err != nil {
			logger.DebugContext(ctx, "Port scan failed",
				slog.String("address", address),
				slog.String("error", err.Error()),
			)
			continue
		}

		defer conn.Close()

		// Port is open - create discovered device record
		device, err := w.querier.CreateDiscoveredDevice(ctx, db_gen.CreateDiscoveredDeviceParams{
			DiscoveryProfileID: uuid.NullUUID{UUID: profile.ID, Valid: true},
			IpAddress: pqtype.Inet{
				IPNet: net.IPNet{
					IP:   net.ParseIP(profile.TargetValue),
					Mask: net.CIDRMask(32, 32),
				},
				Valid: true,
			},
			Hostname: sql.NullString{String: profile.TargetValue, Valid: true},
			Port:     int32(port),
			Status:   sql.NullString{String: "new", Valid: true},
		})

		if err != nil {
			logger.ErrorContext(ctx, "Failed to create discovered device record",
				slog.String("address", address),
				slog.String("error", err.Error()),
			)
			result.LastRunStatus = "partial"
			continue
		}

		discoveredDevices++
		deviceInfo := DiscoveredDeviceInfo{
			IPAddress: device.IpAddress.IPNet.IP.String(),
			Port:      device.Port,
			Status:    device.Status.String,
		}
		result.DevicesFound = append(result.DevicesFound, deviceInfo)

		logger.DebugContext(ctx, "Device discovered",
			slog.String("ip_address", deviceInfo.IPAddress),
			slog.Int("port", int(deviceInfo.Port)),
		)
	}

	result.DevicesDiscovered = discoveredDevices

	// Update discovery profile with results
	updateErr := w.updateDiscoveryProfileStatus(ctx, profile.ID, discoveredDevices, result.LastRunStatus)
	if updateErr != nil {
		logger.ErrorContext(ctx, "Failed to update discovery profile status",
			slog.String("error", updateErr.Error()),
		)
		// Don't fail the entire job, but log the issue
		result.LastRunStatus = "partial"
	}

	return result, nil
}

// updateDiscoveryProfileStatus updates the discovery profile with completion status.
//
// Note: The current database schema may not support direct status updates.
// This is a placeholder for future enhancement.
func (w *DiscoveryWorker) updateDiscoveryProfileStatus(
	ctx context.Context,
	profileID uuid.UUID,
	devicesDiscovered int,
	status string,
) error {
	// Log the update intent
	w.logger.DebugContext(ctx, "Recording discovery profile status",
		slog.String("profile_id", profileID.String()),
		slog.Int("devices_discovered", devicesDiscovered),
		slog.String("status", status),
	)
	return nil
}

// publishCompletedEvent publishes a discovery completion event to the event bus.
func (w *DiscoveryWorker) publishCompletedEvent(
	ctx context.Context,
	event eventbus.DiscoveryRunEvent,
	status string,
	deviceCount int,
	_ string, // error message (reserved for future use)
) {
	completedEvent := eventbus.DiscoveryCompletedEvent{
		JobID:        event.JobID,
		ProfileID:    event.ProfileID,
		DevicesFound: deviceCount,
		Status:       status,
	}

	err := w.eventBus.Publish(ctx, eventbus.TopicDiscoveryCompleted, completedEvent)
	if err != nil {
		w.logger.ErrorContext(ctx, "Failed to publish discovery completed event",
			slog.String("job_id", event.JobID.String()),
			slog.String("error", err.Error()),
		)
	} else {
		w.logger.DebugContext(ctx, "Published discovery completed event",
			slog.String("job_id", event.JobID.String()),
			slog.String("status", status),
		)
	}
}

// GetJobStatus returns the current status of a discovery job.
//
// Returns nil if the job is not found or has been cleaned up.
//
// Parameters:
//   - jobID: the job identifier to query
//
// Returns:
//   - *JobStatus: the job status or nil if not found
func (w *DiscoveryWorker) GetJobStatus(jobID string) *JobStatus {
	w.jobsMu.RLock()
	defer w.jobsMu.RUnlock()

	return w.jobs[jobID]
}

// isJobRunning checks if a discovery job is currently running for a profile.
func (w *DiscoveryWorker) isJobRunning(profileID uuid.UUID) bool {
	w.jobsMu.RLock()
	defer w.jobsMu.RUnlock()

	for _, job := range w.jobs {
		if job.ProfileID == profileID && job.Status == "running" {
			return true
		}
	}
	return false
}

// registerJob registers a new job in the jobs map.
func (w *DiscoveryWorker) registerJob(job *JobStatus) {
	w.jobsMu.Lock()
	defer w.jobsMu.Unlock()

	w.jobs[job.JobID] = job
}

// unregisterJob removes a job from the jobs map after completion.
func (w *DiscoveryWorker) unregisterJob(jobID string) {
	w.jobsMu.Lock()
	defer w.jobsMu.Unlock()

	delete(w.jobs, jobID)
}

// updateJobStatus updates an existing job's status in the map.
func (w *DiscoveryWorker) updateJobStatus(job *JobStatus) {
	w.jobsMu.Lock()
	defer w.jobsMu.Unlock()

	if existingJob, ok := w.jobs[job.JobID]; ok {
		*existingJob = *job
	}
}
