-- name: GetMetricsByDeviceAndPrefix :many
-- Query metrics for devices with prefix matching (SNMP subtree style)
SELECT timestamp, device_id, name, value, type
FROM metrics
WHERE device_id = ANY(sqlc.arg(device_ids)::uuid[])
  AND name LIKE sqlc.arg(metric_name_pattern)  -- e.g., 'system.%' for subtree
  AND timestamp >= sqlc.arg(start_time)
  AND timestamp <= sqlc.arg(end_time)
ORDER BY timestamp DESC
LIMIT sqlc.arg(limit_count);

-- name: GetLatestMetricsByDeviceAndPrefix :many
-- Query the latest value for each metric (per device) with prefix matching
SELECT DISTINCT ON (device_id, name)
       timestamp, device_id, name, value, type
FROM metrics
WHERE device_id = ANY(sqlc.arg(device_ids)::uuid[])
  AND name LIKE sqlc.arg(metric_name_pattern)
  AND timestamp >= sqlc.arg(start_time)
  AND timestamp <= sqlc.arg(end_time)
ORDER BY device_id, name, timestamp DESC;

-- name: GetAllMetricNames :many
-- Get all unique metric names (for discovery/autocomplete)
SELECT DISTINCT name
FROM metrics
WHERE device_id = ANY(sqlc.arg(device_ids)::uuid[])
ORDER BY name;
