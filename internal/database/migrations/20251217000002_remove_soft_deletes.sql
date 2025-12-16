-- +goose Up
-- +goose StatementBegin

-- 1. Remove deleted_at from credential_profiles
ALTER TABLE credential_profiles DROP COLUMN IF EXISTS deleted_at;

-- 2. Remove deleted_at from discovery_profiles
ALTER TABLE discovery_profiles DROP COLUMN IF EXISTS deleted_at;

-- 3. Remove deleted_at from monitors
ALTER TABLE monitors DROP COLUMN IF EXISTS deleted_at;

-- Note: Indexes on deleted_at are automatically dropped when the column is dropped.

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin

-- 1. Add deleted_at back to credential_profiles
ALTER TABLE credential_profiles ADD COLUMN deleted_at TIMESTAMPTZ DEFAULT NULL;
CREATE INDEX IF NOT EXISTS idx_credential_profiles_deleted_at ON credential_profiles(deleted_at) WHERE deleted_at IS NULL;

-- 2. Add deleted_at back to discovery_profiles
ALTER TABLE discovery_profiles ADD COLUMN deleted_at TIMESTAMPTZ DEFAULT NULL;
CREATE INDEX IF NOT EXISTS idx_discovery_profiles_deleted_at ON discovery_profiles(deleted_at) WHERE deleted_at IS NULL;

-- 3. Add deleted_at back to monitors
ALTER TABLE monitors ADD COLUMN deleted_at TIMESTAMPTZ DEFAULT NULL;
CREATE INDEX IF NOT EXISTS idx_monitors_deleted_at ON monitors(deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_monitors_active ON monitors(id) WHERE status = 'active' AND deleted_at IS NULL;

-- +goose StatementEnd
