-- +goose Up
-- +goose StatementBegin

CREATE TABLE IF NOT EXISTS credential_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    protocol VARCHAR(50) NOT NULL,
    payload JSONB NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ DEFAULT NULL
);

CREATE INDEX IF NOT EXISTS idx_credential_profiles_protocol ON credential_profiles(protocol);
CREATE INDEX IF NOT EXISTS idx_credential_profiles_deleted_at ON credential_profiles(deleted_at) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS discovery_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    target_value TEXT NOT NULL,
    port INTEGER NOT NULL,
    port_scan_timeout_ms INT DEFAULT 1000,
    credential_profile_id UUID NOT NULL REFERENCES credential_profiles(id),
    last_run_at TIMESTAMPTZ,
    last_run_status VARCHAR(50),
    devices_discovered INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ DEFAULT NULL,
    auto_provision BOOLEAN DEFAULT false,
    auto_run BOOLEAN DEFAULT false
);

CREATE INDEX IF NOT EXISTS idx_discovery_profiles_deleted_at ON discovery_profiles(deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_discovery_profiles_credential ON discovery_profiles(credential_profile_id);
CREATE INDEX IF NOT EXISTS idx_discovery_profiles_port ON discovery_profiles(port);

CREATE TABLE IF NOT EXISTS monitors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    display_name VARCHAR(255),
    hostname VARCHAR(255),
    ip_address INET NOT NULL,
    plugin_id VARCHAR(100) NOT NULL,
    credential_profile_id UUID NOT NULL REFERENCES credential_profiles(id),
    discovery_profile_id UUID NOT NULL REFERENCES discovery_profiles(id),
    polling_interval_seconds INT DEFAULT 60,
    status VARCHAR(50) DEFAULT 'active',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ DEFAULT NULL,
    port INT DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_monitors_ip_address ON monitors(ip_address);
CREATE INDEX IF NOT EXISTS idx_monitors_status ON monitors(status) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_monitors_plugin_id ON monitors(plugin_id);
CREATE INDEX IF NOT EXISTS idx_monitors_deleted_at ON monitors(deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_monitors_active ON monitors(id) WHERE status = 'active' AND deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS discovered_devices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    discovery_profile_id UUID REFERENCES discovery_profiles(id) ON DELETE CASCADE,
    ip_address INET NOT NULL,
    port INT NOT NULL,
    status VARCHAR(50) DEFAULT 'new',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_discovered_devices_profile ON discovered_devices(discovery_profile_id);
CREATE INDEX IF NOT EXISTS idx_discovered_devices_status ON discovered_devices(status);

CREATE TABLE IF NOT EXISTS metrics (
    timestamp TIMESTAMPTZ NOT NULL,
    device_id UUID NOT NULL REFERENCES monitors(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    value DOUBLE PRECISION NOT NULL,
    type VARCHAR(20) DEFAULT 'gauge'
);

-- TimescaleDB setup
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM timescaledb_information.hypertables WHERE hypertable_name = 'metrics') THEN
        PERFORM create_hypertable('metrics', 'timestamp', chunk_time_interval => INTERVAL '1 day');
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_metrics_device_time ON metrics(device_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_metrics_name_prefix ON metrics(name text_pattern_ops);
CREATE INDEX IF NOT EXISTS idx_metrics_device_name_time ON metrics(device_id, name, timestamp DESC);

-- Policies
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
    EXCEPTION WHEN OTHERS THEN
        NULL;
    END;

    -- Retention
    BEGIN
        PERFORM add_retention_policy('metrics', INTERVAL '90 days');
    EXCEPTION WHEN OTHERS THEN
        NULL;
    END;
END $$;

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS metrics CASCADE;
DROP TABLE IF EXISTS discovered_devices CASCADE;
DROP TABLE IF EXISTS monitors CASCADE;
DROP TABLE IF EXISTS discovery_profiles CASCADE;
DROP TABLE IF EXISTS credential_profiles CASCADE;
-- +goose StatementEnd