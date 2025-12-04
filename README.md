# NMSlite Mock API

A complete mock implementation of the NMSlite API following the architecture specification in `docs/arch.md`. All endpoints are implemented with in-memory data storage for rapid prototyping and testing.

## Features

✅ All endpoints from the API specification  
✅ In-memory mock data store  
✅ JWT-like token generation  
✅ CRUD operations for credentials and devices  
✅ Device discovery simulation  
✅ Metrics endpoints (latest & history)  
✅ Health check endpoint  
✅ Standard JSON response envelope  
✅ Error handling with detailed messages  

## Building

```bash
go build -o nmslite ./cmd/nmslite/
```

## Running

```bash
./nmslite
```

The server starts on `http://localhost:8443`

## Quick Start

### 1. Health Check (No auth required)

```bash
curl http://localhost:8443/health
```

**Response:**
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

### 2. Login

```bash
curl -X POST http://localhost:8443/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "admin", "password": "secret"}'
```

**Test Credentials:**
- Username: `admin`
- Password: `secret`

**Response:**
```json
{
  "success": true,
  "data": {
    "access_token": "...",
    "refresh_token": "...",
    "expires_at": "2025-12-03T10:15:00Z"
  }
}
```

### 3. List Devices

```bash
curl http://localhost:8443/api/v1/devices
```

**Response:**
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
      "last_seen": "2025-12-03T10:00:00Z",
      "created_at": "2025-12-03T09:00:00Z",
      "updated_at": "2025-12-03T09:00:00Z"
    }
  ]
}
```

### 4. Create Device

```bash
curl -X POST http://localhost:8443/api/v1/devices \
  -H "Content-Type: application/json" \
  -d '{
    "ip": "192.168.1.101",
    "hostname": "SERVER-02",
    "os": "Windows Server 2022",
    "polling_interval": 60
  }'
```

### 5. Create Credential

```bash
curl -X POST http://localhost:8443/api/v1/credentials \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Windows Admin",
    "credential_type": "winrm_ntlm",
    "username": "administrator",
    "password": "P@ssw0rd123",
    "domain": "CORP",
    "port": 5985,
    "use_ssl": false
  }'
```

### 6. Provision Device

```bash
curl -X POST http://localhost:8443/api/v1/devices/1/provision \
  -H "Content-Type: application/json" \
  -d '{
    "credential_id": 1,
    "polling_interval": 60
  }'
```

### 7. Get Device Metrics

```bash
curl http://localhost:8443/api/v1/devices/1/metrics
```

**Response:**
```json
{
  "success": true,
  "data": {
    "device_id": 1,
    "timestamp": "2025-12-03T10:05:00Z",
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

## API Endpoints

### Authentication

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/v1/auth/login` | Login, get tokens |
| POST | `/api/v1/auth/refresh` | Refresh access token |

### Credentials

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/credentials` | List all credentials |
| POST | `/api/v1/credentials` | Create credential |
| GET | `/api/v1/credentials/{id}` | Get credential by ID |
| PUT | `/api/v1/credentials/{id}` | Update credential |
| DELETE | `/api/v1/credentials/{id}` | Delete credential |

### Devices

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/devices` | List all devices |
| POST | `/api/v1/devices` | Create device |
| GET | `/api/v1/devices/{id}` | Get device by ID |
| PUT | `/api/v1/devices/{id}` | Update device |
| DELETE | `/api/v1/devices/{id}` | Delete device |
| POST | `/api/v1/devices/discover` | Discover devices in subnet |
| POST | `/api/v1/devices/{id}/provision` | Provision monitoring |
| POST | `/api/v1/devices/{id}/deprovision` | Deprovision monitoring |

### Metrics

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/v1/devices/{id}/metrics` | Get latest metrics |
| POST | `/api/v1/devices/{id}/metrics/history` | Get historical metrics |

### Health

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check (no auth) |

## Project Structure

```
NMSlite/
├── cmd/
│   └── nmslite/
│       └── main.go                 # Entry point
├── internal/
│   ├── handler/
│   │   ├── helpers.go             # Response helpers
│   │   ├── types.go               # Request/response types
│   │   ├── auth.go                # Auth endpoints
│   │   ├── credential.go          # Credential CRUD
│   │   ├── device.go              # Device CRUD + provisioning
│   │   ├── metrics.go             # Metrics endpoints
│   │   └── health.go              # Health check
│   ├── model/
│   │   └── model.go               # Data models
│   ├── server/
│   │   └── server.go              # HTTP server setup
│   └── store/
│       └── mock.go                # In-memory mock store
├── go.mod                          # Module definition
└── README.md                       # This file
```

## Response Format

### Success Response

```json
{
  "success": true,
  "data": { /* response data */ }
}
```

### Error Response

```json
{
  "success": false,
  "error": {
    "code": "ERROR_CODE",
    "message": "Human readable error message",
    "details": { /* optional details */ }
  }
}
```

## Mock Data

The API comes with pre-loaded mock data:

**User:**
- ID: 1
- Username: `admin`
- Password: `secret`
- Role: `admin`

**Credential:**
- ID: 1
- Name: Default WinRM Credential
- Type: winrm_basic
- Username: administrator

**Device:**
- ID: 1
- IP: 192.168.1.100
- Hostname: SERVER-01
- OS: Windows Server 2022
- Status: discovered
- Polling Interval: 60s

**Metrics:** Pre-populated with sample metrics for device 1

## Next Steps

After validating the mock API:

1. **Database Integration** - Replace mock store with PostgreSQL
2. **Message Queue** - Integrate NATS JetStream for event-driven polling
3. **Actual Collectors** - Implement WMI/WinRM collectors
4. **Authentication** - Add proper JWT validation middleware
5. **Encryption** - Implement credential encryption (AES-256-GCM)
6. **Testing** - Add comprehensive unit and integration tests

## Dependencies

- `github.com/go-chi/chi/v5` - HTTP router
- `github.com/golang-jwt/jwt/v5` - JWT tokens (for future use)
- `github.com/google/uuid` - UUID generation (for future use)

## License

MIT
