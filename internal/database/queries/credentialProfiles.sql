-- name: CreateCredentialProfile :one
INSERT INTO credential_profiles (
    name,
    description,
    protocol,
    payload
) VALUES (
    $1, $2, $3, $4
)
RETURNING *;

-- name: GetCredentialProfile :one
SELECT * FROM credential_profiles
WHERE id = $1 LIMIT 1;

-- name: ListCredentialProfiles :many
SELECT * FROM credential_profiles
ORDER BY name;

-- name: UpdateCredentialProfile :one
UPDATE credential_profiles
SET 
    name = $2,
    description = $3,
    protocol = $4,
    payload = $5,
    updated_at = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteCredentialProfile :exec
DELETE FROM credential_profiles
WHERE id = $1;
