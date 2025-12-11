-- +goose Up
-- +goose StatementBegin
ALTER TABLE discovery_profiles 
ADD COLUMN auto_provision BOOLEAN DEFAULT false;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE discovery_profiles 
DROP COLUMN auto_provision;
-- +goose StatementEnd
