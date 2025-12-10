-- +goose Up
-- +goose StatementBegin
CREATE TABLE discovered_devices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    discovery_profile_id UUID REFERENCES discovery_profiles(id) ON DELETE CASCADE,
    
    -- Identity
    ip_address INET NOT NULL,
    hostname VARCHAR(255),
    
    -- Technical Details found during scan
    port INT NOT NULL, -- The open port we found (e.g., 5985)
    
    -- Workflow State
    status VARCHAR(50) DEFAULT 'new', -- 'new', 'provisioned', 'ignored'
    
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_discovered_devices_profile ON discovered_devices(discovery_profile_id);
CREATE INDEX idx_discovered_devices_status ON discovered_devices(status);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE discovered_devices;
-- +goose StatementEnd
