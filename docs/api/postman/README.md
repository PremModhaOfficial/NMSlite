# NMS Lite API - Postman Collection

This directory contains Postman collection and environment files for testing the NMS Lite API.

## Files

| File | Description |
|------|-------------|
| `NMSlite-API.postman_collection.json` | Complete API collection with all endpoints |
| `NMSlite-Environment.postman_environment.json` | Environment variables for local testing |

## Quick Start

### 1. Import into Postman

1. Open Postman
2. Click **Import** (top-left)
3. Drag and drop both JSON files, or click **Upload Files** and select them
4. The collection "NMS Lite API" and environment "NMS Lite - Local" will appear

### 2. Select Environment

1. In the top-right corner, click the environment dropdown
2. Select **NMS Lite - Local**

### 3. Start Testing

1. Ensure the NMS Lite server is running on `http://localhost:8080`
2. Execute requests in order (see Testing Workflow below)

## Testing Workflow

Follow this sequence for a complete test:

```
1. Health & Auth
   ├── GET  /health           → Verify server is running
   ├── GET  /ready            → Verify all services ready
   └── POST /api/v1/login     → Get JWT token (auto-saved)

2. Credentials (need token first)
   ├── GET  /credentials      → List existing (should be empty)
   ├── POST /credentials      → Create SSH credential (ID auto-saved)
   ├── GET  /credentials/{id} → Verify creation
   ├── PUT  /credentials/{id} → Update credential
   └── DELETE /credentials/{id} → Remove (skip if needed for discovery)

3. Discoveries (need credential first)
   ├── GET  /discoveries      → List existing
   ├── POST /discoveries      → Create discovery profile (ID auto-saved)
   ├── GET  /discoveries/{id} → Verify creation
   ├── POST /discoveries/{id}/run → Start scan
   └── GET  /discoveries/{id}/results → Get final results

4. Monitors (created by discovery)
   ├── GET  /monitors         → List discovered devices
   ├── GET  /monitors/{id}    → Get device details
   ├── PATCH /monitors/{id}   → Update configuration
   ├── DELETE /monitors/{id}  → Soft delete
   └── PATCH /monitors/{id}/restore → Restore deleted

5. Metrics (batch queries)
   └── POST /metrics/query    → Query metrics for one or more monitors

6. Protocols (reference)
   └── GET  /protocols        → List all supported protocols
```

## Environment Variables

| Variable | Description | Auto-populated |
|----------|-------------|----------------|
| `base_url` | API server URL (default: `http://localhost:8080`) | No |
| `admin_username` | Login username (default: `admin`) | No |
| `admin_password` | Login password (default: `Admin@123`) | No |
| `token` | JWT authentication token | Yes (after login) |
| `credential_id` | Last created credential profile ID | Yes (after create) |
| `discovery_id` | Last created discovery profile ID | Yes (after create) |
| `monitor_id` | Monitor/device ID | Yes (from list) |

### Customizing Variables

1. Click the **eye icon** next to the environment dropdown
2. Click **Edit** on the environment
3. Modify values as needed:
   - Change `base_url` for remote servers
   - Update credentials if different from defaults
   - Manually set IDs for specific resources

## Credential Data Examples

### SSH
```json
{
    "name": "Linux Servers SSH",
    "description": "SSH credentials for Linux infrastructure",
    "protocol": "ssh",
    "credential_data": {
        "username": "admin",
        "password": "secure_password",
        "port": 22
    }
}
```

### SSH with Key Authentication
```json
{
    "name": "Linux Servers SSH Key",
    "description": "SSH key-based authentication",
    "protocol": "ssh",
    "credential_data": {
        "username": "admin",
        "private_key": "-----BEGIN RSA PRIVATE KEY-----\n...\n-----END RSA PRIVATE KEY-----",
        "passphrase": "key_passphrase",
        "port": 22
    }
}
```

### WinRM
```json
{
    "name": "Windows Servers WinRM",
    "description": "Windows Remote Management credentials",
    "protocol": "winrm",
    "credential_data": {
        "username": "administrator",
        "password": "WindowsPassword123!",
        "domain": "CORP"
    }
}
```

### SNMP v2c
```json
{
    "name": "Network Devices SNMP",
    "description": "SNMP v2c community string",
    "protocol": "snmp-v2c",
    "credential_data": {
        "community": "public",
        "port": 161
    }
}
```

### SNMP v3
```json
{
    "name": "Secure Network SNMP v3",
    "description": "SNMP v3 with authentication and privacy",
    "protocol": "snmp-v3",
    "credential_data": {
        "username": "snmpuser",
        "security_level": "authPriv",
        "auth_protocol": "SHA",
        "auth_password": "authpass123",
        "priv_protocol": "AES",
        "priv_password": "privpass123",
        "port": 161
    }
}
```

**SNMP v3 Security Levels:**
- `noAuthNoPriv` - No authentication or privacy
- `authNoPriv` - Authentication only
- `authPriv` - Authentication and privacy (recommended)

**Authentication Protocols:** `MD5`, `SHA`, `SHA-224`, `SHA-256`, `SHA-384`, `SHA-512`

**Privacy Protocols:** `DES`, `3DES`, `AES`, `AES-192`, `AES-256`

## Discovery Target Types

| Type | Example | Description |
|------|---------|-------------|
| `cidr` | `192.168.1.0/24` | Scan entire subnet (256 IPs) |
| `ip_range` | `192.168.1.1-192.168.1.50` | Scan specific IP range |
| `ip` | `192.168.1.100` | Scan single IP address |

## Response Formats

### Success Response
```json
{
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "name": "Resource Name",
    "created_at": "2025-12-10T12:00:00Z",
    "updated_at": "2025-12-10T12:00:00Z"
}
```

### List Response
```json
{
    "credentials": [...],
    "pagination": {
        "page": 1,
        "limit": 50,
        "total": 10
    }
}
```

### Error Response
```json
{
    "error": {
        "code": "VALIDATION_ERROR",
        "message": "Request validation failed",
        "details": [
            {
                "field": "ip_address",
                "reason": "Invalid IP address format"
            }
        ],
        "request_id": "req_abc123"
    }
}
```

## Common Error Codes

| Code | HTTP Status | Description |
|------|-------------|-------------|
| `VALIDATION_ERROR` | 400 | Invalid request payload |
| `INVALID_REQUEST` | 400 | Malformed JSON |
| `UNAUTHORIZED` | 401 | Missing or invalid JWT token |
| `FORBIDDEN` | 403 | Insufficient permissions |
| `NOT_FOUND` | 404 | Resource doesn't exist |
| `CONFLICT` | 409 | Resource already exists |
| `UNPROCESSABLE_ENTITY` | 422 | Business logic error |
| `INTERNAL_ERROR` | 500 | Server error |

## Metrics Query Examples

The `/api/v1/metrics/query` endpoint supports batch queries across multiple devices with advanced filtering.

### Basic Query - Single Device
```json
{
    "device_ids": ["550e8400-e29b-41d4-a716-446655440000"],
    "start": "2025-12-01T00:00:00Z",
    "end": "2025-12-31T23:59:59Z",
    "metric_groups": ["host.cpu", "host.memory"],
    "limit": 50
}
```

**Response Format:**
```json
{
    "data": {
        "550e8400-e29b-41d4-a716-446655440000": [
            {
                "timestamp": "2025-12-11T10:30:00Z",
                "metric_group": "host.cpu",
                "metric_name": "usage_percent",
                "value": 45.2,
                "unit": "percent",
                "tags": {"core": "0"}
            }
        ]
    },
    "count": 10,
    "query": {...}
}
```

### Multi-Device Query
```json
{
    "device_ids": [
        "550e8400-e29b-41d4-a716-446655440000",
        "660e8400-e29b-41d4-a716-446655440001"
    ],
    "start": "2025-12-01T00:00:00Z",
    "end": "2025-12-31T23:59:59Z",
    "limit": 100
}
```
Results are grouped by `device_id` in the response. Invalid device IDs are silently ignored.

### Latest Metrics Only
```json
{
    "device_ids": ["550e8400-e29b-41d4-a716-446655440000"],
    "start": "2025-12-01T00:00:00Z",
    "end": "2025-12-31T23:59:59Z",
    "metric_groups": ["host.cpu", "host.memory"],
    "latest": true
}
```
Returns only the most recent metric per device/metric_group combination - useful for dashboards.

### Query with Tag Filters
```json
{
    "device_ids": ["550e8400-e29b-41d4-a716-446655440000"],
    "start": "2025-12-01T00:00:00Z",
    "end": "2025-12-31T23:59:59Z",
    "metric_groups": ["host.cpu"],
    "tag_filters": [
        {"key": "core", "op": "eq", "values": ["0"]},
        {"key": "type", "op": "in", "values": ["user", "system"]}
    ],
    "limit": 100
}
```

**Available Tag Filter Operators:**

| Operator | Description | Example |
|----------|-------------|---------|
| `eq` | Equals (exact match) | `{"key": "core", "op": "eq", "values": ["0"]}` |
| `in` | In list (matches any) | `{"key": "core", "op": "in", "values": ["0", "1", "2"]}` |
| `like` | SQL LIKE pattern (% wildcards) | `{"key": "device", "op": "like", "values": ["eth%"]}` |
| `exists` | Tag key exists | `{"key": "error", "op": "exists", "values": []}` |
| `gt` | Greater than (numeric) | `{"key": "threshold", "op": "gt", "values": ["80"]}` |
| `lt` | Less than (numeric) | `{"key": "threshold", "op": "lt", "values": ["20"]}` |
| `gte` | Greater than or equal | `{"key": "priority", "op": "gte", "values": ["5"]}` |
| `lte` | Less than or equal | `{"key": "priority", "op": "lte", "values": ["3"]}` |

Multiple filters are combined with **AND** logic.

### Common Use Cases

**Get CPU metrics for specific cores:**
```json
{
    "device_ids": ["..."],
    "start": "2025-12-11T00:00:00Z",
    "end": "2025-12-11T23:59:59Z",
    "metric_groups": ["host.cpu"],
    "tag_filters": [
        {"key": "core", "op": "in", "values": ["0", "1", "2", "3"]}
    ]
}
```

**Get network metrics for specific interfaces:**
```json
{
    "device_ids": ["..."],
    "start": "2025-12-11T00:00:00Z",
    "end": "2025-12-11T23:59:59Z",
    "metric_groups": ["net.interface"],
    "tag_filters": [
        {"key": "interface", "op": "like", "values": ["eth%"]}
    ]
}
```

**Find metrics exceeding thresholds:**
```json
{
    "device_ids": ["..."],
    "start": "2025-12-11T00:00:00Z",
    "end": "2025-12-11T23:59:59Z",
    "tag_filters": [
        {"key": "threshold_exceeded", "op": "eq", "values": ["true"]}
    ]
}
```

## Tips

### Running Tests Automatically
- Use Postman's **Collection Runner** to execute all requests in sequence
- Tests validate status codes and response structure automatically

### Token Expiration
- JWT tokens expire after a configured time (check `expires_at` in login response)
- Re-run the Login request to get a fresh token

### Parallel Testing
- Create additional environments for different servers (staging, production)
- Switch environments to test different deployments

### Debugging
- Check the Postman Console (View → Show Postman Console) for detailed request/response logs
- Use `console.log()` in test scripts for debugging

## Troubleshooting

**"UNAUTHORIZED" errors:**
- Run the Login request first
- Check if token has expired
- Verify credentials are correct

**"NOT_FOUND" errors:**
- Verify the resource ID exists
- Check if the resource was soft-deleted

**Connection refused:**
- Ensure NMS Lite server is running
- Verify `base_url` is correct
- Check firewall/network settings
