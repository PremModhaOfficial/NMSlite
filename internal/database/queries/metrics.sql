-- name: GetMetricsByDeviceAndPrefix :many
-- Query metrics for devices with per-metric limiting using LATERAL JOIN
-- Returns top N rows per (device_id, metric_name) group ordered by timestamp DESC
SELECT m.timestamp, m.device_id, m.name, m.value, m.type
FROM (
  SELECT DISTINCT metrics.device_id, metrics.name
  FROM metrics
  WHERE metrics.device_id = ANY(sqlc.arg(device_ids)::bigint[])
    AND metrics.name LIKE sqlc.arg(metric_name_pattern)
    AND metrics.timestamp >= sqlc.arg(start_time)
    AND metrics.timestamp <= sqlc.arg(end_time)
) groups
CROSS JOIN LATERAL (
  SELECT metrics.timestamp, metrics.device_id, metrics.name, metrics.value, metrics.type
  FROM metrics
  WHERE metrics.device_id = groups.device_id
    AND metrics.name = groups.name
    AND metrics.timestamp >= sqlc.arg(start_time)
    AND metrics.timestamp <= sqlc.arg(end_time)
  ORDER BY metrics.timestamp DESC
  LIMIT sqlc.arg(limit_count)
) m
ORDER BY m.device_id, m.name, m.timestamp DESC;

-- name: GetLatestMetricsByDeviceAndPrefix :many
-- Query the latest value for each metric (per device) with prefix matching
SELECT DISTINCT ON (device_id, name)
       timestamp, device_id, name, value, type
FROM metrics
WHERE device_id = ANY(sqlc.arg(device_ids)::bigint[])
  AND name LIKE sqlc.arg(metric_name_pattern)
  AND timestamp >= sqlc.arg(start_time)
  AND timestamp <= sqlc.arg(end_time)
ORDER BY device_id, name, timestamp DESC;

-- name: GetAllMetricNames :many
-- Get all unique metric names (for discovery/autocomplete)
SELECT DISTINCT name
FROM metrics
WHERE device_id = ANY(sqlc.arg(device_ids)::bigint[])
ORDER BY name;
