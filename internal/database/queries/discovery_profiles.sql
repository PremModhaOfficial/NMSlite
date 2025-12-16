-- name: ListDiscoveryProfiles :many
SELECT * FROM discovery_profiles
ORDER BY created_at DESC;

-- name: CreateDiscoveryProfile :one
INSERT INTO discovery_profiles (
    name, target_value, port, port_scan_timeout_ms, credential_profile_id, auto_provision, auto_run
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
RETURNING *;

-- name: GetDiscoveryProfile :one
SELECT * FROM discovery_profiles
WHERE id = $1;

-- name: UpdateDiscoveryProfile :one
UPDATE discovery_profiles
SET 
    name = $2,
    target_value = $3,
    port = $4,
    port_scan_timeout_ms = $5,
    credential_profile_id = $6,
    auto_provision = $7,
    auto_run = $8,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteDiscoveryProfile :exec
DELETE FROM discovery_profiles
WHERE id = $1;

-- name: UpdateDiscoveryProfileStatus :exec
UPDATE discovery_profiles
SET 
    last_run_at = NOW(),
    last_run_status = $2,
    devices_discovered = $3,
    updated_at = NOW()
WHERE id = $1;
