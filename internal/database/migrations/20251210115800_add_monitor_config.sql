-- +goose Up
-- +goose StatementBegin
ALTER TABLE monitors ADD COLUMN config JSONB NOT NULL DEFAULT '{}';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE monitors DROP COLUMN config;
-- +goose StatementEnd
