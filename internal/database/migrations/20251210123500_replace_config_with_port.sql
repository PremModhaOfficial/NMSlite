-- +goose Up
-- +goose StatementBegin
ALTER TABLE monitors DROP COLUMN config;
ALTER TABLE monitors ADD COLUMN port INT DEFAULT 0;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE monitors DROP COLUMN port;
ALTER TABLE monitors ADD COLUMN config JSONB NOT NULL DEFAULT '{}';
-- +goose StatementEnd
