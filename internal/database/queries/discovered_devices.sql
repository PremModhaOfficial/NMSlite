-- name: CreateDiscoveredDevice :one
INSERT INTO discovered_devices (
    discovery_profile_id, ip_address, hostname, port, status
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING *;

-- name: ListDiscoveredDevices :many
SELECT * FROM discovered_devices
WHERE discovery_profile_id = $1
ORDER BY created_at DESC;

-- name: ClearDiscoveredDevices :exec
DELETE FROM discovered_devices
WHERE discovery_profile_id = $1;
