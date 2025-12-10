-- name: CreateCredentialProfile :one
INSERT INTO credential_profiles (
    name,
    description,
    protocol,
    credential_data
) VALUES (
    $1, $2, $3, $4
)
RETURNING *;

-- name: GetCredentialProfile :one
SELECT * FROM credential_profiles
WHERE id = $1 AND deleted_at IS NULL LIMIT 1;

-- name: ListCredentialProfiles :many
SELECT * FROM credential_profiles
WHERE deleted_at IS NULL
ORDER BY name;

-- name: UpdateCredentialProfile :one
UPDATE credential_profiles
SET 
    name = $2,
    description = $3,
    protocol = $4,
    credential_data = $5,
    updated_at = NOW()
WHERE id = $1 AND deleted_at IS NULL
RETURNING *;

-- name: DeleteCredentialProfile :exec
UPDATE credential_profiles
SET deleted_at = NOW()
WHERE id = $1;
