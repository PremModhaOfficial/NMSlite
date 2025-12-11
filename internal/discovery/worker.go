// Package discovery provides discovery worker and event handling functionality.
package discovery

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/nmslite/nmslite/internal/channels"
	"github.com/nmslite/nmslite/internal/credentials"
	"github.com/nmslite/nmslite/internal/database/db_gen"
	"github.com/nmslite/nmslite/internal/plugins"
	"github.com/sqlc-dev/pqtype"
)

// Worker processes discovery events asynchronously and manages job lifecycle.
type Worker struct {
	events      *channels.EventChannels
	querier     db_gen.Querier
	registry    plugins.PluginRegistry
	executor    *plugins.Executor
	credentials *credentials.Service
	logger      *slog.Logger

	// runningMu protects runningProfiles
	runningMu sync.RWMutex
	// runningProfiles tracks which profiles are currently running
	runningProfiles map[uuid.UUID]bool
}

// NewWorker creates a new discovery worker instance with plugin support.
func NewWorker(
	events *channels.EventChannels,
	querier db_gen.Querier,
	registry plugins.PluginRegistry,
	executor *plugins.Executor,
	credentials *credentials.Service,
	logger *slog.Logger,
) *Worker {
	return &Worker{
		events:          events,
		querier:         querier,
		registry:        registry,
		executor:        executor,
		credentials:     credentials,
		logger:          logger,
		runningProfiles: make(map[uuid.UUID]bool),
	}
}

// Run starts the discovery worker and begins processing discovery events.
func (w *Worker) Run(ctx context.Context) error {
	w.logger.InfoContext(ctx, "Discovery worker starting (with plugin support, channels-based)",
		slog.String("worker", "discovery"),
	)

	for {
		select {
		case <-ctx.Done():
			w.logger.InfoContext(ctx, "Discovery worker shutting down",
				slog.String("reason", ctx.Err().Error()),
			)
			return ctx.Err()

		case event, ok := <-w.events.DiscoveryStarted:
			if !ok {
				w.logger.WarnContext(ctx, "DiscoveryStarted channel closed, exiting worker")
				return fmt.Errorf("discovery started channel closed")
			}

			// Handle the discovery start event - already typed!
			w.handleDiscoveryStartedEvent(ctx, event)
		}
	}
}

// handleDiscoveryStartedEvent processes a single discovery start event.
func (w *Worker) handleDiscoveryStartedEvent(ctx context.Context, event channels.DiscoveryStartedEvent) {
	jobID := event.JobID.String()
	logger := w.logger.With(
		slog.String("job_id", jobID),
		slog.String("profile_id", event.ProfileID.String()),
		slog.String("started_at", event.StartedAt.Format(time.RFC3339)),
	)

	// Check if profile is already running
	w.runningMu.RLock()
	isRunning := w.runningProfiles[event.ProfileID]
	w.runningMu.RUnlock()

	if isRunning {
		logger.WarnContext(ctx, "Discovery already running for this profile, skipping duplicate")
		w.publishCompletedEvent(ctx, event, "failed", 0, "duplicate discovery run detected")
		return
	}

	// Validate discovery profile exists
	profile, err := w.querier.GetDiscoveryProfile(ctx, event.ProfileID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
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

	// Mark profile as running
	w.runningMu.Lock()
	w.runningProfiles[event.ProfileID] = true
	w.runningMu.Unlock()
	defer func() {
		w.runningMu.Lock()
		delete(w.runningProfiles, event.ProfileID)
		w.runningMu.Unlock()
	}()

	logger.InfoContext(ctx, "Starting discovery run (plugin-based)",
		slog.String("profile_name", profile.Name),
		slog.String("target_type", profile.TargetType),
	)

	// Execute discovery
	monitorCount, jobErr := w.executeDiscovery(ctx, profile, logger)

	// Determine final status
	status := "success"
	if jobErr != nil {
		status = "failed"
		logger.ErrorContext(ctx, "Discovery execution failed",
			slog.String("error", jobErr.Error()),
		)
	}

	// Update discovery profile status in database
	updateErr := w.querier.UpdateDiscoveryProfileStatus(ctx, db_gen.UpdateDiscoveryProfileStatusParams{
		ID:                profile.ID,
		LastRunStatus:     sql.NullString{String: status, Valid: true},
		DevicesDiscovered: sql.NullInt32{Int32: int32(monitorCount), Valid: true},
	})
	if updateErr != nil {
		logger.ErrorContext(ctx, "Failed to update discovery profile status",
			slog.String("error", updateErr.Error()),
		)
	}

	// Publish completion event
	w.publishCompletedEvent(ctx, event, status, monitorCount, "")

	logger.InfoContext(ctx, "Discovery run completed",
		slog.String("status", status),
		slog.Int("monitors_created", monitorCount),
	)
}

// executeDiscovery runs the simplified auto-provision discovery process.
func (w *Worker) executeDiscovery(
	ctx context.Context,
	profile db_gen.DiscoveryProfile,
	logger *slog.Logger,
) (int, error) {
	// 1. Parse ports and credential IDs
	var ports []int
	if err := json.Unmarshal(profile.Ports, &ports); err != nil {
		return 0, fmt.Errorf("failed to parse ports: %w", err)
	}

	var credentialIDs []string
	if err := json.Unmarshal(profile.CredentialProfileIds, &credentialIDs); err != nil {
		return 0, fmt.Errorf("failed to parse credential_profile_ids: %w", err)
	}

	if len(credentialIDs) == 0 {
		return 0, fmt.Errorf("no credential profiles specified")
	}

	// Decrypt target value
	decryptedTarget := profile.TargetValue // TODO: Add decryption when auth service available

	// Get port scan timeout, default to 1 second if not set
	timeout := time.Duration(1000) * time.Millisecond
	if profile.PortScanTimeoutMs.Valid && profile.PortScanTimeoutMs.Int32 > 0 {
		timeout = time.Duration(profile.PortScanTimeoutMs.Int32) * time.Millisecond
	}

	monitorsCreated := 0

	// 2. For each port
	for _, port := range ports {
		// Check context cancellation
		if err := ctx.Err(); err != nil {
			return monitorsCreated, fmt.Errorf("discovery cancelled: %w", err)
		}

		// 2a. TCP liveness check
		if !w.isPortOpen(ctx, decryptedTarget, port, timeout) {
			logger.DebugContext(ctx, "Port closed or unreachable",
				slog.String("ip", decryptedTarget),
				slog.Int("port", port),
			)
			continue
		}

		logger.InfoContext(ctx, "Port open, saving to discovered_devices",
			slog.String("ip", decryptedTarget),
			slog.Int("port", port),
		)

		// 2b. Save to discovered_devices table
		discoveredDevice, err := w.querier.CreateDiscoveredDevice(ctx, db_gen.CreateDiscoveredDeviceParams{
			DiscoveryProfileID: uuid.NullUUID{UUID: profile.ID, Valid: true},
			IpAddress:          pqtype.Inet{IPNet: net.IPNet{IP: net.ParseIP(decryptedTarget), Mask: net.CIDRMask(32, 32)}, Valid: true},
			Hostname:           sql.NullString{Valid: false}, // null initially
			Port:               int32(port),
			Status:             sql.NullString{String: "new", Valid: true},
		})

		if err != nil {
			logger.ErrorContext(ctx, "Failed to save discovered device",
				slog.String("ip", decryptedTarget),
				slog.Int("port", port),
				slog.String("error", err.Error()),
			)
			continue
		}

		// 2c. If auto_provision is enabled, create monitor
		if profile.AutoProvision.Valid && profile.AutoProvision.Bool {
			logger.InfoContext(ctx, "Auto-provision enabled, creating monitor",
				slog.String("ip", decryptedTarget),
				slog.Int("port", port),
			)

			// Find plugins that handle this port
			matchingPlugins := w.registry.GetByPort(port)
			if len(matchingPlugins) == 0 {
				logger.InfoContext(ctx, "No plugin found for port, skipping auto-provision",
					slog.Int("port", port),
				)
				continue
			}

			// Take first plugin
			plugin := matchingPlugins[0]

			// Take first credential
			credID, err := uuid.Parse(credentialIDs[0])
			if err != nil {
				logger.WarnContext(ctx, "Invalid credential ID",
					slog.String("credential_id", credentialIDs[0]),
					slog.String("error", err.Error()),
				)
				continue
			}

			// Create monitor
			_, err = w.querier.CreateMonitor(ctx, db_gen.CreateMonitorParams{
				DisplayName:         sql.NullString{Valid: false}, // Will be populated during polling
				Hostname:            sql.NullString{Valid: false}, // Will be populated during polling
				IpAddress:           pqtype.Inet{IPNet: net.IPNet{IP: net.ParseIP(decryptedTarget), Mask: net.CIDRMask(32, 32)}, Valid: true},
				PluginID:            plugin.Manifest.ID,
				CredentialProfileID: uuid.NullUUID{UUID: credID, Valid: true},
				DiscoveryProfileID:  uuid.NullUUID{UUID: profile.ID, Valid: true},
				Port:                sql.NullInt32{Int32: int32(port), Valid: true},
			})

			if err != nil {
				logger.ErrorContext(ctx, "Failed to create monitor",
					slog.String("plugin", plugin.Manifest.ID),
					slog.String("error", err.Error()),
				)
				continue
			}

			// Update discovered_device status to "provisioned"
			err = w.querier.UpdateDiscoveredDeviceStatus(ctx, db_gen.UpdateDiscoveredDeviceStatusParams{
				ID:     discoveredDevice.ID,
				Status: sql.NullString{String: "provisioned", Valid: true},
			})

			if err != nil {
				logger.WarnContext(ctx, "Failed to update discovered device status",
					slog.String("device_id", discoveredDevice.ID.String()),
					slog.String("error", err.Error()),
				)
			}

			monitorsCreated++
			logger.InfoContext(ctx, "Monitor auto-created for device",
				slog.String("ip", decryptedTarget),
				slog.Int("port", port),
				slog.String("plugin", plugin.Manifest.ID),
			)
		}
	}

	return monitorsCreated, nil
}

// isPortOpen checks if a TCP port is open on the target
func (w *Worker) isPortOpen(ctx context.Context, target string, port int, timeout time.Duration) bool {
	address := fmt.Sprintf("%s:%d", target, port)

	scanCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(scanCtx, "tcp", address)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// publishCompletedEvent publishes a discovery completion event to the event bus.
func (w *Worker) publishCompletedEvent(
	ctx context.Context,
	event channels.DiscoveryStartedEvent,
	statusStr string,
	deviceCount int,
	_ string, // error message (reserved for future use)
) {
	completedEvent := channels.DiscoveryCompletedEvent{
		JobID:        event.JobID,
		ProfileID:    event.ProfileID,
		Status:       statusStr, // "success", "partial", "failed"
		DevicesFound: deviceCount,
		StartedAt:    event.StartedAt,
		CompletedAt:  time.Now(),
	}

	// Non-blocking send with context
	select {
	case w.events.DiscoveryCompleted <- completedEvent:
		w.logger.DebugContext(ctx, "Published discovery completed event",
			slog.String("job_id", event.JobID.String()),
			slog.String("status", statusStr),
			slog.Int("devices_found", deviceCount),
		)
	case <-ctx.Done():
		w.logger.WarnContext(ctx, "Context cancelled while publishing completion event",
			slog.String("job_id", event.JobID.String()),
		)
	default:
		// Channel full - log warning
		w.logger.WarnContext(ctx, "DiscoveryCompleted channel full, event dropped",
			slog.String("job_id", event.JobID.String()),
			slog.String("status", statusStr),
		)
	}
}
