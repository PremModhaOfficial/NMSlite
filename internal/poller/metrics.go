package poller

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Metric represents a single metric measurement matching the DB schema
type Metric struct {
	Timestamp   time.Time              `json:"timestamp"`
	MetricGroup string                 `json:"metric_group"`
	DeviceID    string                 `json:"device_id"`
	Tags        map[string]string      `json:"tags"`
	ValUsed     *float64               `json:"val_used,omitempty"`
	ValTotal    *float64               `json:"val_total,omitempty"`
	ExtraData   map[string]interface{} `json:"extra_data,omitempty"`
}

// MetricBatch represents a batch of metrics for a monitor
type MetricBatch struct {
	MonitorID uuid.UUID `json:"monitor_id"`
	Timestamp time.Time `json:"timestamp"`
	Metrics   []Metric  `json:"metrics"`
}

// ParseMetricsFromPlugin converts plugin output to typed metrics
// raw is expected to be an array of metric objects from the plugin
// The monitorID is automatically set as device_id if not provided by the plugin
func ParseMetricsFromPlugin(monitorID uuid.UUID, timestamp time.Time, raw []interface{}) ([]Metric, error) {
	if raw == nil {
		return nil, fmt.Errorf("raw metrics data is nil")
	}

	if len(raw) == 0 {
		return []Metric{}, nil
	}

	metrics := make([]Metric, 0, len(raw))

	for i, item := range raw {
		// Convert the interface{} to a map
		metricMap, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("metric at index %d is not a valid object: %T", i, item)
		}

		// Auto-inject device_id from monitorID if not provided by plugin
		if _, hasDeviceID := metricMap["device_id"]; !hasDeviceID {
			metricMap["device_id"] = monitorID.String()
		}

		// Parse the metric from the map
		metric, err := parseMetricFromMap(metricMap, timestamp)
		if err != nil {
			return nil, fmt.Errorf("failed to parse metric at index %d: %w", i, err)
		}

		metrics = append(metrics, metric)
	}

	return metrics, nil
}

// parseMetricFromMap converts a map to a Metric struct
func parseMetricFromMap(data map[string]interface{}, defaultTimestamp time.Time) (Metric, error) {
	metric := Metric{
		Timestamp: defaultTimestamp,
		Tags:      make(map[string]string),
		ExtraData: make(map[string]interface{}),
	}

	// Parse metric_group (required)
	if metricGroup, ok := data["metric_group"].(string); ok {
		metric.MetricGroup = metricGroup
	} else {
		return metric, fmt.Errorf("missing or invalid 'metric_group' field")
	}

	// Parse device_id (required)
	if deviceID, ok := data["device_id"].(string); ok {
		metric.DeviceID = deviceID
	} else {
		return metric, fmt.Errorf("missing or invalid 'device_id' field")
	}

	// Parse timestamp (optional, use default if not provided)
	if ts, ok := data["timestamp"].(string); ok {
		parsedTime, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			return metric, fmt.Errorf("invalid timestamp format: %w", err)
		}
		metric.Timestamp = parsedTime
	}

	// Parse tags (optional)
	if tagsRaw, ok := data["tags"]; ok && tagsRaw != nil {
		switch tags := tagsRaw.(type) {
		case map[string]interface{}:
			for k, v := range tags {
				if strVal, ok := v.(string); ok {
					metric.Tags[k] = strVal
				} else {
					metric.Tags[k] = fmt.Sprintf("%v", v)
				}
			}
		case map[string]string:
			metric.Tags = tags
		default:
			return metric, fmt.Errorf("invalid 'tags' field type: %T", tagsRaw)
		}
	}

	// Parse val_used (optional)
	if valUsed, ok := data["val_used"]; ok && valUsed != nil {
		val, err := parseFloat(valUsed)
		if err != nil {
			return metric, fmt.Errorf("invalid 'val_used' value: %w", err)
		}
		metric.ValUsed = &val
	}

	// Parse val_total (optional)
	if valTotal, ok := data["val_total"]; ok && valTotal != nil {
		val, err := parseFloat(valTotal)
		if err != nil {
			return metric, fmt.Errorf("invalid 'val_total' value: %w", err)
		}
		metric.ValTotal = &val
	}

	// Parse extra_data (optional)
	if extraData, ok := data["extra_data"].(map[string]interface{}); ok {
		metric.ExtraData = extraData
	} else if extraDataRaw, ok := data["extra_data"]; ok && extraDataRaw != nil {
		// Try to marshal and unmarshal to ensure proper type
		jsonBytes, err := json.Marshal(extraDataRaw)
		if err != nil {
			return metric, fmt.Errorf("invalid 'extra_data' field: %w", err)
		}
		if err := json.Unmarshal(jsonBytes, &metric.ExtraData); err != nil {
			return metric, fmt.Errorf("failed to parse 'extra_data': %w", err)
		}
	}

	return metric, nil
}

// parseFloat attempts to convert various numeric types to float64
func parseFloat(val interface{}) (float64, error) {
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
