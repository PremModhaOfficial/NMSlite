-- +goose Up
-- +goose StatementBegin
SELECT 'up SQL query';

CREATE TABLE credential_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    protocol VARCHAR(50) NOT NULL, -- e.g., 'winrm', 'ssh', 'snmp-v2c'
    credential_data JSONB NOT NULL, -- Encrypted secrets (AES-256-GCM)
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ DEFAULT NULL -- Soft delete
);

CREATE INDEX idx_credential_profiles_protocol ON credential_profiles(protocol);
CREATE INDEX idx_credential_profiles_deleted_at ON credential_profiles(deleted_at) WHERE deleted_at IS NULL;

---
CREATE TABLE discovery_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    target_type VARCHAR(50) NOT NULL, -- 'cidr', 'range', 'ip'
    target_value TEXT NOT NULL,
    ports JSONB NOT NULL, -- e.g. [22, 5985]
    port_scan_timeout_ms INT DEFAULT 1000,
    credential_profile_ids JSONB NOT NULL, -- Array of UUIDs: ["id1", "id2"]
    last_run_at TIMESTAMPTZ,
    last_run_status VARCHAR(50), -- 'success', 'partial', 'failed'
    devices_discovered INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ DEFAULT NULL -- Soft delete
);

CREATE INDEX idx_discovery_profiles_deleted_at ON discovery_profiles(deleted_at) WHERE deleted_at IS NULL;
---
CREATE TABLE monitors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    display_name VARCHAR(255),
    hostname VARCHAR(255),
    ip_address INET NOT NULL,
    -- Configuration
    plugin_id VARCHAR(100) NOT NULL, -- Logic driver (e.g., 'windows-winrm')
    credential_profile_id UUID REFERENCES credential_profiles(id),
    discovery_profile_id UUID REFERENCES discovery_profiles(id) ON DELETE RESTRICT,
    polling_interval_seconds INT DEFAULT 60,
    -- Persisted State (only status - for maintenance mode and cache loading)
    status VARCHAR(50) DEFAULT 'active', -- 'active', 'maintenance', 'down'
    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ DEFAULT NULL -- Soft delete
);
-- Performance indexes
CREATE INDEX idx_monitors_ip_address ON monitors(ip_address);
CREATE INDEX idx_monitors_status ON monitors(status) WHERE deleted_at IS NULL;
CREATE INDEX idx_monitors_plugin_id ON monitors(plugin_id);
CREATE INDEX idx_monitors_deleted_at ON monitors(deleted_at) WHERE deleted_at IS NULL;
-- Index for loading active monitors into cache on startup
CREATE INDEX idx_monitors_active ON monitors(id) WHERE status = 'active' AND deleted_at IS NULL;

---
CREATE TABLE metrics (
    timestamp TIMESTAMPTZ NOT NULL,
    metric_group VARCHAR(50) NOT NULL, -- e.g., 'host.cpu', 'host.memory', 'net.interface'
    device_id UUID NOT NULL,
    tags JSONB NOT NULL DEFAULT '{}', -- e.g., {"core": "0", "mount": "/", "interface": "eth0"}
    val_used DOUBLE PRECISION,
    val_total DOUBLE PRECISION,
    extra_data JSONB
);

-- Convert to TimescaleDB hypertable (chunk interval: 1 day)
SELECT create_hypertable('metrics', 'timestamp', chunk_time_interval => INTERVAL '1 day');

-- Indexes for query performance
CREATE INDEX idx_metrics_device_time ON metrics(device_id, timestamp DESC);
CREATE INDEX idx_metrics_group_time ON metrics(metric_group, timestamp DESC);
CREATE INDEX idx_metrics_tags ON metrics USING GIN (tags);

-- Compression policy (compress chunks older than 1 hour)
ALTER TABLE metrics SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'device_id, metric_group',
    timescaledb.compress_orderby = 'timestamp DESC'
);
SELECT add_compression_policy('metrics', INTERVAL '1 hour');

-- Retention policy (configurable, default 90 days)
SELECT add_retention_policy('metrics', INTERVAL '90 days');


-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- Remove retention policy
SELECT remove_retention_policy('metrics');

-- Remove compression policy
SELECT remove_compression_policy('metrics');

-- Drop metrics table (hypertable)
DROP TABLE metrics;

-- Drop monitors indexes
DROP INDEX idx_monitors_active;
DROP INDEX idx_monitors_deleted_at;
DROP INDEX idx_monitors_plugin_id;
DROP INDEX idx_monitors_status;
DROP INDEX idx_monitors_ip_address;

-- Drop monitors table
DROP TABLE monitors;

-- Drop discovery_profiles index
DROP INDEX idx_discovery_profiles_deleted_at;

-- Drop discovery_profiles table
DROP TABLE discovery_profiles;

-- Drop credential_profiles indexes
DROP INDEX idx_credential_profiles_deleted_at;
DROP INDEX idx_credential_profiles_protocol;

-- Drop credential_profiles table
DROP TABLE credential_profiles;
-- +goose StatementEnd
