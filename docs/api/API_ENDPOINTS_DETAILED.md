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
    "domain": "internal.corp"
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

### Run Discovery
**POST** `/api/v1/discoveries/{id}/run`

Triggers an asynchronous discovery scan.

**Response (202 Accepted):**
```json
{
  "status": "accepted",
  "message": "Discovery started",
  "profile_id": "660e8400-e29b-41d4-a716-446655440001"
}
```

**Errors:**
- `404 NOT_FOUND` - Discovery profile not found

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

---

## Metrics

### Query Metrics (Batch)
**POST** `/api/v1/metrics/query`

Query metrics for one or more monitors in a single request. Results are grouped by device ID.

**Request:**
```json
{
  "device_ids": [
    "880e8400-e29b-41d4-a716-446655440003",
    "880e8400-e29b-41d4-a716-446655440004"
  ],
  "start": "2023-12-07T11:00:00Z",
  "end": "2023-12-07T12:00:00Z",
  "metric_groups": ["host.cpu", "host.memory"],
  "tag_filters": [
    {"key": "core", "op": "eq", "values": ["0"]}
  ],
  "limit": 100,
  "latest": false
}
```

**Request Fields:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `device_ids` | `uuid[]` | Yes | Array of monitor UUIDs to query |
| `start` | `datetime` | Yes | Start of time range (ISO 8601) |
| `end` | `datetime` | Yes | End of time range (ISO 8601) |
| `metric_groups` | `string[]` | No | Filter by metric groups (e.g., `host.cpu`, `host.memory`) |
| `tag_filters` | `object[]` | No | Tag filters (see operators below) |
| `limit` | `int` | No | Max results per device (default: 100, max: 1000) |
| `latest` | `bool` | No | If true, return only latest value per device/metric_group |

**Tag Filter Operators:**
- `eq` - Equals (single value)
- `in` - In array (multiple values)  
- `like` - SQL LIKE pattern
- `exists` - Tag key exists
- `gt`, `lt`, `gte`, `lte` - Numeric comparisons

**Response (200 OK):**
```json
{
  "data": {
    "880e8400-e29b-41d4-a716-446655440003": [
      {
        "timestamp": "2023-12-07T11:30:00Z",
        "metric_group": "host.cpu",
        "device_id": "880e8400-e29b-41d4-a716-446655440003",
        "tags": {"core": "0"},
        "val_used": 15.5,
        "val_total": 100.0
      }
    ],
    "880e8400-e29b-41d4-a716-446655440004": []
  },
  "count": 1,
  "query": {
    "device_ids": ["880e8400-e29b-41d4-a716-446655440003", "880e8400-e29b-41d4-a716-446655440004"],
    "start": "2023-12-07T11:00:00Z",
    "end": "2023-12-07T12:00:00Z",
    "metric_groups": ["host.cpu", "host.memory"],
    "limit": 100,
    "latest": false
  }
}
```

**Notes:**
- Invalid/non-existent device IDs are silently ignored (won't appear in response)
- Devices with no metrics in the time range return empty array `[]`
- For single device queries, pass one UUID in the array
- `count` is total metrics across all devices

**Errors:**
- `400 INVALID_REQUEST` - `device_ids` array is empty
- `400 INVALID_BODY` - Invalid JSON body
- `500 QUERY_ERROR` - Database query failed

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
