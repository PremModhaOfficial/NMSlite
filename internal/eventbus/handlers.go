package eventbus

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/nmslite/nmslite/internal/database/db_gen"
	"github.com/sqlc-dev/pqtype"
)

// StartStateHandler starts a goroutine that listens for discovery completion events
// and updates the database with results. This handler processes events asynchronously.
func StartStateHandler(eb *EventBus, logger *slog.Logger) {
	ch := eb.Subscribe(TopicDiscoveryCompleted)

	for event := range ch {
		if completedEvent, ok := event.Payload.(DiscoveryCompletedEvent); ok {
			logger.Info("Processing discovery completion",
				"job_id", completedEvent.JobID.String(),
				"profile_id", completedEvent.ProfileID.String(),
				"devices_found", completedEvent.DevicesFound,
				"status", completedEvent.Status,
			)

			// Clean up completed job from store after a delay
			go func() {
				time.Sleep(5 * time.Minute)
				GetGlobalJobStore().DeleteJob(completedEvent.JobID)
				logger.Debug("Cleaned up discovery job", "job_id", completedEvent.JobID.String())
			}()
		}
	}
}

// StartDiscoveryWorker starts a goroutine that listens for discovery run events
// and executes the discovery scan asynchronously. This worker processes port scans
// and creates discovered device records in the database.
func StartDiscoveryWorker(eb *EventBus, q db_gen.Querier, logger *slog.Logger) {
	ch := eb.Subscribe(TopicDiscoveryRun)

	for event := range ch {
		if runEvent, ok := event.Payload.(DiscoveryRunEvent); ok {
			// Process discovery run asynchronously
			go processDiscoveryRun(eb, q, runEvent, logger)
		}
	}
}

// processDiscoveryRun executes a single discovery job asynchronously
func processDiscoveryRun(eb *EventBus, q db_gen.Querier, runEvent DiscoveryRunEvent, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	jobStore := GetGlobalJobStore()

	// Get or create job status
	job, exists := jobStore.GetJob(runEvent.JobID)
	if !exists {
		job = &DiscoveryJob{
			JobID:     runEvent.JobID,
			ProfileID: runEvent.ProfileID,
			Status:    "running",
			Progress:  0,
			StartedAt: time.Now(),
		}
		jobStore.SetJob(job)
	}

	logger.Info("Starting discovery job execution",
		"job_id", runEvent.JobID.String(),
		"profile_id", runEvent.ProfileID.String(),
	)

	// Fetch the discovery profile
	profile, err := q.GetDiscoveryProfile(ctx, runEvent.ProfileID)
	if err != nil {
		logger.Error("Failed to fetch discovery profile",
			"job_id", runEvent.JobID.String(),
			"profile_id", runEvent.ProfileID.String(),
			"error", err,
		)
		job.Status = "failed"
		job.Error = fmt.Sprintf("Failed to fetch profile: %v", err)
		jobStore.SetJob(job)
		return
	}

	// Parse ports
	var ports []int
	if err := json.Unmarshal(profile.Ports, &ports); err != nil {
		logger.Error("Failed to parse ports",
			"job_id", runEvent.JobID.String(),
			"error", err,
		)
		job.Status = "failed"
		job.Error = fmt.Sprintf("Failed to parse ports: %v", err)
		jobStore.SetJob(job)
		return
	}

	// Decrypt target value (simplified - in production use proper encryption)
	targetIP := profile.TargetValue

	// Clear previous results for this profile
	if err := q.ClearDiscoveredDevices(ctx, uuid.NullUUID{UUID: runEvent.ProfileID, Valid: true}); err != nil {
		logger.Warn("Failed to clear previous discovered devices",
			"job_id", runEvent.JobID.String(),
			"error", err,
		)
	}

	devicesDiscovered := 0
	totalPorts := len(ports)

	// Scan each port
	for idx, port := range ports {
		select {
		case <-ctx.Done():
			job.Status = "failed"
			job.Error = "Discovery operation timed out"
			jobStore.SetJob(job)
			logger.Error("Discovery job timed out", "job_id", runEvent.JobID.String())
			return
		default:
		}

		// Update progress
		job.Progress = (idx * 100) / totalPorts
		jobStore.SetJob(job)

		address := fmt.Sprintf("%s:%d", targetIP, port)
		timeout := time.Duration(profile.PortScanTimeoutMs.Int32) * time.Millisecond
		if timeout == 0 {
			timeout = 1 * time.Second
		}

		// Attempt TCP connection
		conn, err := net.DialTimeout("tcp", address, timeout)
		if err == nil {
			conn.Close()
			// Port is OPEN - create discovered device record
			_, err := q.CreateDiscoveredDevice(ctx, db_gen.CreateDiscoveredDeviceParams{
				DiscoveryProfileID: uuid.NullUUID{UUID: runEvent.ProfileID, Valid: true},
				IpAddress:          pqtype.Inet{IPNet: net.IPNet{IP: net.ParseIP(targetIP), Mask: net.CIDRMask(32, 32)}, Valid: true},
				Hostname:           sql.NullString{String: targetIP, Valid: true},
				Port:               int32(port),
				Status:             sql.NullString{String: "new", Valid: true},
			})

			if err != nil {
				logger.Warn("Failed to create discovered device record",
					"job_id", runEvent.JobID.String(),
					"port", port,
					"error", err,
				)
			} else {
				devicesDiscovered++
				logger.Debug("Discovered device",
					"job_id", runEvent.JobID.String(),
					"ip", targetIP,
					"port", port,
				)
			}
		}
	}

	// Mark job as completed
	now := time.Now()
	job.Status = "completed"
	job.Progress = 100
	job.CompletedAt = &now
	job.DevicesFound = devicesDiscovered
	jobStore.SetJob(job)

	logger.Info("Discovery job completed",
		"job_id", runEvent.JobID.String(),
		"profile_id", runEvent.ProfileID.String(),
		"devices_found", devicesDiscovered,
	)

	// Publish completion event
	completedEvent := DiscoveryCompletedEvent{
		JobID:        runEvent.JobID,
		ProfileID:    runEvent.ProfileID,
		DevicesFound: devicesDiscovered,
		Status:       "success",
	}
	eb.Publish(context.Background(), TopicDiscoveryCompleted, completedEvent)
}
