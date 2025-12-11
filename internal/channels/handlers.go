package channels

import (
	"context"
	"log/slog"
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
