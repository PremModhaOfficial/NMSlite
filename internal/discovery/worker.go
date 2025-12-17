// Package discovery provides discovery worker and event handling functionality.
package discovery

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	auth2 "github.com/nmslite/nmslite/internal/api/auth"
	"github.com/nmslite/nmslite/internal/database/dbgen"
	"github.com/nmslite/nmslite/internal/globals"
	"github.com/nmslite/nmslite/internal/poller"
)

// Worker processes discovery events asynchronously.
type Worker struct {
	events        *globals.EventChannels
	querier       dbgen.Querier
	pluginManager *poller.PluginManager // Renamed from registry
	// executor    *plugins.Executor - REMOVED (not used in discovery)
	credentials *auth2.CredentialService
	authService *auth2.Service
	logger      *slog.Logger

	// runningMu protects runningProfiles
	runningMu sync.RWMutex
	// runningProfiles tracks which profiles are currently running
	runningProfiles map[uuid.UUID]bool
}

// NewWorker creates a new discovery worker instance with plugin support.
func NewWorker(
	events *globals.EventChannels,
	querier dbgen.Querier,
	pluginManager *poller.PluginManager,
	// executor *plugins.Executor, - REMOVED
	credentials *auth2.CredentialService,
	authService *auth2.Service,
	logger *slog.Logger,
) *Worker {
	return &Worker{
		events:        events,
		querier:       querier,
		pluginManager: pluginManager,
		// executor:        executor,
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
func (w *Worker) handleDiscoveryStartedEvent(ctx context.Context, event globals.DiscoveryRequestEvent) {
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

	// Update status to RUNNING
	updateRunErr := w.querier.UpdateDiscoveryProfileStatus(ctx, dbgen.UpdateDiscoveryProfileStatusParams{
		ID:                profile.ID,
		LastRunStatus:     pgtype.Text{String: "running", Valid: true},
		DevicesDiscovered: pgtype.Int4{Int32: 0, Valid: true},
	})
	if updateRunErr != nil {
		logger.ErrorContext(ctx, "Failed to update discovery profile status to running",
			slog.String("error", updateRunErr.Error()),
		)
		// Continue even if status update failed, main logic is execution
	}

	// Execute discovery
	monitorCount, totalIPs, jobErr := w.executeDiscovery(ctx, profile, logger)

	// Determine final status based on discovery results:
	// - "success": all IPs discovered (monitorCount == totalIPs)
	// - "partial": some devices discovered (0 < monitorCount < totalIPs)
	// - "failed": zero devices discovered or execution error
	var status string
	if jobErr != nil {
		status = "failed"
		logger.ErrorContext(ctx, "Discovery execution failed",
			slog.String("error", jobErr.Error()),
		)
	} else if monitorCount == 0 {
		status = "failed"
		logger.WarnContext(ctx, "Discovery completed but no devices were found")
	} else if monitorCount < totalIPs {
		status = "partial"
		logger.InfoContext(ctx, "Discovery completed with partial results",
			slog.Int("discovered", monitorCount),
			slog.Int("total_ips", totalIPs),
		)
	} else {
		status = "success"
	}

	// Update discovery profile status in database - FINAL
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

// executeDiscovery runs discovery with protocol-specific handshake validation.
// Returns: (validatedCount, totalIPCount, error)
func (w *Worker) executeDiscovery(
	ctx context.Context,
	profile dbgen.DiscoveryProfile,
	logger *slog.Logger,
) (int, int, error) {
	// Get port and credential from profile
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
		return 0, 0, fmt.Errorf("failed to expand target value: %w", err)
	}

	// Get credential profile to determine protocol
	credProfile, err := w.querier.GetCredentialProfile(ctx, credentialID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to fetch credential profile: %w", err)
	}

	logger.InfoContext(ctx, "Target expanded to IPs",
		slog.String("target", decryptedTarget),
		slog.Int("ip_count", len(targetIPs)),
		slog.String("target_type", string(DetectTargetType(decryptedTarget))),
		slog.Int("port", port),
		slog.String("credential_id", credentialID.String()),
		slog.String("protocol", credProfile.Protocol),
	)

	// Get handshake timeout, default to 5 seconds if not set
	handshakeTimeout := time.Duration(5000) * time.Millisecond
	if profile.PortScanTimeoutMs.Valid && profile.PortScanTimeoutMs.Int32 > 0 {
		handshakeTimeout = time.Duration(profile.PortScanTimeoutMs.Int32) * time.Millisecond
	}

	// Resolve plugin/protocol handler
	// Attempt to find registered plugin for the protocol
	var plugin *globals.PluginInfo
	registeredPlugin, ok := w.pluginManager.Get(credProfile.Protocol)
	if ok {
		plugin = registeredPlugin
	} else {
		// If not found in registry (e.g. internal ssh/snmp), create a placeholder
		// This ensures we can still pass a valid PluginInfo to the event handler
		plugin = &globals.PluginInfo{
			Name:     credProfile.Protocol, // Use protocol as name
			Protocol: credProfile.Protocol,
		}
		logger.DebugContext(ctx, "Using internal/placeholder plugin for protocol",
			slog.String("protocol", credProfile.Protocol),
		)
	}

	// Get decrypted credentials
	creds, err := w.credentials.GetDecrypted(ctx, credentialID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to decrypt credentials: %w", err)
	}

	validatedCount := 0

	// For each IP address in the expanded target
	for _, targetIP := range targetIPs {
		// Check context cancellation
		if err := ctx.Err(); err != nil {
			return validatedCount, len(targetIPs), fmt.Errorf("discovery cancelled: %w", err)
		}

		// Delegated validation logic - pass the single resolved plugin (real or placeholder)
		// We wrap it in a slice because validateTarget expects a list (though we only check one now)
		validatedPlugin, hostname, valid := w.validateTarget(ctx, targetIP, port, creds, handshakeTimeout, []*globals.PluginInfo{plugin}, logger)

		if valid {
			logger.InfoContext(ctx, "Protocol handshake succeeded",
				slog.String("ip", targetIP),
				slog.Int("port", port),
				slog.String("protocol", validatedPlugin.Protocol),
				slog.String("credential_id", credentialID.String()),
			)

			// Publish DeviceValidatedEvent - handler creates DB entries
			select {
			case w.events.DeviceValidated <- globals.DeviceValidatedEvent{
				DiscoveryProfile:  profile,
				CredentialProfile: credProfile,
				Plugin:            validatedPlugin,
				IP:                targetIP,
				Port:              port,
				Hostname:          hostname,
			}:
				validatedCount++
			case <-ctx.Done():
				return validatedCount, len(targetIPs), ctx.Err()
			default:
				logger.WarnContext(ctx, "DeviceValidated channel full, event dropped")
			}
		} else {
			logger.DebugContext(ctx, "No valid handshake for IP",
				slog.String("ip", targetIP),
				slog.Int("port", port),
			)
		}
	}

	return validatedCount, len(targetIPs), nil
}

// validateTarget attempts to validate an IP against a list of plugins
func (w *Worker) validateTarget(
	ctx context.Context,
	ip string,
	port int,
	creds *auth2.Credentials,
	timeout time.Duration,
	plugins []*globals.PluginInfo,
	logger *slog.Logger,
) (*globals.PluginInfo, string, bool) {

	for _, plugin := range plugins {
		var result *HandshakeResult

		switch plugin.Protocol {
		case "ssh":
			result, _ = ValidateSSH(ip, port, creds, timeout)
		case "winrm":
			result, _ = ValidateWinRM(ip, port, creds, timeout)
		case "snmp-v2c":
			result, _ = ValidateSNMPv2c(ip, port, creds, timeout)
		case "snmp-v3":
			result, _ = ValidateSNMPv3(ip, port, creds, timeout)
		default:
			logger.WarnContext(ctx, "Unknown protocol, skipping handshake",
				slog.String("protocol", plugin.Protocol),
			)
			continue
		}

		if result != nil && result.Success {
			return plugin, result.Hostname, true
		}
	}

	return nil, "", false
}

// isPortOpen checks if a TCP port is open on the target

// publishCompletedEvent publishes a discovery completion event to the event bus.
func (w *Worker) publishCompletedEvent(
	ctx context.Context,
	event globals.DiscoveryRequestEvent,
	statusStr string,
	deviceCount int,
	_ string, // error message (reserved for future use)
) {
	completedEvent := globals.DiscoveryStatusEvent{
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

// StartDiscoveryCompletionLogger starts a goroutine that logs discovery completion events AND broadcasts them.
func StartDiscoveryCompletionLogger(ctx context.Context, events *globals.EventChannels, hub *Hub, logger *slog.Logger) {
	go func() {
		for {
			select {
			case event, ok := <-events.DiscoveryStatus:
				if !ok {
					return
				}
				logger.InfoContext(ctx, "Discovery completed",
					slog.String("profile_id", event.ProfileID.String()),
					slog.String("status", event.Status),
					slog.Int("devices_found", event.DevicesFound),
					slog.String("duration", event.CompletedAt.Sub(event.StartedAt).String()),
				)

				// Broadcast to websocket
				hub.Broadcast("status_change", event.ProfileID, map[string]interface{}{
					"status":        event.Status,
					"devices_found": event.DevicesFound,
					"duration":      event.CompletedAt.Sub(event.StartedAt).String(),
				})

			case <-ctx.Done():
				return
			case <-events.Done():
				return
			}
		}
	}()
}

// StartProvisionHandler listens for DeviceValidatedEvent, creates DB entries, and broadcasts.
func StartProvisionHandler(ctx context.Context, events *globals.EventChannels, querier dbgen.Querier, hub *Hub, logger *slog.Logger, provisioner *Provisioner) {
	go func() {
		for {
			select {
			case event, ok := <-events.DeviceValidated:
				if !ok {
					return
				}

				logger.InfoContext(ctx, "Device validated, creating discovered_devices entry",
					slog.String("ip", event.IP),
					slog.Int("port", event.Port),
					slog.String("protocol", event.Plugin.Protocol),
				)

				// Broadcast device found event immediately
				hub.Broadcast("device_found", event.DiscoveryProfile.ID, map[string]interface{}{
					"ip":       event.IP,
					"port":     event.Port,
					"protocol": event.Plugin.Protocol,
					"plugin":   event.Plugin.Name,
				})

				// 1. Create discovered_devices entry
				_, err := querier.CreateDiscoveredDevice(ctx, dbgen.CreateDiscoveredDeviceParams{
					DiscoveryProfileID: uuid.NullUUID{UUID: event.DiscoveryProfile.ID, Valid: true},
					IpAddress:          netip.MustParseAddr(event.IP),
					Port:               int32(event.Port),
					Status:             pgtype.Text{String: "validated", Valid: true},
				})
				if err != nil {
					logger.ErrorContext(ctx, "Failed to create discovered_devices entry",
						slog.String("ip", event.IP),
						slog.String("error", err.Error()),
					)
					continue
				}

				// 2. If auto_provision → Use Provisioner
				if event.DiscoveryProfile.AutoProvision.Valid && event.DiscoveryProfile.AutoProvision.Bool {
					if err := provisioner.ProvisionFromEvent(ctx, event); err != nil {
						logger.ErrorContext(ctx, "Failed to auto-provision monitor",
							slog.String("error", err.Error()),
							slog.String("ip", event.IP),
						)
					} else {
						logger.InfoContext(ctx, "Monitor created via auto-provision",
							slog.String("ip", event.IP),
						)
					}
				}

			case <-ctx.Done():
				return
			case <-events.Done():
				return
			}
		}
	}()
}
