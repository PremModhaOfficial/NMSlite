#!/bin/bash
set -e

# Login
echo "Logging in..."
LOGIN_RESP=$(curl -s -X POST http://localhost:8080/api/v1/login -H "Content-Type: application/json" -d '{"username":"admin", "password":"Admin@123"}')
TOKEN=$(echo $LOGIN_RESP | python3 -c "import sys, json; print(json.load(sys.stdin)['token'])")

echo "Logged in."

# Create Credential Profile
echo "Creating Credential Profile..."
CRED_PAYLOAD='{"name": "WinRM Creds A", "protocol": "windows-winrm", "payload": {"username": "vboxuser", "password": "admin"}}'
CRED_RESP=$(curl -s -X POST http://localhost:8080/api/v1/credentials \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "$CRED_PAYLOAD")
CRED_ID=$(echo $CRED_RESP | python3 -c "import sys, json; print(json.load(sys.stdin)['id'])")

echo "Credential Profile Created: ID=$CRED_ID"

# Create Discovery Profile
echo "Creating Discovery Profile (Auto-Run/Auto-Provision)..."
DISC_PAYLOAD="{\"name\": \"WinRM Discovery A\", \"target_value\": \"127.0.0.1\", \"port\": 15985, \"credential_profile_id\": $CRED_ID, \"auto_run\": true, \"auto_provision\": true}"
DISC_RESP=$(curl -s -X POST http://localhost:8080/api/v1/discoveries \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "$DISC_PAYLOAD")

echo "Discovery Profile Created."
