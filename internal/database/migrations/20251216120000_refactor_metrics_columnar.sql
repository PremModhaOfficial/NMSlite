-- +goose Up
-- +goose StatementBegin
-- Remove existing TimescaleDB policies if they exist
SELECT remove_retention_policy('metrics', if_exists => true);
SELECT remove_compression_policy('metrics', if_exists => true);

-- Drop existing metrics table (hypertable)
DROP TABLE IF EXISTS metrics;

-- Create new key-value metrics table with simplified field names
CREATE TABLE metrics (
    timestamp TIMESTAMPTZ NOT NULL,
    device_id UUID NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,           -- Hierarchical: "system.cpu.usage"
    value DOUBLE PRECISION NOT NULL,
    type VARCHAR(20) DEFAULT 'gauge'      -- 'gauge', 'counter', 'derive'
);

-- Convert to TimescaleDB hypertable (chunk interval: 1 day)
SELECT create_hypertable('metrics', 'timestamp', chunk_time_interval => INTERVAL '1 day');

-- Indexes for query performance
CREATE INDEX idx_metrics_device_time ON metrics(device_id, timestamp DESC);
CREATE INDEX idx_metrics_name_prefix ON metrics(name text_pattern_ops);  -- For prefix LIKE queries
CREATE INDEX idx_metrics_device_name_time ON metrics(device_id, name, timestamp DESC);

-- Compression policy (compress chunks older than 1 hour)
ALTER TABLE metrics SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'device_id',
    timescaledb.compress_orderby = 'timestamp DESC'
);
SELECT add_compression_policy('metrics', INTERVAL '1 hour');

-- Retention policy (90 days)
SELECT add_retention_policy('metrics', INTERVAL '90 days');

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Remove TimescaleDB policies
SELECT remove_retention_policy('metrics', if_exists => true);
SELECT remove_compression_policy('metrics', if_exists => true);

-- Drop the key-value table
DROP TABLE IF EXISTS metrics;

-- Recreate original metrics table structure (the very first tag-based version)
CREATE TABLE metrics (
    timestamp TIMESTAMPTZ NOT NULL,
    metric_group VARCHAR(50) NOT NULL,
    device_id UUID NOT NULL,
    tags JSONB NOT NULL DEFAULT '{}',
    val_used DOUBLE PRECISION,
    val_total DOUBLE PRECISION
);

-- Convert to TimescaleDB hypertable
SELECT create_hypertable('metrics', 'timestamp', chunk_time_interval => INTERVAL '1 day');

-- Recreate original indexes
CREATE INDEX idx_metrics_device_time ON metrics(device_id, timestamp DESC);
CREATE INDEX idx_metrics_group_time ON metrics(metric_group, timestamp DESC);
CREATE INDEX idx_metrics_tags ON metrics USING GIN (tags);

-- Recreate original compression policy
ALTER TABLE metrics SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'device_id, metric_group',
    timescaledb.compress_orderby = 'timestamp DESC'
);
SELECT add_compression_policy('metrics', INTERVAL '1 hour');

-- Recreate retention policy
SELECT add_retention_policy('metrics', INTERVAL '90 days');

-- +goose StatementEnd
