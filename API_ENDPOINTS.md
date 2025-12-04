# NMSlite Mock API - Endpoint Summary

## Complete API Endpoint Reference

### Base URL
```
http://localhost:8443/api/v1
```

---

## Authentication Endpoints

### Login
**POST** `/auth/login`

Get access and refresh tokens.

**Request Body:**
```json
{
  "username": "admin",
  "password": "secret"
}
```

**Response (200 OK):**
```json
{
  "success": true,
  "data": {
    "access_token": "string",
    "refresh_token": "string",
    "expires_at": "2025-12-04T18:22:41Z"
  }
}
```

---

### Refresh Token
**POST** `/auth/refresh`

Get a new access token using a refresh token.

**Request Body:**
```json
{
  "refresh_token": "string"
}
```

**Response (200 OK):**
```json
{
  "success": true,
  "data": {
    "access_token": "string",
    "refresh_token": "string",
    "expires_at": "2025-12-04T18:22:41Z"
  }
}
```

---

## Credential Endpoints

### List All Credentials
**GET** `/credentials`

**Response (200 OK):**
```json
{
  "success": true,
  "data": [
    {
      "id": 1,
      "name": "Default WinRM Credential",
      "credential_type": "winrm_basic",
      "username": "administrator",
      "domain": "WORKGROUP",
      "port": 5985,
      "use_ssl": false,
      "created_at": "2025-12-04T18:07:39Z",
      "updated_at": "2025-12-04T18:07:39Z"
    }
  ]
}
```

---

### Create Credential
**POST** `/credentials`

**Request Body:**
```json
{
  "name": "Windows Admin",
  "credential_type": "winrm_ntlm",
  "username": "administrator",
  "password": "P@ssw0rd123",
  "domain": "CORP",
  "port": 5985,
  "use_ssl": false
}
```

**Response (201 Created):**
```json
{
  "success": true,
  "data": {
    "id": 2,
    "name": "Windows Admin",
    "credential_type": "winrm_ntlm",
    "username": "administrator",
    "domain": "CORP",
    "port": 5985,
    "use_ssl": false,
    "created_at": "2025-12-04T18:07:41Z",
    "updated_at": "2025-12-04T18:07:41Z"
  }
}
```

---

### Get Credential by ID
**GET** `/credentials/{id}`

**Response (200 OK):**
```json
{
  "success": true,
  "data": {
    "id": 1,
    "name": "Default WinRM Credential",
    "credential_type": "winrm_basic",
    "username": "administrator",
    "domain": "WORKGROUP",
    "port": 5985,
    "use_ssl": false,
    "created_at": "2025-12-04T18:07:39Z",
    "updated_at": "2025-12-04T18:07:39Z"
  }
}
```

---

### Update Credential
**PUT** `/credentials/{id}`

**Request Body:** (all fields optional)
```json
{
  "name": "Updated Name",
  "credential_type": "wmi",
  "username": "newuser",
  "password": "newpass",
  "domain": "NEWDOMAIN",
  "port": 135,
  "use_ssl": true
}
```

**Response (200 OK):**
```json
{
  "success": true,
  "data": {
    "id": 1,
    "name": "Updated Name",
    "credential_type": "wmi",
    "username": "newuser",
    "domain": "NEWDOMAIN",
    "port": 135,
    "use_ssl": true,
    "created_at": "2025-12-04T18:07:39Z",
    "updated_at": "2025-12-04T18:07:41Z"
  }
}
```

---

### Delete Credential
**DELETE** `/credentials/{id}`

**Response (200 OK):**
```json
{
  "success": true,
  "data": {
    "message": "Credential deleted"
  }
}
```

---

## Device Endpoints

### List All Devices
**GET** `/devices`

**Response (200 OK):**
```json
{
  "success": true,
  "data": [
    {
      "id": 1,
      "ip": "192.168.1.100",
      "hostname": "SERVER-01",
      "os": "Windows Server 2022",
      "status": "discovered",
      "polling_interval": 60,
      "last_seen": "2025-12-04T18:07:39Z",
      "created_at": "2025-12-04T18:07:39Z",
      "updated_at": "2025-12-04T18:07:39Z"
    }
  ]
}
```

---

### Create Device
**POST** `/devices`

**Request Body:**
```json
{
  "ip": "192.168.1.101",
  "hostname": "SERVER-02",
  "os": "Windows Server 2022",
  "polling_interval": 60
}
```

**Response (201 Created):**
```json
{
  "success": true,
  "data": {
    "id": 2,
    "ip": "192.168.1.101",
    "hostname": "SERVER-02",
    "os": "Windows Server 2022",
    "status": "discovered",
    "polling_interval": 60,
    "last_seen": "2025-12-04T18:07:41Z",
    "created_at": "2025-12-04T18:07:41Z",
    "updated_at": "2025-12-04T18:07:41Z"
  }
}
```

---

### Get Device by ID
**GET** `/devices/{id}`

**Response (200 OK):**
```json
{
  "success": true,
  "data": {
    "id": 1,
    "ip": "192.168.1.100",
    "hostname": "SERVER-01",
    "os": "Windows Server 2022",
    "status": "discovered",
    "polling_interval": 60,
    "last_seen": "2025-12-04T18:07:39Z",
    "created_at": "2025-12-04T18:07:39Z",
    "updated_at": "2025-12-04T18:07:39Z"
  }
}
```

---

### Update Device
**PUT** `/devices/{id}`

**Request Body:** (all fields optional)
```json
{
  "hostname": "SERVER-01-UPDATED",
  "os": "Windows Server 2022 Datacenter",
  "status": "provisioned",
  "polling_interval": 120
}
```

**Response (200 OK):**
```json
{
  "success": true,
  "data": {
    "id": 1,
    "ip": "192.168.1.100",
    "hostname": "SERVER-01-UPDATED",
    "os": "Windows Server 2022 Datacenter",
    "status": "provisioned",
    "polling_interval": 120,
    "last_seen": "2025-12-04T18:07:41Z",
    "created_at": "2025-12-04T18:07:39Z",
    "updated_at": "2025-12-04T18:07:41Z"
  }
}
```

---

### Delete Device
**DELETE** `/devices/{id}`

**Response (200 OK):**
```json
{
  "success": true,
  "data": {
    "message": "Device deleted"
  }
}
```

---

### Discover Devices in Subnet
**POST** `/devices/discover`

Discover Windows machines in a network subnet (mock implementation).

**Request Body:**
```json
{
  "subnet": "192.168.1.0/24"
}
```

**Response (200 OK):**
```json
{
  "success": true,
  "data": {
    "subnet": "192.168.1.0/24",
    "count": 3,
    "discovered": [
      {
        "ip": "192.168.1.101",
        "hostname": "SERVER-02",
        "os": "Windows Server 2022"
      },
      {
        "ip": "192.168.1.102",
        "hostname": "SERVER-03",
        "os": "Windows Server 2019"
      },
      {
        "ip": "192.168.1.103",
        "hostname": "WORKSTATION-01",
        "os": "Windows 10"
      }
    ]
  }
}
```

---

### Provision Device for Monitoring
**POST** `/devices/{id}/provision`

Enable monitoring on a device by associating a credential.

**Request Body:**
```json
{
  "credential_id": 1,
  "polling_interval": 60
}
```

**Response (200 OK):**
```json
{
  "success": true,
  "data": {
    "device_id": 1,
    "status": "provisioned",
    "message": "Device provisioned for monitoring"
  }
}
```

---

### Deprovision Device
**POST** `/devices/{id}/deprovision`

Disable monitoring on a device.

**Response (200 OK):**
```json
{
  "success": true,
  "data": {
    "device_id": 1,
    "status": "discovered",
    "message": "Device deprovisioned"
  }
}
```

---

## Metrics Endpoints

### Get Latest Metrics
**GET** `/devices/{id}/metrics`

Get the most recent metrics for a device.

**Response (200 OK):**
```json
{
  "success": true,
  "data": {
    "device_id": 1,
    "timestamp": "2025-12-04T18:07:41Z",
    "cpu": {
      "usage_percent": 45.5
    },
    "memory": {
      "total_bytes": 17179869184,
      "used_bytes": 8589934592,
      "usage_percent": 50.0
    },
    "disk": {
      "total_bytes": 500107862016,
      "used_bytes": 250053931008,
      "usage_percent": 50.0
    },
    "network": {
      "bytes_sent_per_sec": 1048576,
      "bytes_recv_per_sec": 2097152,
      "utilization_percent": 25.0,
      "packets_sent": 1000000,
      "packets_recv": 2000000,
      "errors": 0,
      "dropped": 5
    },
    "process_count": 142
  }
}
```

---

### Get Metrics History
**POST** `/devices/{id}/metrics/history`

Get historical metrics for a device.

**Request Body:** (optional)
```json
{
  "start_time": "2025-12-03T00:00:00Z",
  "end_time": "2025-12-04T23:59:59Z",
  "limit": 24
}
```

**Response (200 OK):**
```json
{
  "success": true,
  "data": {
    "device_id": 1,
    "count": 1,
    "metrics": [
      {
        "device_id": 1,
        "timestamp": "2025-12-04T18:07:41Z",
        "cpu": {
          "usage_percent": 45.5
        },
        "memory": {
          "total_bytes": 17179869184,
          "used_bytes": 8589934592,
          "usage_percent": 50.0
        },
        "disk": {
          "total_bytes": 500107862016,
          "used_bytes": 250053931008,
          "usage_percent": 50.0
        },
        "network": {
          "bytes_sent_per_sec": 1048576,
          "bytes_recv_per_sec": 2097152,
          "utilization_percent": 25.0,
          "packets_sent": 1000000,
          "packets_recv": 2000000,
          "errors": 0,
          "dropped": 5
        },
        "process_count": 142
      }
    ]
  }
}
```

---

## Health Check Endpoint

### Health Status
**GET** `/health`

Check API health status (no authentication required).

**Response (200 OK):**
```json
{
  "success": true,
  "data": {
    "status": "ok",
    "service": "NMSlite Mock API",
    "version": "1.0.0"
  }
}
```

---

## Error Response Format

All endpoints may return error responses in the following format:

**Response (4xx/5xx):**
```json
{
  "success": false,
  "error": {
    "code": "ERROR_CODE",
    "message": "Human readable error message",
    "details": {
      "field": "value",
      "context": "additional info"
    }
  }
}
```

### Common Error Codes

- `INVALID_REQUEST` - Malformed request body
- `INVALID_ID` - Invalid ID format
- `VALIDATION_ERROR` - Missing required fields
- `NOT_FOUND` - Resource not found
- `INVALID_CREDENTIALS` - Login failed

---

## Testing with curl

### Login
```bash
curl -X POST http://localhost:8443/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"secret"}'
```

### Create Device
```bash
curl -X POST http://localhost:8443/api/v1/devices \
  -H "Content-Type: application/json" \
  -d '{"ip":"192.168.1.101","hostname":"SERVER-02","polling_interval":60}'
```

### List Devices
```bash
curl http://localhost:8443/api/v1/devices
```

### Get Metrics
```bash
curl http://localhost:8443/api/v1/devices/1/metrics
```

### Provision Device
```bash
curl -X POST http://localhost:8443/api/v1/devices/1/provision \
  -H "Content-Type: application/json" \
  -d '{"credential_id":1,"polling_interval":60}'
```
