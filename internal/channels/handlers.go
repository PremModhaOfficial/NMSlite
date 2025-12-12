package channels

import (
	"context"
	"log/slog"
	"net/netip"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nmslite/nmslite/internal/database/dbgen"
)

// StartDiscoveryCompletionLogger starts a goroutine that logs discovery completion events.
// This replaces the old eventbus handlers pattern with a simple channel consumer.
func StartDiscoveryCompletionLogger(ctx context.Context, events *EventChannels, logger *slog.Logger) {
	go func() {
		for {
			select {
			case event, ok := <-events.DiscoveryCompleted:
				if !ok {
					return
				}
				logger.InfoContext(ctx, "Discovery completed",
					slog.String("profile_id", event.ProfileID.String()),
					slog.String("status", event.Status),
					slog.Int("devices_found", event.DevicesFound),
					slog.String("duration", event.CompletedAt.Sub(event.StartedAt).String()),
				)
			case <-ctx.Done():
				return
			case <-events.Done():
				return
			}
		}
	}()
}

// StartProvisionHandler listens for DeviceValidatedEvent and creates discovered devices and monitors
func StartProvisionHandler(ctx context.Context, events *EventChannels, querier dbgen.Querier, logger *slog.Logger) {
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
					slog.String("protocol", event.Plugin.Manifest.Protocol),
				)

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

				// 2. If auto_provision → Create monitor
				if event.DiscoveryProfile.AutoProvision.Valid && event.DiscoveryProfile.AutoProvision.Bool {
					logger.InfoContext(ctx, "Auto-provision enabled, creating monitor",
						slog.String("ip", event.IP),
						slog.Int("port", event.Port),
					)

					_, err := querier.CreateMonitor(ctx, dbgen.CreateMonitorParams{
						IpAddress:           netip.MustParseAddr(event.IP),
						Port:                pgtype.Int4{Int32: int32(event.Port), Valid: true},
						PluginID:            event.Plugin.Manifest.ID,
						CredentialProfileID: event.CredentialProfile.ID,
						DiscoveryProfileID:  event.DiscoveryProfile.ID,
					})
					if err != nil {
						logger.ErrorContext(ctx, "Failed to create monitor",
							slog.String("plugin", event.Plugin.Manifest.ID),
							slog.String("error", err.Error()),
						)
						continue
					}

					logger.InfoContext(ctx, "Monitor created via auto-provision",
						slog.String("ip", event.IP),
						slog.String("plugin", event.Plugin.Manifest.ID),
					)
				}

			case <-ctx.Done():
				return
			case <-events.Done():
				return
			}
		}
	}()
}
