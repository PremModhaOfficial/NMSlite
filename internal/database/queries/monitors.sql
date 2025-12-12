-- name: ListMonitors :many
SELECT * FROM monitors
WHERE deleted_at IS NULL
ORDER BY created_at DESC;

-- name: CreateMonitor :one
INSERT INTO monitors (
    display_name,
    hostname,
    ip_address,
    plugin_id,
    credential_profile_id,
    discovery_profile_id,
    port,
    polling_interval_seconds,
    status
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, COALESCE($8, 60), COALESCE($9, 'active')
)
RETURNING *;

-- name: GetMonitor :one
SELECT * FROM monitors
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdateMonitor :one
UPDATE monitors
SET 
    display_name = $2,
    hostname = $3,
    ip_address = $4,
    plugin_id = $5,
    credential_profile_id = $6,
    polling_interval_seconds = $7,
    port = $8,
    status = $9,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: DeleteMonitor :exec
UPDATE monitors
SET deleted_at = NOW()
WHERE id = $1;

-- name: ListActiveMonitorsWithCredentials :many
-- Loads active monitors with their credential data in a single query.
-- Used by scheduler to initialize cache at startup.
SELECT 
    m.id, m.display_name, m.hostname, m.ip_address, m.plugin_id, 
    m.credential_profile_id, m.discovery_profile_id, m.port, 
    m.polling_interval_seconds, m.status, m.created_at, m.updated_at, m.deleted_at,
    c.credential_data
FROM monitors m
JOIN credential_profiles c ON m.credential_profile_id = c.id
WHERE m.status = 'active' AND m.deleted_at IS NULL;

-- name: UpdateMonitorStatus :exec
-- Updates monitor status (active/down) and updated_at timestamp.
UPDATE monitors
SET status = $2, updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;

-- name: GetExistingMonitorIDs :many
-- Returns only monitor IDs that exist and are not soft-deleted.
-- Used to validate a batch of IDs before metrics queries.
SELECT id FROM monitors WHERE id = ANY($1::uuid[]) AND deleted_at IS NULL;