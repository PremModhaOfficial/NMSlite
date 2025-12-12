-- +goose Up
-- +goose StatementBegin

-- Delete monitors with NULL credential_profile_id or discovery_profile_id
DELETE FROM monitors 
WHERE credential_profile_id IS NULL OR discovery_profile_id IS NULL;

-- Add NOT NULL constraints
ALTER TABLE monitors 
ALTER COLUMN credential_profile_id SET NOT NULL,
ALTER COLUMN discovery_profile_id SET NOT NULL;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- Remove NOT NULL constraints to allow rollback
ALTER TABLE monitors 
ALTER COLUMN credential_profile_id DROP NOT NULL,
ALTER COLUMN discovery_profile_id DROP NOT NULL;

-- +goose StatementEnd
