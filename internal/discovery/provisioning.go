package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nmslite/nmslite/internal/channels"
	"github.com/nmslite/nmslite/internal/database/dbgen"
	"github.com/nmslite/nmslite/internal/plugins"
)

// Provisioner handles the logic for provisioning monitors from discovered devices.
type Provisioner struct {
	querier  dbgen.Querier
	events   *channels.EventChannels
	registry *plugins.Registry
	logger   *slog.Logger
}

// NewProvisioner creates a new Provisioner.
func NewProvisioner(querier dbgen.Querier, events *channels.EventChannels, registry *plugins.Registry, logger *slog.Logger) *Provisioner {
	return &Provisioner{
		querier:  querier,
		events:   events,
		registry: registry,
		logger:   logger,
	}
}

// ProvisionFromEvent creates a monitor based on a validated discovery event.
func (p *Provisioner) ProvisionFromEvent(ctx context.Context, event channels.DeviceValidatedEvent) error {
	p.logger.InfoContext(ctx, "Provisioning monitor from event",
		slog.String("ip", event.IP),
		slog.String("plugin", event.Plugin.Manifest.ID),
	)

	monitor, err := p.querier.CreateMonitor(ctx, dbgen.CreateMonitorParams{
		IpAddress:           netip.MustParseAddr(event.IP),
		Hostname:            pgtype.Text{String: event.Hostname, Valid: event.Hostname != ""},
		Port:                pgtype.Int4{Int32: int32(event.Port), Valid: true},
		PluginID:            event.Plugin.Manifest.ID,
		CredentialProfileID: event.CredentialProfile.ID,
		DiscoveryProfileID:  event.DiscoveryProfile.ID,
	})
	if err != nil {
		return fmt.Errorf("failed to create monitor: %w", err)
	}

	return p.pushToPoller(ctx, monitor.ID)
}

// ProvisionFromID provisions a monitor from an existing discovered_device ID.
func (p *Provisioner) ProvisionFromID(ctx context.Context, deviceID uuid.UUID) (*dbgen.Monitor, error) {
	// 1. Fetch Discovered Device
	device, err := p.querier.GetDiscoveredDevice(ctx, deviceID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch discovered device: %w", err)
	}

	if !device.DiscoveryProfileID.Valid {
		return nil, fmt.Errorf("discovered device has no associated discovery profile")
	}

	// 2. Fetch Discovery Profile
	profile, err := p.querier.GetDiscoveryProfile(ctx, device.DiscoveryProfileID.UUID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch discovery profile: %w", err)
	}

	// 3. Fetch Credential Profile (to get protocol)
	credProfile, err := p.querier.GetCredentialProfile(ctx, profile.CredentialProfileID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch credential profile: %w", err)
	}

	// 4. Resolve Plugin ID
	// Try to find registered plugin
	pluginID := credProfile.Protocol // Default to protocol name if not internal
	if plugin, err := p.registry.GetByProtocol(credProfile.Protocol); err == nil {
		pluginID = plugin.Manifest.ID
	} else {
		// Fallback for internal protocols or if registry lookup fails (assuming protocol name = plugin id for simple cases)
		// Or we can check if it's one of the known internal ones?
		// Worker.go handles this by creating a placeholder.
		// For provisioning, we need the DB 'plugin_id' logic.
		// If it's internal, usually plugin_id is just the protocol name (e.g. 'ping'?).
		// Wait, worker.go uses `credProfile.Protocol` as placeholder ID.
	}

	p.logger.InfoContext(ctx, "Provisioning monitor from ID",
		slog.String("device_id", deviceID.String()),
		slog.String("ip", device.IpAddress.String()),
		slog.String("plugin", pluginID),
	)

	// 5. Create Monitor
	// Note: Hostname is not available in DiscoveredDevice table, so we leave it empty or null.
	monitor, err := p.querier.CreateMonitor(ctx, dbgen.CreateMonitorParams{
		IpAddress:           device.IpAddress,
		Hostname:            pgtype.Text{Valid: false}, // Unknown
		Port:                pgtype.Int4{Int32: device.Port, Valid: true},
		PluginID:            pluginID,
		CredentialProfileID: profile.CredentialProfileID,
		DiscoveryProfileID:  profile.ID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create monitor: %w", err)
	}

	// 6. Update status of DiscoveredDevice
	if err := p.querier.UpdateDiscoveredDeviceStatus(ctx, dbgen.UpdateDiscoveredDeviceStatusParams{
		ID:     deviceID,
		Status: pgtype.Text{String: "provisioned", Valid: true},
	}); err != nil {
		p.logger.WarnContext(ctx, "Failed to update discovered device status", slog.String("error", err.Error()))
	}

	// 7. Push to Poller
	if err := p.pushToPoller(ctx, monitor.ID); err != nil {
		return &monitor, fmt.Errorf("monitor created but cache invalidation failed: %w", err)
	}

	return &monitor, nil
}

func (p *Provisioner) pushToPoller(ctx context.Context, monitorID uuid.UUID) error {
	fullMonitor, err := p.querier.GetMonitorWithCredentials(ctx, monitorID)
	if err != nil {
		return fmt.Errorf("failed to fetch full monitor for cache: %w", err)
	}

	select {
	case p.events.CacheInvalidate <- channels.CacheInvalidateEvent{
		UpdateType: "update",
		Monitors:   []dbgen.GetMonitorWithCredentialsRow{fullMonitor},
	}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
