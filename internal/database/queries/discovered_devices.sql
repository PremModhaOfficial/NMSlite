-- name: CreateDiscoveredDevice :one
INSERT INTO discovered_devices (
    discovery_profile_id, ip_address, port, status
) VALUES (
    $1, $2, $3, $4
)
RETURNING *;

-- name: ListDiscoveredDevices :many
SELECT * FROM discovered_devices
WHERE discovery_profile_id = $1
ORDER BY created_at DESC;

-- name: UpdateDiscoveredDeviceStatus :exec
UPDATE discovered_devices
SET 
    status = $2,
    updated_at = NOW()
WHERE id = $1;

-- name: GetDiscoveredDevice :one
SELECT * FROM discovered_devices
WHERE id = $1;

-- name: ClearDiscoveredDevices :exec
DELETE FROM discovered_devices
WHERE discovery_profile_id = $1;
