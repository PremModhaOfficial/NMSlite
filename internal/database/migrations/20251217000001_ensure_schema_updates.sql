-- +goose Up
-- +goose StatementBegin

-- This migration is a "fixer" to ensure existing databases are brought up to the latest schema
-- correctly, particularly if the initial '20251217000000' migration was marked as applied
-- but didn't execute the transitional logic.

DO $$
BEGIN
    -- 1. credential_profiles: Rename credential_data to payload
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'credential_profiles' AND column_name = 'credential_data') THEN
        ALTER TABLE credential_profiles RENAME COLUMN credential_data TO payload;
    END IF;

    -- 2. monitors: Add port if missing
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'monitors') THEN
        IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'monitors' AND column_name = 'port') THEN
            ALTER TABLE monitors ADD COLUMN port INT DEFAULT 0;
        END IF;
        -- Remove config if exists (replaced by port/logic)
        IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'monitors' AND column_name = 'config') THEN
            ALTER TABLE monitors DROP COLUMN config;
        END IF;
    END IF;

    -- 3. discovery_profiles: Cleanup and Additions
    IF EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'discovery_profiles') THEN
        -- Remove orphaned target_type
        IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'discovery_profiles' AND column_name = 'target_type') THEN
            ALTER TABLE discovery_profiles DROP COLUMN target_type;
        END IF;
        -- Add auto_provision
        IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'discovery_profiles' AND column_name = 'auto_provision') THEN
            ALTER TABLE discovery_profiles ADD COLUMN auto_provision BOOLEAN DEFAULT false;
        END IF;
        -- Add auto_run
        IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'discovery_profiles' AND column_name = 'auto_run') THEN
            ALTER TABLE discovery_profiles ADD COLUMN auto_run BOOLEAN DEFAULT false;
        END IF;
    END IF;

    -- 4. metrics: structural refactor
    IF EXISTS (SELECT 1 FROM information_schema.columns WHERE table_name = 'metrics' AND column_name = 'metric_group') THEN
        DROP TABLE metrics CASCADE;
    END IF;
END $$;

-- Re-run CREATE TABLE statements with IF NOT EXISTS to ensure tables are created
-- if they were dropped or missing (e.g. metrics).

CREATE TABLE IF NOT EXISTS metrics (
    timestamp TIMESTAMPTZ NOT NULL,
    device_id UUID NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    value DOUBLE PRECISION NOT NULL,
    type VARCHAR(20) DEFAULT 'gauge'
);

-- Ensure indexes and hypertable setup for metrics if it was just recreated
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM timescaledb_information.hypertables WHERE hypertable_name = 'metrics') THEN
        PERFORM create_hypertable('metrics', 'timestamp', chunk_time_interval => INTERVAL '1 day');
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_metrics_device_time ON metrics(device_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_metrics_name_prefix ON metrics(name text_pattern_ops);
CREATE INDEX IF NOT EXISTS idx_metrics_device_name_time ON metrics(device_id, name, timestamp DESC);

-- Policies (safe to re-run, will fail gracefully or be skipped if exist)
DO $$
BEGIN
    -- Compression
    BEGIN
        ALTER TABLE metrics SET (
            timescaledb.compress,
            timescaledb.compress_segmentby = 'device_id',
            timescaledb.compress_orderby = 'timestamp DESC'
        );
        PERFORM add_compression_policy('metrics', INTERVAL '1 hour');
    EXCEPTION WHEN OTHERS THEN NULL; END;

    -- Retention
    BEGIN
        PERFORM add_retention_policy('metrics', INTERVAL '90 days');
    EXCEPTION WHEN OTHERS THEN NULL; END;
END $$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- No-op for down as this is just ensuring consistency
SELECT 'down SQL query';
-- +goose StatementEnd
