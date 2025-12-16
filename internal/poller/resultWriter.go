package poller

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/nmslite/nmslite/internal/globals"
	// plugins "github.com/nmslite/nmslite/internal/plugins" - REMOVED
)

// ResultWriter handles writing poll results to the database via BatchWriter
type ResultWriter struct {
	logger      *slog.Logger
	batchWriter *BatchWriter
}

// NewResultWriter creates a new ResultWriter
func NewResultWriter(batchWriter *BatchWriter) *ResultWriter {
	return &ResultWriter{
		batchWriter: batchWriter,
		logger:      slog.Default(),
	}
}

// Write processes poll results and submits metrics to BatchWriter for bulk insertion
func (w *ResultWriter) Write(ctx context.Context, monitorID uuid.UUID, results []globals.PollResult) {
	timestamp := time.Now()

	for _, result := range results {
		w.logger.Info("poll result received",
			"monitor_id", monitorID,
			"request_id", result.RequestID,
			"status", result.Status,
			"timestamp", result.Timestamp,
			"metric_count", len(result.Metrics),
		)

		if result.Status != "success" {
			w.logger.Warn("skipping failed poll result",
				"monitor_id", monitorID,
				"request_id", result.RequestID,
				"status", result.Status,
				"error", result.Error,
			)
			continue
		}

		metrics, err := parseMetricsFromPlugin(monitorID, timestamp, result.Metrics)
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

		for _, record := range metrics {
			if err := w.batchWriter.Submit(ctx, record); err != nil {
				w.logger.Error("failed to submit metric to batch writer",
					"monitor_id", monitorID,
					"request_id", result.RequestID,
					"name", record.Name,
					"error", err,
				)
				continue
			}

			w.logger.Debug("metric submitted to batch writer",
				"monitor_id", monitorID,
				"request_id", result.RequestID,
				"name", record.Name,
			)
		}

		w.logger.Info("poll result processed successfully",
			"monitor_id", monitorID,
			"request_id", result.RequestID,
			"metrics_submitted", len(metrics),
		)
	}
}

// parseMetricsFromPlugin converts plugin output to typed MetricRecord
// raw is expected to be an array of metric objects from the plugin
func parseMetricsFromPlugin(monitorID uuid.UUID, timestamp time.Time, raw []interface{}) ([]MetricRecord, error) {
	if raw == nil {
		return nil, fmt.Errorf("raw metrics data is nil")
	}

	if len(raw) == 0 {
		return []MetricRecord{}, nil
	}

	metrics := make([]MetricRecord, 0, len(raw))

	for i, item := range raw {
		metricMap, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("metric at index %d is not a valid object: %T", i, item)
		}

		record, err := parseMetricFromMap(metricMap, monitorID, timestamp)
		if err != nil {
			return nil, fmt.Errorf("failed to parse metric at index %d: %w", i, err)
		}

		metrics = append(metrics, record)
	}

	return metrics, nil
}

// parseMetricFromMap converts a map to a MetricRecord struct
func parseMetricFromMap(data map[string]interface{}, monitorID uuid.UUID, defaultTimestamp time.Time) (MetricRecord, error) {
	record := MetricRecord{
		Timestamp: defaultTimestamp,
		MonitorID: monitorID,
		Type:      "gauge", // Default type
	}

	// Parse name (required)
	if name, ok := data["name"].(string); ok && name != "" {
		record.Name = name
	} else {
		return record, fmt.Errorf("missing or invalid 'name' field")
	}

	// Parse value (required)
	value, err := parseFloat(data["value"])
	if err != nil {
		return record, fmt.Errorf("invalid 'value': %w", err)
	}
	record.Value = value

	// Parse type (optional, defaults to "gauge")
	if metricType, ok := data["type"].(string); ok && metricType != "" {
		record.Type = metricType
	}

	// Parse timestamp (optional, use default if not provided)
	if ts, ok := data["timestamp"].(string); ok {
		parsedTime, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			return record, fmt.Errorf("invalid timestamp format: %w", err)
		}
		record.Timestamp = parsedTime
	}

	return record, nil
}

// parseFloat attempts to convert various numeric types to float64
func parseFloat(val interface{}) (float64, error) {
	if val == nil {
		return 0, fmt.Errorf("value is nil")
	}
	switch v := val.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int:
		return float64(v), nil
	case int32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case uint:
		return float64(v), nil
	case uint32:
		return float64(v), nil
	case uint64:
		return float64(v), nil
	case string:
		var f float64
		if _, err := fmt.Sscanf(v, "%f", &f); err != nil {
			return 0, fmt.Errorf("cannot parse string '%s' as float: %w", v, err)
		}
		return f, nil
	default:
		return 0, fmt.Errorf("unsupported type %T", val)
	}
}
