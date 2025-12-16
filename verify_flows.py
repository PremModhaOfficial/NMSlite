import requests
import json
import sys
import time
import urllib3

urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)

BASE_URL = "https://localhost:8080/api/v1"

def log(msg):
    print(f"[TEST] {msg}")

def fail(msg):
    print(f"[TEST] FAILED: {msg}")
    sys.exit(1)

# 1. Login
log("Logging in...")
try:
    resp = requests.post(f"{BASE_URL}/login", json={"username": "admin", "password": "Admin@123"}, verify=False)
    if resp.status_code != 200:
        fail(f"Login failed: {resp.text}")
    token = resp.json()["token"]
    headers = {"Authorization": f"Bearer {token}"}
    log("Login successful.")
except Exception as e:
    fail(f"Login exception: {e}")

# 2. Create Credential Profile
log("Creating Credential Profile...")
cred_payload = {
    "name": "Test WinRM Creds",
    "description": "Created by test script",
    "protocol": "winrm",
    "payload": {"username": "Administrator", "password": "Password123!"} 
}
resp = requests.post(f"{BASE_URL}/credentials", json=cred_payload, headers=headers, verify=False)
if resp.status_code != 201:
    fail(f"Create Credential failed: {resp.text}")
cred_id = resp.json()["id"]
log(f"Credential created: {cred_id}")

# 3. Create Discovery Profile
log("Creating Discovery Profile...")
disc_payload = {
    "name": "Test Discovery",
    "target_value": "192.168.1.10", # Dummy IP
    "credential_profile_id": cred_id,
    "port": 5985,
    "auto_run": False
}
resp = requests.post(f"{BASE_URL}/discoveries", json=disc_payload, headers=headers, verify=False)
if resp.status_code != 201:
    fail(f"Create Discovery failed: {resp.text}")
disc_id = resp.json()["id"]
log(f"Discovery profile created: {disc_id}")

# 4. Add Monitor (Verify: Fetch -> Push -> Scheduler Add)
log("Creating Monitor (Triggers: Add Monitor Flow)...")
mon_payload = {
    "ip_address": "192.168.1.10",
    "plugin_id": "windows-winrm",
    "credential_profile_id": cred_id,
    "discovery_profile_id": disc_id,
    "polling_interval_seconds": 30,
    "status": "active"
}
resp = requests.post(f"{BASE_URL}/monitors", json=mon_payload, headers=headers, verify=False)
if resp.status_code != 201:
    fail(f"Create Monitor failed: {resp.text}")
mon_id = resp.json()["id"]
log(f"Monitor created: {mon_id}")

# Wait for scheduler to process
time.sleep(2)

# 5. Update Credential (Verify: Fetch Affected -> Push Batch -> Scheduler Update All)
log("Updating Credential (Triggers: Update Credential Flow)...")
update_cred_payload = {
    "name": "Test WinRM Creds Updated",
    "description": "Updated by test script",
    "protocol": "winrm",
    "payload": {"username": "Administrator", "password": "NewPassword123!"}
}
resp = requests.put(f"{BASE_URL}/credentials/{cred_id}", json=update_cred_payload, headers=headers, verify=False)
if resp.status_code != 200:
    fail(f"Update Credential failed: {resp.text}")
log("Credential updated.")

# Wait for scheduler to process
time.sleep(2)

# 6. Delete Monitor (Verify: Push ID -> Scheduler Remove)
log("Deleting Monitor (Triggers: Delete Monitor Flow)...")
resp = requests.delete(f"{BASE_URL}/monitors/{mon_id}", headers=headers, verify=False)
if resp.status_code != 204:
    fail(f"Delete Monitor failed: {resp.text}")
log("Monitor deleted.")

# Wait for scheduler to process
time.sleep(2)

log("All steps completed successfully.")
