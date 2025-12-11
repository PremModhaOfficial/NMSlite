package api

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Constants for query limits and validation
const (
	DefaultLimit    = 100
	MaxLimit        = 1000
	MaxTagFilters   = 10
	QueryTimeout    = 30 * time.Second
	MaxTagKeyLength = 64
)

// MetricsQueryRequest represents the POST body for querying metrics
type MetricsQueryRequest struct {
	DeviceIDs    []uuid.UUID `json:"device_ids"`
	Start        time.Time   `json:"start"`
	End          time.Time   `json:"end"`
	MetricGroups []string    `json:"metric_groups,omitempty"`
	TagFilters   []TagFilter `json:"tag_filters,omitempty"`
	Limit        int         `json:"limit,omitempty"`
	Latest       bool        `json:"latest,omitempty"`
}

// TagFilter represents a filter on JSONB tag fields
type TagFilter struct {
	Key    string   `json:"key"`
	Op     string   `json:"op"`     // eq, in, like, exists, gt, lt, gte, lte
	Values []string `json:"values"` // values for eq, in, like, gt, lt, gte, lte
}

// MetricRow represents a single metric result row
type MetricRow struct {
	Timestamp   time.Time              `json:"timestamp"`
	MetricGroup string                 `json:"metric_group"`
	DeviceID    uuid.UUID              `json:"device_id"`
	Tags        map[string]interface{} `json:"tags"`
	ValUsed     *float64               `json:"val_used,omitempty"`
	ValTotal    *float64               `json:"val_total,omitempty"`
	ExtraData   map[string]interface{} `json:"extra_data,omitempty"`
}

// MetricsQueryResponse represents the response structure
type MetricsQueryResponse struct {
	Data  []MetricRow `json:"data"`
	Count int         `json:"count"`
	Query struct {
		Start        time.Time `json:"start"`
		End          time.Time `json:"end"`
		MetricGroups []string  `json:"metric_groups,omitempty"`
		Limit        int       `json:"limit"`
		Latest       bool      `json:"latest"`
	} `json:"query"`
}

// BatchMetricsQueryResponse represents grouped metrics response by device
type BatchMetricsQueryResponse struct {
	Data  map[string][]MetricRow `json:"data"`  // key: device_id string
	Count int                    `json:"count"` // total metrics across all devices
	Query struct {
		DeviceIDs    []string  `json:"device_ids"`
		Start        time.Time `json:"start"`
		End          time.Time `json:"end"`
		MetricGroups []string  `json:"metric_groups,omitempty"`
		Limit        int       `json:"limit"`
		Latest       bool      `json:"latest"`
	} `json:"query"`
}

// MetricsQueryBuilder constructs SQL queries for metrics
type MetricsQueryBuilder struct {
	deviceIDs []uuid.UUID
	request   MetricsQueryRequest
}

// NewMetricsQueryBuilder creates a new query builder
func NewMetricsQueryBuilder(deviceIDs []uuid.UUID, req MetricsQueryRequest) *MetricsQueryBuilder {
	return &MetricsQueryBuilder{
		deviceIDs: deviceIDs,
		request:   req,
	}
}

// validate checks if the query request is valid
func (b *MetricsQueryBuilder) validate() error {
	// Check device IDs
	if len(b.deviceIDs) == 0 {
		return fmt.Errorf("at least one device_id is required")
	}

	// Check time range
	if b.request.Start.IsZero() || b.request.End.IsZero() {
		return fmt.Errorf("start and end times are required")
	}
	if b.request.End.Before(b.request.Start) {
		return fmt.Errorf("end time must be after start time")
	}

	// Check limit
	if b.request.Limit == 0 {
		b.request.Limit = DefaultLimit
	}
	if b.request.Limit > MaxLimit {
		return fmt.Errorf("limit cannot exceed %d", MaxLimit)
	}

	// Check tag filters count
	if len(b.request.TagFilters) > MaxTagFilters {
		return fmt.Errorf("cannot have more than %d tag filters", MaxTagFilters)
	}

	// Validate each tag filter
	for i, filter := range b.request.TagFilters {
		if !isValidTagKey(filter.Key) {
			return fmt.Errorf("invalid tag key at filter %d: must be alphanumeric with underscore/dash/dot, max %d chars", i, MaxTagKeyLength)
		}

		switch filter.Op {
		case "eq":
			if len(filter.Values) != 1 {
				return fmt.Errorf("eq operator requires exactly one value at filter %d", i)
			}
		case "in":
			if len(filter.Values) == 0 {
				return fmt.Errorf("in operator requires at least one value at filter %d", i)
			}
		case "like":
			if len(filter.Values) != 1 {
				return fmt.Errorf("like operator requires exactly one value at filter %d", i)
			}
		case "exists":
			// No values needed for exists
		case "gt", "lt", "gte", "lte":
			if len(filter.Values) != 1 {
				return fmt.Errorf("%s operator requires exactly one value at filter %d", filter.Op, i)
			}
		default:
			return fmt.Errorf("unsupported operator '%s' at filter %d", filter.Op, i)
		}
	}

	return nil
}

// isValidTagKey checks if a tag key is valid
func isValidTagKey(key string) bool {
	if key == "" || len(key) > MaxTagKeyLength {
		return false
	}
	// Allow alphanumeric, underscore, dash, and dot
	match, _ := regexp.MatchString(`^[a-zA-Z0-9_.-]+$`, key)
	return match
}

// Build constructs the SQL query and returns the query string and arguments
func (b *MetricsQueryBuilder) Build() (string, []interface{}, error) {
	if err := b.validate(); err != nil {
		return "", nil, err
	}

	var queryParts []string
	var args []interface{}
	paramCount := 0

	// Base SELECT
	if b.request.Latest {
		queryParts = append(queryParts, "SELECT DISTINCT ON (device_id, metric_group) timestamp, metric_group, device_id, tags, val_used, val_total, extra_data")
	} else {
		queryParts = append(queryParts, "SELECT timestamp, metric_group, device_id, tags, val_used, val_total, extra_data")
	}

	queryParts = append(queryParts, "FROM metrics")

	// WHERE clause
	var conditions []string

	// Device ID filter
	paramCount++
	conditions = append(conditions, fmt.Sprintf("device_id = ANY($%d)", paramCount))
	args = append(args, b.deviceIDs)

	// Timestamp range filter
	paramCount++
	conditions = append(conditions, fmt.Sprintf("timestamp >= $%d", paramCount))
	args = append(args, b.request.Start)

	paramCount++
	conditions = append(conditions, fmt.Sprintf("timestamp <= $%d", paramCount))
	args = append(args, b.request.End)

	// Metric groups filter
	if len(b.request.MetricGroups) > 0 {
		paramCount++
		conditions = append(conditions, fmt.Sprintf("metric_group = ANY($%d)", paramCount))
		args = append(args, b.request.MetricGroups)
	}

	// Tag filters
	for _, filter := range b.request.TagFilters {
		var condition string
		switch filter.Op {
		case "eq":
			paramCount++
			condition = fmt.Sprintf("tags->>'%s' = $%d", filter.Key, paramCount)
			args = append(args, filter.Values[0])

		case "in":
			paramCount++
			condition = fmt.Sprintf("tags->>'%s' = ANY($%d)", filter.Key, paramCount)
			args = append(args, filter.Values)

		case "like":
			paramCount++
			condition = fmt.Sprintf("tags->>'%s' LIKE $%d", filter.Key, paramCount)
			args = append(args, filter.Values[0])

		case "exists":
			condition = fmt.Sprintf("tags ? '%s'", filter.Key)

		case "gt":
			paramCount++
			condition = fmt.Sprintf("(tags->>'%s')::numeric > $%d", filter.Key, paramCount)
			args = append(args, filter.Values[0])

		case "lt":
			paramCount++
			condition = fmt.Sprintf("(tags->>'%s')::numeric < $%d", filter.Key, paramCount)
			args = append(args, filter.Values[0])

		case "gte":
			paramCount++
			condition = fmt.Sprintf("(tags->>'%s')::numeric >= $%d", filter.Key, paramCount)
			args = append(args, filter.Values[0])

		case "lte":
			paramCount++
			condition = fmt.Sprintf("(tags->>'%s')::numeric <= $%d", filter.Key, paramCount)
			args = append(args, filter.Values[0])
		}

		if condition != "" {
			conditions = append(conditions, condition)
		}
	}

	queryParts = append(queryParts, "WHERE "+strings.Join(conditions, " AND "))

	// ORDER BY
	if b.request.Latest {
		queryParts = append(queryParts, "ORDER BY device_id, metric_group, timestamp DESC")
	} else {
		queryParts = append(queryParts, "ORDER BY timestamp DESC")
	}

	// LIMIT
	paramCount++
	queryParts = append(queryParts, fmt.Sprintf("LIMIT $%d", paramCount))
	args = append(args, b.request.Limit)

	query := strings.Join(queryParts, " ")
	return query, args, nil
}

// ExecuteQuery executes the metrics query and returns the results
func ExecuteMetricsQuery(ctx context.Context, pool *pgxpool.Pool, deviceIDs []uuid.UUID, req MetricsQueryRequest) (*BatchMetricsQueryResponse, error) {
	builder := NewMetricsQueryBuilder(deviceIDs, req)
	query, args, err := builder.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build query: %w", err)
	}

	// Create context with timeout
	queryCtx, cancel := context.WithTimeout(ctx, QueryTimeout)
	defer cancel()

	// Execute query
	rows, err := pool.Query(queryCtx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	// Parse results
	var results []MetricRow
	for rows.Next() {
		var row MetricRow
		var tagsJSON []byte
		var extraDataJSON []byte

		err := rows.Scan(
			&row.Timestamp,
			&row.MetricGroup,
			&row.DeviceID,
			&tagsJSON,
			&row.ValUsed,
			&row.ValTotal,
			&extraDataJSON,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Unmarshal JSONB fields
		if len(tagsJSON) > 0 {
			if err := json.Unmarshal(tagsJSON, &row.Tags); err != nil {
				return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
			}
		}

		if len(extraDataJSON) > 0 {
			if err := json.Unmarshal(extraDataJSON, &row.ExtraData); err != nil {
				return nil, fmt.Errorf("failed to unmarshal extra_data: %w", err)
			}
		}

		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	// Initialize map with empty slices for ALL requested devices
	groupedData := make(map[string][]MetricRow)
	for _, id := range deviceIDs {
		groupedData[id.String()] = []MetricRow{}
	}

	// Populate with actual results
	for _, row := range results {
		key := row.DeviceID.String()
		groupedData[key] = append(groupedData[key], row)
	}

	// Build device ID strings for response
	deviceIDStrings := make([]string, len(deviceIDs))
	for i, id := range deviceIDs {
		deviceIDStrings[i] = id.String()
	}

	response := &BatchMetricsQueryResponse{
		Data:  groupedData,
		Count: len(results),
	}
	response.Query.DeviceIDs = deviceIDStrings
	response.Query.Start = req.Start
	response.Query.End = req.End
	response.Query.MetricGroups = req.MetricGroups
	response.Query.Limit = req.Limit
	response.Query.Latest = req.Latest

	return response, nil
}
