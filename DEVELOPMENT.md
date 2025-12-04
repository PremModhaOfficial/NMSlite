# NMSlite Mock API - Development Guide

## Overview

This document serves as a guide for developing the NMSlite API from the mock implementation to a production-ready system.

## Current Status: Phase 1 ✅ Complete

All 19 API endpoints are implemented with mock data storage and are fully functional.

## Architecture

```
┌─────────────┐
│   Client    │ (Postman, curl, etc.)
└──────┬──────┘
       │
       │ HTTPS + JSON
       ▼
┌──────────────────────────────────────────────────────┐
│  HTTP Layer (chi router)                             │
│  ├── Auth Handler                                    │
│  ├── Credential Handler                              │
│  ├── Device Handler                                  │
│  ├── Metrics Handler                                 │
│  └── Health Handler                                  │
└──────────┬───────────────────────────────────────────┘
           │
           ▼
┌──────────────────────────────────────────────────────┐
│  Service Layer (future)                              │
│  ├── Auth Service                                    │
│  ├── Credential Service                              │
│  ├── Device Service                                  │
│  ├── Metrics Service                                 │
│  └── Polling Service                                 │
└──────────┬───────────────────────────────────────────┘
           │
           ▼
┌──────────────────────────────────────────────────────┐
│  Repository Layer (future)                           │
│  ├── User Repository                                 │
│  ├── Credential Repository                           │
│  ├── Device Repository                               │
│  └── Metrics Repository                              │
└──────────┬───────────────────────────────────────────┘
           │
           ▼
┌──────────────────────────────────────────────────────┐
│  Storage Layer                                       │
│  ├── Mock Store (current)                            │
│  └── PostgreSQL (future)                             │
└──────────────────────────────────────────────────────┘
```

## Development Roadmap

### Phase 1: Mock API ✅ COMPLETE
- [x] All 19 endpoints implemented
- [x] In-memory data store
- [x] Request/response handling
- [x] Error handling
- [x] Documentation

### Phase 2: Database Integration (Next)
- [ ] Install PostgreSQL dependencies (pgx/v5, golang-migrate)
- [ ] Implement database schema
- [ ] Create migration files
- [ ] Replace mock store with PostgreSQL repositories
- [ ] Add connection pooling configuration
- [ ] Test database operations

**Estimated Effort:** 2-3 days
**Key Files to Create:**
- `internal/repository/user.go`
- `internal/repository/credential.go`
- `internal/repository/device.go`
- `internal/repository/metrics.go`
- `migrations/000001_init_schema.up.sql`
- `migrations/000001_init_schema.down.sql`

**Key Files to Modify:**
- `internal/handler/*` - Update to use services instead of store
- `internal/server/server.go` - Pass database connection to handlers

### Phase 3: Message Queue Integration
- [ ] Add NATS JetStream dependency
- [ ] Implement queue publisher
- [ ] Implement queue consumer/worker
- [ ] Create polling scheduler
- [ ] Implement message handlers for async operations

**Estimated Effort:** 3-4 days
**Key Files to Create:**
- `internal/queue/queue.go` - Interface
- `internal/queue/nats.go` - NATS implementation
- `internal/worker/polling.go` - Polling worker
- `internal/worker/storage.go` - Storage writer

### Phase 4: Real Collectors
- [ ] Implement WMI collector (go-ole)
- [ ] Implement WinRM collector (masterzen/winrm)
- [ ] Create collector registry
- [ ] Add collector tests with mock Windows responses

**Estimated Effort:** 4-5 days
**Key Files to Create:**
- `internal/collector/collector.go` - Interface
- `internal/collector/registry.go` - Registry
- `internal/collector/wmi.go` - WMI implementation
- `internal/collector/winrm.go` - WinRM implementation

### Phase 5: Security Hardening
- [ ] Implement JWT validation middleware
- [ ] Add password hashing (bcrypt)
- [ ] Implement credential encryption (AES-256-GCM)
- [ ] Setup TLS/HTTPS
- [ ] Add rate limiting middleware
- [ ] Input validation and sanitization

**Estimated Effort:** 2-3 days
**Key Files to Modify:**
- `internal/handler/auth.go` - Real JWT implementation
- `internal/server/server.go` - Add middleware
- `internal/crypto/crypto.go` - Encryption utilities

### Phase 6: Testing
- [ ] Unit tests for handlers
- [ ] Unit tests for services
- [ ] Integration tests with database
- [ ] API contract tests
- [ ] Load tests
- [ ] Docker compose for testing environment

**Estimated Effort:** 3-4 days
**Key Files to Create:**
- `internal/handler/*_test.go`
- `internal/service/*_test.go`
- `docker-compose.yml` - Testing environment

## Code Guidelines

### Handler Pattern

```go
// All handlers follow this pattern:
type XyzHandler struct {
    store *store.MockStore  // Replace with service layer
}

func NewXyzHandler(s *store.MockStore) *XyzHandler {
    return &XyzHandler{store: s}
}

func (h *XyzHandler) Create(w http.ResponseWriter, r *http.Request) {
    // 1. Parse request
    var req CreateRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        respondError(w, http.StatusBadRequest, "INVALID_REQUEST", "...")
        return
    }
    
    // 2. Validate
    if req.RequiredField == "" {
        respondError(w, http.StatusBadRequest, "VALIDATION_ERROR", "...")
        return
    }
    
    // 3. Call service
    result, err := h.service.Create(r.Context(), req)
    if err != nil {
        respondError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "...")
        return
    }
    
    // 4. Respond
    respondSuccess(w, http.StatusCreated, result)
}
```

### Response Format

Always use the standard envelope:

```go
// Success
{
  "success": true,
  "data": { /* response */ }
}

// Error
{
  "success": false,
  "error": {
    "code": "ERROR_CODE",
    "message": "Description",
    "details": { /* optional */ }
  }
}
```

## Dependencies to Add Later

```bash
# Database
go get github.com/jackc/pgx/v5
go get -u github.com/golang-migrate/migrate/v4/cmd/migrate

# Queue
go get github.com/nats-io/nats.go
go get github.com/nats-io/nats-server/v2

# Security
go get golang.org/x/crypto/bcrypt
go get golang.org/x/crypto/aes
go get github.com/golang-jwt/jwt/v5

# Collections
go get github.com/masterzen/winrm
go get github.com/go-ole/go-ole

# Validation
go get github.com/go-playground/validator/v10

# Logging
# Using stdlib log/slog (no dependency needed)

# Testing
go get github.com/stretchr/testify/assert
go get github.com/stretchr/testify/require
```

## File Organization Best Practices

```
internal/
├── handler/           # HTTP handlers
│   ├── *_test.go     # Handler tests
│   └── ...
├── service/           # Business logic (Phase 2+)
│   ├── *_test.go
│   └── ...
├── repository/        # Data access (Phase 2+)
│   ├── *_test.go
│   └── ...
├── model/             # Data structures
├── queue/             # Message queue (Phase 3+)
├── collector/         # Metric collectors (Phase 4+)
├── crypto/            # Encryption (Phase 5+)
├── middleware/        # HTTP middleware (Phase 5+)
└── server/            # Server setup
```

## Testing Strategy

### Phase 1-2: Unit Tests
- Handler tests with mock store
- Service tests with mock repository
- Repository tests with test database

### Phase 3-4: Integration Tests
- End-to-end API tests
- Queue consumer tests
- Collector output validation

### Phase 5-6: Performance Tests
- Load tests with multiple concurrent requests
- Database query performance
- Memory profiling

## Local Development Setup

### Requirements
- Go 1.25+
- PostgreSQL 15+ (Phase 2+)
- NATS server (Phase 3+)

### Quick Start
```bash
# Build
go build -o nmslite ./cmd/nmslite/

# Run
./nmslite

# Test endpoints
curl http://localhost:8443/health
```

## Database Migration Example (Phase 2)

```sql
-- migrations/000001_init_schema.up.sql

CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'operator',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- ... more tables from docs/arch.md
```

## Troubleshooting

### Common Issues

1. **Port already in use**
   ```bash
   # Find process using port 8443
   lsof -i :8443
   kill -9 <PID>
   ```

2. **Dependencies not installed**
   ```bash
   go mod tidy
   go mod download
   ```

3. **Build fails**
   ```bash
   go clean
   go mod tidy
   go build ./...
   ```

## Performance Targets (from docs/arch.md)

- API response (p95): < 100ms
- Metrics query (p95): < 500ms
- Poll cycle: < 30s per device
- Concurrent workers: 10-50 configurable
- Target devices: 100-500 per instance

## Code Review Checklist

Before merging:
- [ ] All tests pass
- [ ] Code follows Go conventions
- [ ] Error handling is comprehensive
- [ ] Documentation is updated
- [ ] No hardcoded secrets
- [ ] Logging is appropriate
- [ ] Performance impact assessed

## Resources

- Go idioms: https://effective-go.golang.org/
- chi router: https://github.com/go-chi/chi
- PostgreSQL: https://www.postgresql.org/docs/
- NATS: https://docs.nats.io/
- Security best practices: https://owasp.org/

## Contact & Questions

For implementation details, refer to:
- `docs/arch.md` - Full architecture specification
- `README.md` - Quick start guide
- `API_ENDPOINTS.md` - Complete endpoint reference
