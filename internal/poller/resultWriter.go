package poller

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/nmslite/nmslite/internal/plugins"
)

// ResultWriter handles writing poll results to the database via BatchWriter
type ResultWriter struct {
	logger      *slog.Logger
	batchWriter *BatchWriter
}

// NewResultWriter creates a new ResultWriter with BatchWriter integration
func NewResultWriter(logger *slog.Logger, batchWriter *BatchWriter) *ResultWriter {
	return &ResultWriter{
		logger:      logger,
		batchWriter: batchWriter,
	}
}

// Write processes poll results and submits metrics to BatchWriter for bulk insertion
func (w *ResultWriter) Write(ctx context.Context, monitorID uuid.UUID, results []plugins.PollResult) {
	timestamp := time.Now()

	for _, result := range results {
		w.logger.Info("poll result received",
			"monitor_id", monitorID,
			"request_id", result.RequestID,
			"status", result.Status,
			"timestamp", result.Timestamp,
			"metric_count", len(result.Metrics),
		)

		// Skip processing if status is not successful
		if result.Status != "success" {
			w.logger.Warn("skipping failed poll result",
				"monitor_id", monitorID,
				"request_id", result.RequestID,
				"status", result.Status,
				"error", result.Error,
			)
			continue
		}

		// Parse metrics from plugin output
		metrics, err := ParseMetricsFromPlugin(monitorID, timestamp, result.Metrics)
		if err != nil {
			w.logger.Error("failed to parse metrics",
				"monitor_id", monitorID,
				"request_id", result.RequestID,
				"error", err,
			)
			continue
		}

		w.logger.Debug("parsed metrics from plugin",
			"monitor_id", monitorID,
			"request_id", result.RequestID,
			"metric_count", len(metrics),
		)

		// Submit each metric to BatchWriter
		for _, metric := range metrics {
			// Convert poller.Metric to MetricRecord for BatchWriter
			record := MetricRecord{
				MonitorID:   monitorID,
				Timestamp:   metric.Timestamp,
				MetricGroup: metric.MetricGroup,
				Tags:        convertTags(metric.Tags),
				ValUsed:     metric.ValUsed,
				ValTotal:    metric.ValTotal,
			}

			// Submit to BatchWriter with context
			if err := w.batchWriter.Submit(ctx, record); err != nil {
				w.logger.Error("failed to submit metric to batch writer",
					"monitor_id", monitorID,
					"request_id", result.RequestID,
					"metric_group", metric.MetricGroup,
					"error", err,
				)
				continue
			}

			w.logger.Debug("metric submitted to batch writer",
				"monitor_id", monitorID,
				"request_id", result.RequestID,
				"metric_group", metric.MetricGroup,
			)
		}

		w.logger.Info("poll result processed successfully",
			"monitor_id", monitorID,
			"request_id", result.RequestID,
			"metrics_submitted", len(metrics),
		)
	}
}

// convertTags converts map[string]string to map[string]interface{} for BatchWriter
func convertTags(tags map[string]string) map[string]interface{} {
	if tags == nil {
		return nil
	}

	result := make(map[string]interface{}, len(tags))
	for k, v := range tags {
		result[k] = v
	}
	return result
}
