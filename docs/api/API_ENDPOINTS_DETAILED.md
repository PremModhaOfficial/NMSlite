# NMS Lite API Endpoints Documentation

**Base URL:** `http://localhost:8080/api/v1`

**Authentication:** All endpoints (except `/login`, `/health`, `/ready`) require JWT Bearer token in the `Authorization` header.

---

## Authentication

### Login
**POST** `/api/v1/login`

Authenticate and receive JWT token.

**Request:**
```json
{
  "username": "admin",
  "password": "changeme"
}
```

**Response (200 OK):**
```json
{
  "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "expires_at": "2023-12-08T12:00:00Z",
  "user": "admin"
}
```

**Errors:**
- `401 UNAUTHORIZED` - Invalid credentials

---

## Protocols

### List All Supported Protocols
**GET** `/api/v1/protocols`

Returns all available protocols with their metadata.

**Response (200 OK):**
```json
{
  "protocols": [
    {
      "id": "winrm",
      "name": "Windows Remote Management",
      "description": "Windows Server management protocol",
      "default_port": 5985,
      "transport": "http"
    },
    {
      "id": "ssh",
      "name": "Secure Shell",
      "description": "SSH protocol for Linux/Unix systems",
      "default_port": 22,
      "transport": "tcp"
    },
    {
      "id": "snmp-v2c",
      "name": "SNMP v2c",
      "description": "Simple Network Management Protocol v2c",
      "default_port": 161,
      "transport": "udp"
    }
  ]
}
```

### Get Protocol Schema
**GET** `/api/v1/protocols/{protocol_id}/schema`

Returns the JSON schema for a specific protocol's credential structure.

**Example:** `GET /api/v1/protocols/winrm/schema`

**Response (200 OK):**
```json
{
  "protocol": "winrm",
  "schema": {
    "$schema": "http://json-schema.org/draft-07/schema#",
    "type": "object",
    "required": ["username", "password"],
    "properties": {
      "username": {
        "type": "string",
        "minLength": 1,
        "description": "Windows username"
      },
      "password": {
        "type": "string",
        "minLength": 1,
        "description": "Windows password"
      },
      "domain": {
        "type": "string",
        "description": "Windows domain (optional)"
      },
      "use_https": {
        "type": "boolean",
        "default": false,
        "description": "Use HTTPS instead of HTTP"
      }
    }
  }
}
```

**Errors:**
- `404 NOT_FOUND` - Protocol not found

---

## Credential Profiles

### List All Credential Profiles
**GET** `/api/v1/credentials`

**Query Parameters:**
- `protocol` (optional) - Filter by protocol (e.g., `?protocol=winrm`)
- `page` (optional) - Page number (default: 1)
- `limit` (optional) - Items per page (default: 50)

**Response (200 OK):**
```json
{
  "credentials": [
    {
      "id": "550e8400-e29b-41d4-a716-446655440000",
      "name": "Production Windows Cluster",
      "description": "Credentials for all Win2019 servers",
      "protocol": "winrm",
      "created_at": "2023-12-07T10:00:00Z",
      "updated_at": "2023-12-07T10:00:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 50,
    "total": 1
  }
}
```

**Note:** `credential_details` is never returned for security reasons.

### Create Credential Profile
**POST** `/api/v1/credentials`

**Request:**
```json
{
  "name": "Production Windows Cluster",
  "description": "Credentials for all Win2019 servers",
  "protocol": "winrm",
  "credential_details": {
    "username": "admin_user",
    "password": "SecurePassword123!",
    "domain": "internal.corp",
    "use_https": false
  }
}
```

**Response (201 Created):**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "Production Windows Cluster",
  "description": "Credentials for all Win2019 servers",
  "protocol": "winrm",
  "created_at": "2023-12-07T10:00:00Z",
  "updated_at": "2023-12-07T10:00:00Z"
}
```

**Errors:**
- `400 VALIDATION_ERROR` - Invalid credential_details (doesn't match protocol schema)
- `409 CONFLICT` - Credential profile with same name already exists

### Get Credential Profile by ID
**GET** `/api/v1/credentials/{id}`

**Response (200 OK):**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "Production Windows Cluster",
  "description": "Credentials for all Win2019 servers",
  "protocol": "winrm",
  "created_at": "2023-12-07T10:00:00Z",
  "updated_at": "2023-12-07T10:00:00Z"
}
```

**Errors:**
- `404 NOT_FOUND` - Credential profile not found

### Update Credential Profile
**PUT** `/api/v1/credentials/{id}`

**Request:**
```json
{
  "name": "Updated Windows Cluster",
  "description": "Updated description",
  "credential_details": {
    "username": "new_admin",
    "password": "NewPassword123!",
    "domain": "internal.corp"
  }
}
```

**Response (200 OK):**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "Updated Windows Cluster",
  "description": "Updated description",
  "protocol": "winrm",
  "created_at": "2023-12-07T10:00:00Z",
  "updated_at": "2023-12-07T12:00:00Z"
}
```

**Errors:**
- `400 VALIDATION_ERROR` - Invalid credential_details
- `404 NOT_FOUND` - Credential profile not found
- `422 UNPROCESSABLE_ENTITY` - Cannot change protocol of existing credential

### Delete Credential Profile (Soft Delete)
**DELETE** `/api/v1/credentials/{id}`

**Response (204 No Content)**

**Errors:**
- `404 NOT_FOUND` - Credential profile not found
- `409 CONFLICT` - Credential is in use by active monitors

---

## Discovery Profiles

### List All Discovery Profiles
**GET** `/api/v1/discoveries`

**Query Parameters:**
- `page` (optional) - Page number (default: 1)
- `limit` (optional) - Items per page (default: 50)

**Response (200 OK):**
```json
{
  "discoveries": [
    {
      "id": "660e8400-e29b-41d4-a716-446655440001",
      "name": "Datacenter A - Subnet 10",
      "target_type": "cidr",
      "target_value": "192.168.10.0/24",
      "ports": [22, 5985, 443],
      "port_scan_timeout_ms": 2000,
      "credential_profile_ids": [
        "550e8400-e29b-41d4-a716-446655440000"
      ],
      "last_run_at": "2023-12-07T11:00:00Z",
      "last_run_status": "success",
      "devices_discovered": 15,
      "created_at": "2023-12-07T09:00:00Z",
      "updated_at": "2023-12-07T11:00:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 50,
    "total": 1
  }
}
```

### Create Discovery Profile
**POST** `/api/v1/discoveries`

**Request:**
```json
{
  "name": "Datacenter A - Subnet 10",
  "target_type": "cidr",
  "target_value": "192.168.10.0/24",
  "ports": [22, 5985, 443],
  "port_scan_timeout_ms": 2000,
  "credential_profile_ids": [
    "550e8400-e29b-41d4-a716-446655440000"
  ]
}
```

**Target Types:**
- `cidr` - CIDR notation (e.g., `192.168.1.0/24`)
- `ip_range` - IP range (e.g., `192.168.1.1-192.168.1.50`)
- `ip` - Single IP (e.g., `192.168.1.10`)

**Response (201 Created):**
```json
{
  "id": "660e8400-e29b-41d4-a716-446655440001",
  "name": "Datacenter A - Subnet 10",
  "target_type": "cidr",
  "target_value": "192.168.10.0/24",
  "ports": [22, 5985, 443],
  "port_scan_timeout_ms": 2000,
  "credential_profile_ids": [
    "550e8400-e29b-41d4-a716-446655440000"
  ],
  "created_at": "2023-12-07T09:00:00Z",
  "updated_at": "2023-12-07T09:00:00Z"
}
```

**Errors:**
- `400 VALIDATION_ERROR` - Invalid target format or ports
- `404 NOT_FOUND` - Referenced credential profile not found

### Get Discovery Profile by ID
**GET** `/api/v1/discoveries/{id}`

**Response (200 OK):**
```json
{
  "id": "660e8400-e29b-41d4-a716-446655440001",
  "name": "Datacenter A - Subnet 10",
  "target_type": "cidr",
  "target_value": "192.168.10.0/24",
  "ports": [22, 5985, 443],
  "port_scan_timeout_ms": 2000,
  "credential_profile_ids": [
    "550e8400-e29b-41d4-a716-446655440000"
  ],
  "last_run_at": "2023-12-07T11:00:00Z",
  "last_run_status": "success",
  "devices_discovered": 15,
  "created_at": "2023-12-07T09:00:00Z",
  "updated_at": "2023-12-07T11:00:00Z"
}
```

### Update Discovery Profile
**PUT** `/api/v1/discoveries/{id}`

**Request:**
```json
{
  "name": "Updated Discovery Profile",
  "target_value": "192.168.10.0/25",
  "ports": [22, 5985]
}
```

**Response (200 OK):**
```json
{
  "id": "660e8400-e29b-41d4-a716-446655440001",
  "name": "Updated Discovery Profile",
  "target_type": "cidr",
  "target_value": "192.168.10.0/25",
  "ports": [22, 5985],
  "port_scan_timeout_ms": 2000,
  "credential_profile_ids": [
    "550e8400-e29b-41d4-a716-446655440000"
  ],
  "created_at": "2023-12-07T09:00:00Z",
  "updated_at": "2023-12-07T12:00:00Z"
}
```

### Delete Discovery Profile (Soft Delete)
**DELETE** `/api/v1/discoveries/{id}`

**Response (204 No Content)**

**Errors:**
- `404 NOT_FOUND` - Discovery profile not found

### Run Discovery Job
**POST** `/api/v1/discoveries/{id}/run`

Triggers an asynchronous discovery scan.

**Response (202 Accepted):**
```json
{
  "job_id": "770e8400-e29b-41d4-a716-446655440002",
  "status": "running",
  "started_at": "2023-12-07T12:00:00Z",
  "discovery_profile_id": "660e8400-e29b-41d4-a716-446655440001"
}
```

### Get Discovery Job Status
**GET** `/api/v1/discoveries/{id}/jobs/{job_id}`

**Response (200 OK - Running):**
```json
{
  "job_id": "770e8400-e29b-41d4-a716-446655440002",
  "status": "running",
  "started_at": "2023-12-07T12:00:00Z",
  "progress": {
    "total_ips": 256,
    "scanned": 128,
    "discovered": 15,
    "percentage": 50
  }
}
```

**Response (200 OK - Completed):**
```json
{
  "job_id": "770e8400-e29b-41d4-a716-446655440002",
  "status": "completed",
  "started_at": "2023-12-07T12:00:00Z",
  "completed_at": "2023-12-07T12:05:00Z",
  "result": {
    "total_ips": 256,
    "scanned": 256,
    "discovered": 47,
    "failed": 0
  },
  "devices_created": [
    "880e8400-e29b-41d4-a716-446655440003",
    "880e8400-e29b-41d4-a716-446655440004"
  ]
}
```

**Status Values:**
- `pending` - Job queued but not started
- `running` - Currently executing
- `completed` - Finished successfully
- `failed` - Job failed with error
- `cancelled` - Job was cancelled by user

---

## Monitors (Devices)

### List All Monitors
**GET** `/api/v1/monitors`

**Query Parameters:**
- `status` (optional) - Filter by status: `active`, `maintenance`, `down`
- `plugin_id` (optional) - Filter by plugin
- `page` (optional) - Page number (default: 1)
- `limit` (optional) - Items per page (default: 50)

**Response (200 OK):**
```json
{
  "monitors": [
    {
      "id": "880e8400-e29b-41d4-a716-446655440003",
      "display_name": "db-prod-01",
      "hostname": "db-prod-01.internal",
      "ip_address": "192.168.10.15",
      "plugin_id": "windows-winrm",
      "credential_profile_id": "550e8400-e29b-41d4-a716-446655440000",
      "discovery_profile_id": "660e8400-e29b-41d4-a716-446655440001",
      "polling_interval_seconds": 60,
      "status": "active",
      "consecutive_failures": 0,
      "last_poll_at": "2023-12-07T12:00:00Z",
      "last_successful_poll_at": "2023-12-07T12:00:00Z",
      "created_at": "2023-12-07T11:00:00Z",
      "updated_at": "2023-12-07T12:00:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "limit": 50,
    "total": 1
  }
}
```

### Create Monitor (Manual Provisioning)
**POST** `/api/v1/monitors`

Manually create a monitor without discovery.

**Request:**
```json
{
  "display_name": "web-prod-01",
  "hostname": "web-prod-01.internal",
  "ip_address": "192.168.10.20",
  "plugin_id": "linux-ssh",
  "credential_profile_id": "550e8400-e29b-41d4-a716-446655440000",
  "polling_interval_seconds": 60
}
```

**Response (201 Created):**
```json
{
  "id": "880e8400-e29b-41d4-a716-446655440004",
  "display_name": "web-prod-01",
  "hostname": "web-prod-01.internal",
  "ip_address": "192.168.10.20",
  "plugin_id": "linux-ssh",
  "credential_profile_id": "550e8400-e29b-41d4-a716-446655440000",
  "polling_interval_seconds": 60,
  "status": "active",
  "consecutive_failures": 0,
  "created_at": "2023-12-07T12:00:00Z",
  "updated_at": "2023-12-07T12:00:00Z"
}
```

**Errors:**
- `400 VALIDATION_ERROR` - Invalid IP address or missing required fields
- `404 NOT_FOUND` - Referenced credential or plugin not found
- `409 CONFLICT` - Monitor with same IP already exists

### Get Monitor by ID
**GET** `/api/v1/monitors/{id}`

**Response (200 OK):**
```json
{
  "id": "880e8400-e29b-41d4-a716-446655440003",
  "display_name": "db-prod-01",
  "hostname": "db-prod-01.internal",
  "ip_address": "192.168.10.15",
  "plugin_id": "windows-winrm",
  "credential_profile_id": "550e8400-e29b-41d4-a716-446655440000",
  "discovery_profile_id": "660e8400-e29b-41d4-a716-446655440001",
  "polling_interval_seconds": 60,
  "status": "active",
  "consecutive_failures": 0,
  "last_poll_at": "2023-12-07T12:00:00Z",
  "last_successful_poll_at": "2023-12-07T12:00:00Z",
  "created_at": "2023-12-07T11:00:00Z",
  "updated_at": "2023-12-07T12:00:00Z"
}
```

### Update Monitor
**PATCH** `/api/v1/monitors/{id}`

**Request (Update Status to Maintenance):**
```json
{
  "status": "maintenance"
}
```

**Request (Update Configuration):**
```json
{
  "display_name": "db-prod-01-updated",
  "polling_interval_seconds": 120
}
```

**Response (200 OK):**
```json
{
  "id": "880e8400-e29b-41d4-a716-446655440003",
  "display_name": "db-prod-01-updated",
  "hostname": "db-prod-01.internal",
  "ip_address": "192.168.10.15",
  "plugin_id": "windows-winrm",
  "credential_profile_id": "550e8400-e29b-41d4-a716-446655440000",
  "polling_interval_seconds": 120,
  "status": "maintenance",
  "consecutive_failures": 0,
  "last_poll_at": "2023-12-07T12:00:00Z",
  "created_at": "2023-12-07T11:00:00Z",
  "updated_at": "2023-12-07T12:30:00Z"
}
```

**Valid Status Transitions:**
- `active` → `maintenance`
- `maintenance` → `active`
- `down` → `active`
- `down` → `maintenance`

### Delete Monitor (Soft Delete)
**DELETE** `/api/v1/monitors/{id}`

**Response (204 No Content)**

**Errors:**
- `404 NOT_FOUND` - Monitor not found

### Restore Soft-Deleted Monitor
**PATCH** `/api/v1/monitors/{id}/restore`

**Response (200 OK):**
```json
{
  "id": "880e8400-e29b-41d4-a716-446655440003",
  "display_name": "db-prod-01",
  "status": "active",
  "deleted_at": null,
  "updated_at": "2023-12-07T13:00:00Z"
}
```

### Get Monitor Metrics
**GET** `/api/v1/monitors/{id}/metrics`

**Query Parameters:**
- `metric_group` (required) - Metric group: `host.cpu`, `host.memory`, `host.storage`, `net.interface`
- `start_time` (optional) - ISO 8601 timestamp (default: 1 hour ago)
- `end_time` (optional) - ISO 8601 timestamp (default: now)
- `tags` (optional) - JSON object for tag filtering (e.g., `{"core":"0"}`)
- `aggregation` (optional) - Aggregation function: `avg`, `min`, `max`, `sum` (default: raw data)
- `interval` (optional) - Aggregation interval: `1m`, `5m`, `1h` (default: raw data)

**Example:** `GET /api/v1/monitors/{id}/metrics?metric_group=host.cpu&start_time=2023-12-07T11:00:00Z&end_time=2023-12-07T12:00:00Z&tags={"core":"0"}`

**Response (200 OK):**
```json
{
  "monitor_id": "880e8400-e29b-41d4-a716-446655440003",
  "metric_group": "host.cpu",
  "tags": {"core": "0"},
  "start_time": "2023-12-07T11:00:00Z",
  "end_time": "2023-12-07T12:00:00Z",
  "data_points": [
    {
      "timestamp": "2023-12-07T11:00:00Z",
      "val_used": 15.5,
      "val_total": 100.0
    },
    {
      "timestamp": "2023-12-07T11:01:00Z",
      "val_used": 18.2,
      "val_total": 100.0
    }
  ],
  "count": 2
}
```

**Response (200 OK - Aggregated):**
```json
{
  "monitor_id": "880e8400-e29b-41d4-a716-446655440003",
  "metric_group": "host.memory",
  "start_time": "2023-12-07T11:00:00Z",
  "end_time": "2023-12-07T12:00:00Z",
  "aggregation": "avg",
  "interval": "5m",
  "data_points": [
    {
      "timestamp": "2023-12-07T11:00:00Z",
      "val_used": 8589934592,
      "val_total": 17179869184
    },
    {
      "timestamp": "2023-12-07T11:05:00Z",
      "val_used": 8658869043,
      "val_total": 17179869184
    }
  ],
  "count": 12
}
```

---

## Health Checks

### Liveness Probe
**GET** `/health`

**Auth:** None required

**Response (200 OK):**
```json
{
  "status": "ok",
  "timestamp": "2023-12-07T12:00:00Z"
}
```

### Readiness Probe
**GET** `/ready`

**Auth:** None required

**Response (200 OK):**
```json
{
  "status": "ready",
  "timestamp": "2023-12-07T12:00:00Z",
  "checks": {
    "database": "ok",
    "queue": "ok",
    "plugins": "ok"
  }
}
```

**Response (503 Service Unavailable):**
```json
{
  "status": "not_ready",
  "timestamp": "2023-12-07T12:00:00Z",
  "checks": {
    "database": "failed",
    "queue": "ok",
    "plugins": "ok"
  },
  "error": "Database connection refused"
}
```

---

## Error Responses

All error responses follow this format:

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

**Standard Error Codes:**
- `VALIDATION_ERROR` (400) - Request payload validation failed
- `INVALID_REQUEST` (400) - Malformed JSON or missing fields
- `UNAUTHORIZED` (401) - Missing or invalid JWT token
- `FORBIDDEN` (403) - Valid token but insufficient permissions
- `NOT_FOUND` (404) - Resource does not exist
- `CONFLICT` (409) - Resource already exists
- `UNPROCESSABLE_ENTITY` (422) - Business logic validation failed
- `RATE_LIMITED` (429) - Too many requests
- `INTERNAL_ERROR` (500) - Unexpected server error
- `SERVICE_UNAVAILABLE` (503) - Database or dependency unavailable

---

## Rate Limiting

**Future Implementation:** API rate limiting will be implemented with the following limits:
- Authenticated requests: 1000 requests per hour per token
- Discovery runs: 10 concurrent jobs per user
- Metric queries: 100 requests per minute per token

Current version has no rate limiting.
