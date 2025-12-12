-- +goose Up
ALTER TABLE discovered_devices DROP COLUMN hostname;
ALTER TABLE metrics DROP COLUMN extra_data;

-- +goose Down
ALTER TABLE discovered_devices ADD COLUMN hostname VARCHAR(255);
ALTER TABLE metrics ADD COLUMN extra_data JSONB;
