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
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: DeleteMonitor :exec
UPDATE monitors
SET deleted_at = NOW()
WHERE id = $1;