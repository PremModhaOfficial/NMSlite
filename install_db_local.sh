#!/bin/bash
set -e

# 1. Fix potential broken state from previous attempt
echo "Fixing package state..."
sudo apt-get install -f -y

# 2. Install PostgreSQL 15 and the CORRECT TimescaleDB extension package
# We use timescaledb-2-postgresql-15 because it's already present on your system
echo "Installing/Verifying PostgreSQL 15 and TimescaleDB..."
sudo apt-get install -y postgresql-15 postgresql-client-15 timescaledb-2-postgresql-15

# 3. Configure TimescaleDB
echo "Configuring TimescaleDB..."
# Run timescaledb-tune to automatically configure postgresql.conf
# We use --continue-on-error in case it was already tuned
sudo timescaledb-tune --quiet --yes || echo "Tune warning (ignoring)..."

# Restart PostgreSQL to apply changes
sudo systemctl restart postgresql

# 4. Setup Database and User
echo "Setting up database 'nmslite' and user 'nmslite'..."

sudo -u postgres psql -c "CREATE USER nmslite WITH PASSWORD 'nmslite';" || echo "User 'nmslite' likely already exists, skipping creation."
sudo -u postgres psql -c "ALTER USER nmslite WITH SUPERUSER;" 
sudo -u postgres psql -c "CREATE DATABASE nmslite OWNER nmslite;" || echo "Database 'nmslite' likely already exists, skipping creation."

# 5. Enable Extension
echo "Enabling TimescaleDB extension..."
sudo -u postgres psql -d nmslite -c "CREATE EXTENSION IF NOT EXISTS timescaledb;"

echo "=========================================="
echo "Installation Complete!"
echo "Database: nmslite"
echo "User:     nmslite"
echo "Password: nmslite"
echo "Host:     localhost:5432"
echo "=========================================="