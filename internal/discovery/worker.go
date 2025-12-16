// Package discovery provides discovery worker and event handling functionality.
package discovery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nmslite/nmslite/internal/auth"
	"github.com/nmslite/nmslite/internal/channels"
	"github.com/nmslite/nmslite/internal/credentials"
	"github.com/nmslite/nmslite/internal/database/dbgen"
	"github.com/nmslite/nmslite/internal/pluginManager"
)

// Worker processes discovery events asynchronously.
type Worker struct {
	events      *channels.EventChannels
	querier     dbgen.Querier
	registry    pluginManager.PluginRegistry
	executor    *pluginManager.Executor
	credentials *credentials.Service
	authService *auth.Service
	logger      *slog.Logger

	// runningMu protects runningProfiles
	runningMu sync.RWMutex
	// runningProfiles tracks which profiles are currently running
	runningProfiles map[uuid.UUID]bool
}

// NewWorker creates a new discovery worker instance with plugin support.
func NewWorker(
	events *channels.EventChannels,
	querier dbgen.Querier,
	registry pluginManager.PluginRegistry,
	executor *pluginManager.Executor,
	credentials *credentials.Service,
	authService *auth.Service,
	logger *slog.Logger,
) *Worker {
	return &Worker{
		events:          events,
		querier:         querier,
		registry:        registry,
		executor:        executor,
		credentials:     credentials,
		authService:     authService,
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

		case event, ok := <-w.events.DiscoveryRequest:
			if !ok {
				w.logger.WarnContext(ctx, "DiscoveryRequest channel closed, exiting worker")
				return fmt.Errorf("discovery started channel closed")
			}

			// Handle the discovery start event - already typed!
			w.handleDiscoveryStartedEvent(ctx, event)
		}
	}
}

// handleDiscoveryStartedEvent processes a single discovery start event.
func (w *Worker) handleDiscoveryStartedEvent(ctx context.Context, event channels.DiscoveryRequestEvent) {
	logger := w.logger.With(
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
		if errors.Is(err, pgx.ErrNoRows) {
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
		slog.String("target_value", profile.TargetValue),
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
	updateErr := w.querier.UpdateDiscoveryProfileStatus(ctx, dbgen.UpdateDiscoveryProfileStatusParams{
		ID:                profile.ID,
		LastRunStatus:     pgtype.Text{String: status, Valid: true},
		DevicesDiscovered: pgtype.Int4{Int32: int32(monitorCount), Valid: true},
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

// executeDiscovery runs discovery with protocol-specific handshake validation
func (w *Worker) executeDiscovery(
	ctx context.Context,
	profile dbgen.DiscoveryProfile,
	logger *slog.Logger,
) (int, error) {
	// Get port and credential from profile (now single values)
	port := int(profile.Port)
	credentialID := profile.CredentialProfileID

	// Decrypt target value
	decryptedTarget := profile.TargetValue
	if decrypted, err := w.authService.Decrypt(profile.TargetValue); err == nil {
		decryptedTarget = string(decrypted)
	} else {
		// If decryption fails, log it but try using raw value (backward compatibility or unencrypted)
		logger.WarnContext(ctx, "Failed to decrypt target value, using raw value",
			slog.String("error", err.Error()),
		)
	}

	// Expand target into individual IPs (handles CIDR, ranges, and single IPs)
	targetIPs, err := ExpandTarget(decryptedTarget)
	if err != nil {
		return 0, fmt.Errorf("failed to expand target value: %w", err)
	}

	logger.InfoContext(ctx, "Target expanded to IPs",
		slog.String("target", decryptedTarget),
		slog.Int("ip_count", len(targetIPs)),
		slog.String("target_type", string(DetectTargetType(decryptedTarget))),
		slog.Int("port", port),
		slog.String("credential_id", credentialID.String()),
	)

	// Get handshake timeout, default to 5 seconds if not set
	handshakeTimeout := time.Duration(5000) * time.Millisecond
	if profile.PortScanTimeoutMs.Valid && profile.PortScanTimeoutMs.Int32 > 0 {
		handshakeTimeout = time.Duration(profile.PortScanTimeoutMs.Int32) * time.Millisecond
	}

	// Find pluginManager that handle this port
	matchingPlugins := w.registry.GetByPort(port)
	if len(matchingPlugins) == 0 {
		return 0, fmt.Errorf("no plugin found for port %d", port)
	}

	// Get credential profile
	credProfile, err := w.querier.GetCredentialProfile(ctx, credentialID)
	if err != nil {
		return 0, fmt.Errorf("failed to fetch credential profile: %w", err)
	}

	// Get decrypted credentials
	creds, err := w.credentials.GetDecrypted(ctx, credentialID)
	if err != nil {
		return 0, fmt.Errorf("failed to decrypt credentials: %w", err)
	}

	validatedCount := 0

	// For each IP address in the expanded target
	for _, targetIP := range targetIPs {
		// Check context cancellation
		if err := ctx.Err(); err != nil {
			return validatedCount, fmt.Errorf("discovery cancelled: %w", err)
		}

		// Try handshake with each matching plugin
		validated := false
		for _, plugin := range matchingPlugins {
			var result *HandshakeResult

			switch plugin.Manifest.Protocol {
			case "ssh":
				result, _ = ValidateSSH(targetIP, port, creds, handshakeTimeout)
			case "winrm":
				result, _ = ValidateWinRM(targetIP, port, creds, handshakeTimeout)
			case "snmp-v2c":
				result, _ = ValidateSNMPv2c(targetIP, port, creds, handshakeTimeout)
			case "snmp-v3":
				result, _ = ValidateSNMPv3(targetIP, port, creds, handshakeTimeout)
			default:
				logger.WarnContext(ctx, "Unknown protocol, skipping handshake",
					slog.String("protocol", plugin.Manifest.Protocol),
				)
				continue
			}

			if result != nil && result.Success {
				logger.InfoContext(ctx, "Protocol handshake succeeded",
					slog.String("ip", targetIP),
					slog.Int("port", port),
					slog.String("protocol", plugin.Manifest.Protocol),
					slog.String("credential_id", credentialID.String()),
				)

				// Publish DeviceValidatedEvent - handler creates DB entries
				select {
				case w.events.DeviceValidated <- channels.DeviceValidatedEvent{
					DiscoveryProfile:  profile,
					CredentialProfile: credProfile,
					Plugin:            plugin,
					IP:                targetIP,
					Port:              port,
				}:
					validatedCount++
					validated = true
				case <-ctx.Done():
					return validatedCount, ctx.Err()
				default:
					logger.WarnContext(ctx, "DeviceValidated channel full, event dropped")
				}

				break // First plugin success for this IP, move to next IP
			}
		}

		if !validated {
			logger.DebugContext(ctx, "No valid handshake for IP",
				slog.String("ip", targetIP),
				slog.Int("port", port),
			)
		}
	}

	return validatedCount, nil
}

// isPortOpen checks if a TCP port is open on the target

// publishCompletedEvent publishes a discovery completion event to the event bus.
func (w *Worker) publishCompletedEvent(
	ctx context.Context,
	event channels.DiscoveryRequestEvent,
	statusStr string,
	deviceCount int,
	_ string, // error message (reserved for future use)
) {
	completedEvent := channels.DiscoveryStatusEvent{
		ProfileID:    event.ProfileID,
		Status:       statusStr, // "success", "partial", "failed"
		DevicesFound: deviceCount,
		StartedAt:    event.StartedAt,
		CompletedAt:  time.Now(),
	}

	// Non-blocking send with context
	select {
	case w.events.DiscoveryStatus <- completedEvent:
		w.logger.DebugContext(ctx, "Published discovery completed event",
			slog.String("profile_id", event.ProfileID.String()),
			slog.String("status", statusStr),
			slog.Int("devices_found", deviceCount),
		)
	case <-ctx.Done():
		w.logger.WarnContext(ctx, "Context cancelled while publishing completion event",
			slog.String("profile_id", event.ProfileID.String()),
		)
	default:
		// Channel full - log warning
		w.logger.WarnContext(ctx, "DiscoveryCompleted channel full, event dropped",
			slog.String("profile_id", event.ProfileID.String()),
			slog.String("status", statusStr),
		)
	}
}
