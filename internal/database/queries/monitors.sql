-- name: ListMonitors :many
SELECT * FROM monitors
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
    $1, $2, $3, $4, $5, $6, $7, 
    COALESCE(sqlc.narg(polling_interval_seconds)::int, 60), 
    COALESCE(sqlc.narg(status)::text, 'active')
)
RETURNING *;

-- name: GetMonitor :one
SELECT * FROM monitors
WHERE id = $1;

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
WHERE id = $1
RETURNING *;

-- name: DeleteMonitor :exec
DELETE FROM monitors
WHERE id = $1;

-- name: ListActiveMonitorsWithCredentials :many
-- Loads active monitors with their credential data in a single query.
-- Used by scheduler to initialize cache at startup.
SELECT 
    m.id, m.display_name, m.hostname, m.ip_address, m.plugin_id, 
    m.credential_profile_id, m.discovery_profile_id, m.port, 
    m.polling_interval_seconds, m.status, m.created_at, m.updated_at,
    c.payload
FROM monitors m
JOIN credential_profiles c ON m.credential_profile_id = c.id
WHERE m.status = 'active';

-- name: UpdateMonitorStatus :exec
-- Updates monitor status (active/down) and updated_at timestamp.
UPDATE monitors
SET status = $2, updated_at = NOW()
WHERE id = $1;

-- name: GetExistingMonitorIDs :many
-- Returns only monitor IDs that exist and are not soft-deleted.
-- Used to validate a batch of IDs before metrics queries.
SELECT id FROM monitors WHERE id = ANY(sqlc.arg(monitor_ids)::uuid[]);

-- name: GetMonitorWithCredentials :one
-- Fetches a single monitor with its credential data.
-- Used for efficient cache invalidation.
SELECT 
    m.id, m.display_name, m.hostname, m.ip_address, m.plugin_id, 
    m.credential_profile_id, m.discovery_profile_id, m.port, 
    m.polling_interval_seconds, m.status, m.created_at, m.updated_at,
    c.payload
FROM monitors m
JOIN credential_profiles c ON m.credential_profile_id = c.id
WHERE m.id = $1;

-- name: GetMonitorsWithCredentialsByCredentialID :many
-- Fetches all monitors using a specific credential profile, with their credential data.
-- Used for efficient cache invalidation when a credential profile changes.
SELECT 
    m.id, m.display_name, m.hostname, m.ip_address, m.plugin_id, 
    m.credential_profile_id, m.discovery_profile_id, m.port, 
    m.polling_interval_seconds, m.status, m.created_at, m.updated_at,
    c.payload
FROM monitors m
JOIN credential_profiles c ON m.credential_profile_id = c.id
WHERE m.credential_profile_id = $1;

-- name: GetMonitorsByCredentialID :many
SELECT id FROM monitors WHERE credential_profile_id = $1;