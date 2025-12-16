-- name: GetMetricsByDeviceAndPrefix :many
-- Query metrics for devices with prefix matching (SNMP subtree style)
SELECT timestamp, device_id, name, value, type
FROM metrics
WHERE device_id = ANY($1::uuid[])
  AND name LIKE $2  -- e.g., 'system.%' for subtree
  AND timestamp >= $3
  AND timestamp <= $4
ORDER BY timestamp DESC
LIMIT $5;

-- name: GetLatestMetricsByDeviceAndPrefix :many
-- Query the latest value for each metric (per device) with prefix matching
SELECT DISTINCT ON (device_id, name)
       timestamp, device_id, name, value, type
FROM metrics
WHERE device_id = ANY($1::uuid[])
  AND name LIKE $2
  AND timestamp >= $3
  AND timestamp <= $4
ORDER BY device_id, name, timestamp DESC;

-- name: GetAllMetricNames :many
-- Get all unique metric names (for discovery/autocomplete)
SELECT DISTINCT name
FROM metrics
WHERE device_id = ANY($1::uuid[])
ORDER BY name;
