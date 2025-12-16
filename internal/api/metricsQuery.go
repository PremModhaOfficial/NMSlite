package api

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nmslite/nmslite/internal/database/dbgen"
)

// Constants for query limits and validation
const (
	DefaultLimit = 100
	MaxLimit     = 1000
	QueryTimeout = 30 * time.Second
)

// MetricsQueryRequest represents the POST body for querying metrics
type MetricsQueryRequest struct {
	DeviceIDs []uuid.UUID `json:"device_ids"`
	Prefix    string      `json:"prefix,omitempty"` // e.g., "system" → queries "system.%"
	Start     time.Time   `json:"start"`
	End       time.Time   `json:"end"`
	Limit     int         `json:"limit,omitempty"`
	Latest    bool        `json:"latest,omitempty"`
}

// MetricsQueryResponse represents the API response with nested metrics
type MetricsQueryResponse struct {
	Data  map[string]map[string]float64 `json:"data"`  // {device_id: {name: value}}
	Count int                           `json:"count"` // total metric values
	Query struct {
		DeviceIDs []string  `json:"device_ids"`
		Prefix    string    `json:"prefix,omitempty"`
		Start     time.Time `json:"start"`
		End       time.Time `json:"end"`
		Limit     int       `json:"limit"`
		Latest    bool      `json:"latest"`
	} `json:"query"`
}

// BatchMetricsQueryResponse is an alias for backward compatibility
type BatchMetricsQueryResponse = MetricsQueryResponse

// MetricRow is kept for backward compatibility but uses key-value format
type MetricRow struct {
	Timestamp time.Time `json:"timestamp"`
	DeviceID  uuid.UUID `json:"device_id"`
	Name      string    `json:"name"`
	Value     float64   `json:"value"`
	Type      string    `json:"type,omitempty"`
}

// validateRequest validates the metrics query request
func validateRequest(req *MetricsQueryRequest) error {
	if len(req.DeviceIDs) == 0 {
		return fmt.Errorf("at least one device_id is required")
	}
	if req.Start.IsZero() || req.End.IsZero() {
		return fmt.Errorf("start and end times are required")
	}
	if req.End.Before(req.Start) {
		return fmt.Errorf("end time must be after start time")
	}
	if req.Limit == 0 {
		req.Limit = DefaultLimit
	}
	if req.Limit > MaxLimit {
		return fmt.Errorf("limit cannot exceed %d", MaxLimit)
	}
	return nil
}

// buildPrefixPattern converts a prefix to a SQL LIKE pattern
// Empty prefix returns "%" (match all), "system" returns "system.%"
func buildPrefixPattern(prefix string) string {
	if prefix == "" {
		return "%" // Match all metrics
	}
	return prefix + ".%"
}

// ExecuteMetricsQuery executes the metrics query using sqlc generated functions
func ExecuteMetricsQuery(ctx context.Context, q dbgen.Querier, deviceIDs []uuid.UUID, req MetricsQueryRequest) (*MetricsQueryResponse, error) {
	if err := validateRequest(&req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}

	// Create context with timeout
	queryCtx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	// Initialize response with empty maps for ALL requested devices
	groupedData := make(map[string]map[string]float64)
	for _, id := range deviceIDs {
		groupedData[id.String()] = make(map[string]float64)
	}

	// Build prefix pattern for LIKE query
	prefixPattern := buildPrefixPattern(req.Prefix)

	var dbRows []dbgen.Metric
	var err error

	if req.Latest {
		// Query latest metrics per device per name
		dbRows, err = q.GetLatestMetricsByDeviceAndPrefix(queryCtx, dbgen.GetLatestMetricsByDeviceAndPrefixParams{
			Column1:     deviceIDs,
			Name:        prefixPattern,
			Timestamp:   pgtype.Timestamptz{Time: req.Start, Valid: true},
			Timestamp_2: pgtype.Timestamptz{Time: req.End, Valid: true},
		})
	} else {
		// Query all metrics within range
		dbRows, err = q.GetMetricsByDeviceAndPrefix(queryCtx, dbgen.GetMetricsByDeviceAndPrefixParams{
			Column1:     deviceIDs,
			Name:        prefixPattern,
			Timestamp:   pgtype.Timestamptz{Time: req.Start, Valid: true},
			Timestamp_2: pgtype.Timestamptz{Time: req.End, Valid: true},
			Limit:       int32(req.Limit),
		})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query metrics: %w", err)
	}

	// Populate grouped data - for non-latest, we keep the first (most recent) value
	totalCount := 0
	for _, row := range dbRows {
		deviceKey := row.DeviceID.String()
		if _, exists := groupedData[deviceKey]; !exists {
			groupedData[deviceKey] = make(map[string]float64)
		}
		// For latest=false, only store first occurrence (most recent due to ORDER BY timestamp DESC)
		if _, exists := groupedData[deviceKey][row.Name]; !exists {
			groupedData[deviceKey][row.Name] = row.Value
			totalCount++
		}
	}

	// Build device ID strings for response
	deviceIDStrings := make([]string, len(deviceIDs))
	for i, id := range deviceIDs {
		deviceIDStrings[i] = id.String()
	}

	response := &MetricsQueryResponse{
		Data:  groupedData,
		Count: totalCount,
	}
	response.Query.DeviceIDs = deviceIDStrings
	response.Query.Prefix = req.Prefix
	response.Query.Start = req.Start
	response.Query.End = req.End
	response.Query.Limit = req.Limit
	response.Query.Latest = req.Latest

	return response, nil
}
