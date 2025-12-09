# Postman Collection for NMSlite Mock API

## Overview

The `NMSlite.postman_collection.json` file contains a complete, ready-to-use Postman collection for testing all 19 API endpoints.

## Features

✅ All 19 endpoints organized in folders  
✅ Automatic environment variable setup  
✅ Pre-configured request bodies  
✅ Automated tests for each endpoint  
✅ Token management (auto-save from login)  
✅ Base URL configuration  
✅ Response validation tests  

## Installation

### Option 1: Import from File

1. Open **Postman**
2. Click **Import** button (top-left)
3. Select **File** tab
4. Choose `NMSlite.postman_collection.json`
5. Click **Import**

### Option 2: Import from Link

1. Open **Postman**
2. Click **Import** button
3. Paste the file path or drag the file directly

### Option 3: Folder Import

If using Postman with folder sync:
1. Copy `NMSlite.postman_collection.json` to your Postman collections folder
2. Restart Postman
3. Collection will appear in your Collections panel

## Initial Setup

### 1. Create an Environment (Optional but Recommended)

While the collection has default variables, creating an environment helps manage different setups:

1. Click the **Environments** gear icon (top-right)
2. Click **Create New Environment**
3. Name it: `NMSlite Local`
4. Set these variables:
   - `base_url`: `http://localhost:8443/api/v1`
   - `health_url`: `http://localhost:8443`

### 2. Start the Mock API Server

```bash
cd /home/prem-modha/projects/NMSlite
./nmslite
```

The server starts on `http://localhost:8443`

### 3. Select Environment

In Postman, select the environment dropdown (top-right) and choose your environment or use the collection defaults.

## Usage Guide

### Typical Workflow

#### 1. **Health Check** (No auth required)
- Request: `GET /health`
- Purpose: Verify API is running
- Expected Status: 200

#### 2. **Login** 
- Request: `POST /auth/login`
- Body: Username & password (admin/secret)
- **Auto-saves tokens** to environment variables
- Expected Status: 200

#### 3. **Create Credential** (Optional)
- Request: `POST /credentials`
- Creates a new credential for device authentication
- Auto-saves `credential_id` to environment
- Expected Status: 201

#### 4. **Create Device**
- Request: `POST /devices`
- Creates a new device to monitor
- Auto-saves `device_id` to environment
- Expected Status: 201

#### 5. **Provision Device**
- Request: `POST /devices/{id}/provision`
- Links credential to device and enables monitoring
- Expected Status: 200

#### 6. **Get Metrics**
- Request: `GET /devices/{id}/metrics`
- Retrieve latest metrics for a device
- Expected Status: 200

#### 7. **Get Metrics History**
- Request: `POST /devices/{id}/metrics/history`
- Retrieve historical metrics
- Expected Status: 200

## Collection Organization

```
NMSlite Mock API
├── Health Check
│   └── Get Health Status
├── Authentication
│   ├── Login ⭐ (saves tokens)
│   └── Refresh Token
├── Credentials
│   ├── List All Credentials
│   ├── Create Credential ⭐ (saves credential_id)
│   ├── Get Credential by ID
│   ├── Update Credential
│   └── Delete Credential
├── Devices
│   ├── List All Devices
│   ├── Create Device ⭐ (saves device_id)
│   ├── Get Device by ID
│   ├── Update Device
│   ├── Delete Device
│   ├── Discover Devices in Subnet
│   ├── Provision Device ⭐
│   └── Deprovision Device
└── Metrics
    ├── Get Latest Metrics
    └── Get Metrics History
```

⭐ = Automatically saves variables for use in subsequent requests

## Automated Tests

Each request includes automated tests that:
- ✅ Verify HTTP status codes
- ✅ Check response structure
- ✅ Validate data integrity
- ✅ Save variables for dependent requests

Run tests by clicking **Send** on any request. Results appear in the **Tests** tab.

## Environment Variables

The collection uses these variables automatically:

| Variable | Default | Purpose |
|----------|---------|---------|
| `base_url` | http://localhost:8443/api/v1 | API base URL |
| `access_token` | (auto) | JWT access token (set by Login) |
| `refresh_token` | (auto) | JWT refresh token (set by Login) |
| `device_id` | 1 | Current device ID (set by Create Device) |
| `credential_id` | 1 | Current credential ID (set by Create Credential) |

## Test Credentials

**Username:** `admin`  
**Password:** `secret`

## Pre-populated Data

The mock API includes sample data you can use immediately:

- **Device 1**: 192.168.1.100 (SERVER-01)
- **Credential 1**: Default WinRM Credential
- **Metrics**: Pre-loaded sample data for device 1

## Running Complete Test Suite

### Method 1: Manual Sequential Testing

1. Click **Health Check** → **Get Health Status** → **Send** ✓
2. Click **Authentication** → **Login** → **Send** ✓
3. Click **Credentials** → **List All Credentials** → **Send** ✓
4. Click **Devices** → **List All Devices** → **Send** ✓
5. Click **Metrics** → **Get Latest Metrics** → **Send** ✓

### Method 2: Collection Runner (Automated)

1. Click **Collection** name
2. Click the **Runner** icon (▶)
3. Select the **NMSlite Mock API** collection
4. Click **Run NMSlite Mock API**
5. All requests execute in sequence with tests

**Note:** Some tests may fail if requests have dependencies. Run Login first!

## Common Tasks

### Test a Single Endpoint

1. Find the endpoint in the collection
2. Click on it
3. Modify the request body if needed
4. Click **Send**
5. Review response in the **Body** tab
6. Check tests in the **Tests** tab

### Create a New Device

1. Open **Devices** → **Create Device**
2. Modify the JSON body with your device IP
3. Click **Send**
4. The new device ID is auto-saved
5. Use this ID for subsequent operations

### Update Device Settings

1. Open **Devices** → **Update Device**
2. Change the URL parameter from `/devices/1` to your device ID
3. Modify the JSON body with new values
4. Click **Send**

### Test Device Discovery

1. Open **Devices** → **Discover Devices in Subnet**
2. Change subnet value as needed (e.g., "192.168.100.0/24")
3. Click **Send**
4. View discovered devices in response

### Check Device Metrics

1. Open **Metrics** → **Get Latest Metrics**
2. Ensure `/devices/1` uses correct device ID
3. Click **Send**
4. View CPU, Memory, Disk, Network metrics

## Troubleshooting

### "Connection refused" Error

**Problem:** Cannot connect to API
**Solution:**
1. Verify server is running: `./nmslite`
2. Check port 8443 is accessible
3. Confirm base_url is correct: `http://localhost:8443/api/v1`

### "Test failed" Errors

**Problem:** Test assertions failing
**Solution:**
1. Verify mock data exists (run Create requests first)
2. Check response body in the **Body** tab
3. Review test code in the **Tests** tab
4. Ensure IDs match (device_id, credential_id)

### Token Expired

**Problem:** 401 Unauthorized after waiting
**Solution:**
1. Run **Authentication** → **Login** again
2. This refreshes the access_token variable
3. Tokens are valid for 15 minutes

### "Not Found" (404) Errors

**Problem:** Resource doesn't exist
**Solution:**
1. Verify the ID parameter is correct
2. Create the resource first if it doesn't exist
3. Check that previous create requests succeeded

## Advanced Usage

### Custom Environment for Testing

Create a separate environment for different test scenarios:

```json
{
  "name": "NMSlite Staging",
  "values": [
    {
      "key": "base_url",
      "value": "http://staging.example.com:8443/api/v1",
      "enabled": true
    },
    {
      "key": "device_id",
      "value": "10",
      "enabled": true
    }
  ]
}
```

### Bulk Operations with Runner

Use the **Collection Runner** with variables to:
- Test multiple device IDs
- Create and delete in sequences
- Load test the API
- Generate test data

### Export Test Results

After running tests:
1. Click the **Runner** icon
2. Run the collection
3. Click **Export Results** to generate reports

## Tips & Tricks

✅ **Keyboard Shortcut:** Use `Ctrl+Enter` (Windows) or `Cmd+Enter` (Mac) to send requests quickly

✅ **Pre-request Scripts:** Auto-generate IDs or timestamps using pre-request scripts

✅ **Response Pretty-Print:** Click **Pretty** in the response to format JSON

✅ **Copy cURL:** Right-click request → **Copy as cURL** to use in terminal

✅ **Save Request as Example:** Click **Save as Example** to create request templates

## Next Steps

After testing the mock API:

1. **Validate Workflow:** Run complete user journeys
2. **Test Error Handling:** Send invalid data to check error responses
3. **Performance Testing:** Use Runner with iterations for load testing
4. **Document Behaviors:** Note API quirks for development team
5. **Share Collection:** Export and share with team members

## Support

For questions about the collection:
- Review `API_ENDPOINTS.md` for endpoint details
- Check `README.md` for quick start
- See `DEVELOPMENT.md` for architecture info

## Collection Version

**Version:** 1.0.0  
**Last Updated:** December 2024  
**API Version:** NMSlite Mock API v1.0
