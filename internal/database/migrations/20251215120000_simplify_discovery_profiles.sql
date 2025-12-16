-- +goose Up
-- +goose StatementBegin
SELECT 'up SQL query - simplifying discovery profiles';

-- Step 1: Create new discovery_profiles table with simplified schema
CREATE TABLE discovery_profiles_new (
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
    auto_provision BOOLEAN DEFAULT false
);

-- Step 2: Migrate existing data - expand each profile into multiple profiles
-- For each existing profile with N ports and M credentials, create N×M new profiles
INSERT INTO discovery_profiles_new (
    id, name, target_value, port, port_scan_timeout_ms, 
    credential_profile_id, last_run_at, last_run_status, 
    devices_discovered, created_at, updated_at, deleted_at, auto_provision
)
SELECT 
    gen_random_uuid() as id,
    CASE 
        -- If only one port and one credential, keep original name
        WHEN jsonb_array_length(old.ports) = 1 AND jsonb_array_length(old.credential_profile_ids) = 1 
        THEN old.name
        -- Otherwise add suffix to distinguish
        ELSE old.name || ' - Port ' || (port_val #>> '{}') || ' - Cred ' || (cred_idx)::text
    END as name,
    old.target_value,
    (port_val #>> '{}')::int as port,
    old.port_scan_timeout_ms,
    (cred_val #>> '{}')::uuid as credential_profile_id,
    old.last_run_at,
    old.last_run_status,
    old.devices_discovered,
    old.created_at,
    NOW() as updated_at, -- Mark as updated during migration
    old.deleted_at,
    old.auto_provision
FROM discovery_profiles old
CROSS JOIN LATERAL jsonb_array_elements(old.ports) WITH ORDINALITY AS ports_data(port_val, port_idx)
CROSS JOIN LATERAL jsonb_array_elements(old.credential_profile_ids) WITH ORDINALITY AS creds_data(cred_val, cred_idx)
WHERE (cred_val #>> '{}') IS NOT NULL AND (cred_val #>> '{}') != '';

-- Step 3: Update monitors to reference the correct new profile
-- Match based on original discovery_profile_id's target_value
UPDATE monitors m
SET discovery_profile_id = (
    SELECT dpn.id
    FROM discovery_profiles_new dpn
    INNER JOIN discovery_profiles old ON dpn.target_value = old.target_value
    WHERE old.id = m.discovery_profile_id
    LIMIT 1
)
WHERE EXISTS (
    SELECT 1 FROM discovery_profiles_new dpn
    INNER JOIN discovery_profiles old ON dpn.target_value = old.target_value
    WHERE old.id = m.discovery_profile_id
);

-- Step 4: Drop old table and rename new table
DROP TABLE discovery_profiles CASCADE;
ALTER TABLE discovery_profiles_new RENAME TO discovery_profiles;

-- Step 5: Recreate indexes
CREATE INDEX idx_discovery_profiles_deleted_at ON discovery_profiles(deleted_at) WHERE deleted_at IS NULL;
CREATE INDEX idx_discovery_profiles_credential ON discovery_profiles(credential_profile_id);
CREATE INDEX idx_discovery_profiles_port ON discovery_profiles(port);

-- Step 6: Recreate FK constraint on monitors table
ALTER TABLE monitors ADD CONSTRAINT monitors_discovery_profile_id_fkey 
    FOREIGN KEY (discovery_profile_id) REFERENCES discovery_profiles(id);

-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
SELECT 'down SQL query - reverting discovery profile simplification';

-- Note: This down migration will lose data if profiles were expanded
-- We cannot reliably reconstruct the original array structure

-- Create old schema table
CREATE TABLE discovery_profiles_old (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    target_value TEXT NOT NULL,
    ports JSONB NOT NULL,
    port_scan_timeout_ms INT DEFAULT 1000,
    credential_profile_ids JSONB NOT NULL,
    last_run_at TIMESTAMPTZ,
    last_run_status VARCHAR(50),
    devices_discovered INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ DEFAULT NULL,
    auto_provision BOOLEAN DEFAULT false
);

-- Group profiles back - this is a lossy operation
-- We'll group by name prefix (before " - Port")
INSERT INTO discovery_profiles_old (
    id, name, target_value, ports, port_scan_timeout_ms,
    credential_profile_ids, last_run_at, last_run_status,
    devices_discovered, created_at, updated_at, deleted_at, auto_provision
)
SELECT 
    gen_random_uuid() as id,
    CASE 
        WHEN name LIKE '% - Port %' THEN split_part(name, ' - Port ', 1)
        ELSE name
    END as name,
    target_value,
    jsonb_agg(DISTINCT port) as ports,
    MAX(port_scan_timeout_ms) as port_scan_timeout_ms,
    jsonb_agg(DISTINCT credential_profile_id) as credential_profile_ids,
    MAX(last_run_at) as last_run_at,
    MAX(last_run_status) as last_run_status,
    MAX(devices_discovered) as devices_discovered,
    MIN(created_at) as created_at,
    MAX(updated_at) as updated_at,
    MAX(deleted_at) as deleted_at,
    bool_or(auto_provision) as auto_provision
FROM discovery_profiles
GROUP BY 
    CASE 
        WHEN name LIKE '% - Port %' THEN split_part(name, ' - Port ', 1)
        ELSE name
    END,
    target_value;

DROP TABLE discovery_profiles;
ALTER TABLE discovery_profiles_old RENAME TO discovery_profiles;

CREATE INDEX idx_discovery_profiles_deleted_at ON discovery_profiles(deleted_at) WHERE deleted_at IS NULL;

-- +goose StatementEnd
