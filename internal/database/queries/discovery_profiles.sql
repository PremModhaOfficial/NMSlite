-- name: ListDiscoveryProfiles :many
SELECT * FROM discovery_profiles
WHERE deleted_at IS NULL
ORDER BY created_at DESC;

-- name: CreateDiscoveryProfile :one
INSERT INTO discovery_profiles (
    name, target_value, ports, port_scan_timeout_ms, credential_profile_ids, auto_provision
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: GetDiscoveryProfile :one
SELECT * FROM discovery_profiles
WHERE id = $1 AND deleted_at IS NULL;

-- name: UpdateDiscoveryProfile :one
UPDATE discovery_profiles
SET 
    name = $2,
    target_value = $3,
    ports = $4,
    port_scan_timeout_ms = $5,
    credential_profile_ids = $6,
    auto_provision = $7,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: DeleteDiscoveryProfile :exec
UPDATE discovery_profiles
SET deleted_at = NOW()
WHERE id = $1;

-- name: UpdateDiscoveryProfileStatus :exec
UPDATE discovery_profiles
SET 
    last_run_at = NOW(),
    last_run_status = $2,
    devices_discovered = $3,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL;
