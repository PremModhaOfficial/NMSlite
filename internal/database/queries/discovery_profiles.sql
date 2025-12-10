-- name: ListDiscoveryProfiles :many
SELECT * FROM discovery_profiles
WHERE deleted_at IS NULL
ORDER BY created_at DESC;

-- name: CreateDiscoveryProfile :one
INSERT INTO discovery_profiles (
    name, target_type, target_value, ports, port_scan_timeout_ms, credential_profile_ids
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
    target_type = $3,
    target_value = $4,
    ports = $5,
    port_scan_timeout_ms = $6,
    credential_profile_ids = $7,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: DeleteDiscoveryProfile :exec
UPDATE discovery_profiles
SET deleted_at = NOW()
WHERE id = $1;
