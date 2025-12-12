-- +goose Up
-- Remove the target_type column from discovery_profiles table
-- This field was orphaned - stored but never used in business logic
-- Target type (CIDR/range/single IP) will now be auto-detected from target_value format
ALTER TABLE discovery_profiles DROP COLUMN target_type;

-- +goose Down
-- Restore the target_type column with default value 'unknown'
-- Note: Historical data cannot be recovered - all restored records will have 'unknown'
ALTER TABLE discovery_profiles ADD COLUMN target_type VARCHAR(50) NOT NULL DEFAULT 'unknown';
