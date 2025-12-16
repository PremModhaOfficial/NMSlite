-- +goose Up
-- +goose StatementBegin
SELECT 'up SQL query - add auto_run to discovery_profiles';

ALTER TABLE discovery_profiles
ADD COLUMN auto_run BOOLEAN DEFAULT false;

COMMENT ON COLUMN discovery_profiles.auto_run IS 'If true, discovery will run automatically on a schedule';

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 'down SQL query - remove auto_run from discovery_profiles';

ALTER TABLE discovery_profiles
DROP COLUMN auto_run;

-- +goose StatementEnd
