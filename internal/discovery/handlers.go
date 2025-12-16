package discovery

import (
	"context"
	"log/slog"
	"net/netip"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nmslite/nmslite/internal/channels"
	"github.com/nmslite/nmslite/internal/database/dbgen"
)

// StartDiscoveryCompletionLogger starts a goroutine that logs discovery completion events AND broadcasts them.
func StartDiscoveryCompletionLogger(ctx context.Context, events *channels.EventChannels, hub *Hub, logger *slog.Logger) {
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

// StartProvisionHandler listens for DeviceValidatedEvent, creates DB entries, and broadcasts progress.
func StartProvisionHandler(ctx context.Context, events *channels.EventChannels, querier dbgen.Querier, hub *Hub, logger *slog.Logger) {
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

				// Broadcast device found event immediately
				hub.Broadcast("device_found", event.DiscoveryProfile.ID, map[string]interface{}{
					"ip":       event.IP,
					"port":     event.Port,
					"protocol": event.Plugin.Manifest.Protocol,
					"plugin":   event.Plugin.Manifest.ID,
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

				// 2. If auto_provision → Create monitor
				if event.DiscoveryProfile.AutoProvision.Valid && event.DiscoveryProfile.AutoProvision.Bool {
					logger.InfoContext(ctx, "Auto-provision enabled, creating monitor",
						slog.String("ip", event.IP),
						slog.Int("port", event.Port),
					)

					monitor, err := querier.CreateMonitor(ctx, dbgen.CreateMonitorParams{
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

					// 3. Emit cache invalidation for the new monitor
					fullMonitor, err := querier.GetMonitorWithCredentials(ctx, monitor.ID)
					if err != nil {
						logger.Error("failed to fetch created monitor for cache invalidation", "error", err)
						continue
					}

					select {
					case events.CacheInvalidate <- channels.CacheInvalidateEvent{
						UpdateType: "update",
						Monitors:   []dbgen.GetMonitorWithCredentialsRow{fullMonitor},
					}:
					case <-ctx.Done():
						return
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
