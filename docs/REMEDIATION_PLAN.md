# NMSlite Remediation Plan

**Created:** December 4, 2025  
**Status:** Pre-Phase 2 Hardening  
**Estimated Total Effort:** 6-8 hours  

---

## Overview

This document outlines step-by-step fixes for critical issues identified during the multi-agent audit. Complete these before starting Phase 2 (Database Integration).

---

## Phase 0: Critical Fixes (Must Complete)

### Task 0.1: Add CredentialID to Device Model

**Priority:** P0 - CRITICAL  
**Effort:** 30 minutes  
**Issue:** Device has no link to Credential - core functionality broken  

**Files to Modify:**
| File | Action |
|------|--------|
| `internal/model/model.go` | Add `CredentialID int` field to `Device` struct |
| `internal/store/mock.go` | Update `CreateDevice()` to accept credential ID |
| `internal/store/mock.go` | Update `UpdateDevice()` to handle credential ID changes |
| `internal/handler/types.go` | Add `CredentialID` to `CreateDeviceRequest` struct |
| `internal/handler/types.go` | Add `CredentialID` to `UpdateDeviceRequest` struct |
| `internal/handler/device.go` | Update `CreateDevice()` handler to use credential ID |
| `internal/handler/device.go` | Update `ProvisionDevice()` to set credential ID on device |

**Validation:**
- [ ] Create device with credential_id in request body
- [ ] GET device shows credential_id in response
- [ ] Provision device updates credential_id

---

### Task 0.2: Fix Nil Pointer in RefreshToken

**Priority:** P0 - CRITICAL  
**Effort:** 10 minutes  
**Issue:** Panic if user ID 1 doesn't exist  

**Files to Modify:**
| File | Action |
|------|--------|
| `internal/handler/auth.go` | Add nil check after `GetUser(1)` call in `RefreshToken()` |

**Validation:**
- [ ] Delete user ID 1 from store, call refresh - should return 401, not panic

---

### Task 0.3: Move Secrets to Environment Variables

**Priority:** P0 - CRITICAL  
**Effort:** 30 minutes  
**Issue:** Hardcoded JWT secret and password in source code  

**Files to Modify:**
| File | Action |
|------|--------|
| `internal/server/server.go` | Read JWT secret from `os.Getenv("JWT_SECRET")` |
| `internal/server/server.go` | Add validation: secret must be 32+ characters |
| `internal/server/server.go` | Provide default only if `ENV=development` |
| `internal/handler/auth.go` | Read mock password from `os.Getenv("MOCK_PASSWORD")` or default to "secret" |
| `cmd/nmslite/main.go` | Remove hardcoded credential printout or gate behind debug flag |
| `.env.example` | Create new file with example environment variables |

**New Files to Create:**
| File | Purpose |
|------|---------|
| `.env.example` | Template for required environment variables |
| `.gitignore` | Add `.env` to prevent committing secrets |

**Validation:**
- [ ] Server fails to start if JWT_SECRET not set (in production mode)
- [ ] Server starts with defaults in development mode
- [ ] Credentials not printed to console in production mode

---

### Task 0.4: Fix Pointer Leak in Store

**Priority:** P0 - CRITICAL  
**Effort:** 1 hour  
**Issue:** Store returns pointers to internal data, allowing mutation outside mutex  

**Files to Modify:**
| File | Action |
|------|--------|
| `internal/store/mock.go` | `GetUser()` - return copy, not pointer to map entry |
| `internal/store/mock.go` | `GetUserByUsername()` - return copy |
| `internal/store/mock.go` | `GetCredential()` - return copy |
| `internal/store/mock.go` | `ListCredentials()` - return slice of copies |
| `internal/store/mock.go` | `GetDevice()` - return copy |
| `internal/store/mock.go` | `ListDevices()` - return slice of copies |
| `internal/store/mock.go` | `GetLatestMetrics()` - return copy |
| `internal/store/mock.go` | `GetMetricsHistory()` - return slice of copies |
| `internal/store/mock.go` | `CreateCredential()` - store copy of input, not input pointer |
| `internal/store/mock.go` | `CreateDevice()` - store copy of input |

**Pattern to Apply:**
```
Before: return s.users[id]
After:  Create copy of struct, return pointer to copy
```

**Validation:**
- [ ] Get device, modify returned struct, get again - original unchanged
- [ ] Run concurrent requests - no race conditions

---

### Task 0.5: Validate Refresh Token

**Priority:** P0 - CRITICAL  
**Effort:** 45 minutes  
**Issue:** Refresh endpoint accepts any token and always returns admin  

**Files to Modify:**
| File | Action |
|------|--------|
| `internal/store/mock.go` | Add `refreshTokens map[string]int` to store (token -> userID) |
| `internal/store/mock.go` | Add `StoreRefreshToken(token string, userID int)` method |
| `internal/store/mock.go` | Add `ValidateRefreshToken(token string) (int, bool)` method |
| `internal/store/mock.go` | Add `RevokeRefreshToken(token string)` method |
| `internal/handler/auth.go` | In `Login()` - store refresh token with user ID |
| `internal/handler/auth.go` | In `RefreshToken()` - validate token, get user ID from store |
| `internal/handler/auth.go` | In `RefreshToken()` - revoke old token, issue new pair |

**Validation:**
- [ ] Login as admin, get refresh token A
- [ ] Use token A to refresh - get new tokens
- [ ] Use token A again - should fail (revoked)
- [ ] Use random token - should fail (invalid)

---

### Task 0.6: Improve Refresh Token Entropy

**Priority:** P0 - CRITICAL  
**Effort:** 15 minutes  
**Issue:** Token generated from `time.Now()` only - predictable  

**Files to Modify:**
| File | Action |
|------|--------|
| `internal/handler/auth.go` | Import `crypto/rand` |
| `internal/handler/auth.go` | Rewrite `generateRefreshToken()` to use crypto/rand |

**Validation:**
- [ ] Generate 1000 tokens in loop - all unique
- [ ] Token format: 32+ random bytes, base64 encoded

---

## Phase 1: High Priority Fixes

### Task 1.1: Enable JWT Middleware

**Priority:** P1 - HIGH  
**Effort:** 1 hour  
**Issue:** All protected endpoints are currently public  

**Files to Modify:**
| File | Action |
|------|--------|
| `internal/middleware/auth.go` | Create new file with JWT validation middleware |
| `internal/server/server.go` | Import middleware package |
| `internal/server/server.go` | Uncomment/replace JWT middleware in protected routes |
| `internal/server/server.go` | Pass JWT secret to middleware |

**New Files to Create:**
| File | Purpose |
|------|---------|
| `internal/middleware/auth.go` | JWT validation middleware |

**Validation:**
- [ ] Request to /api/v1/devices without token - 401
- [ ] Request with invalid token - 401
- [ ] Request with expired token - 401
- [ ] Request with valid token - 200

---

### Task 1.2: Add Request Body Size Limits

**Priority:** P1 - HIGH  
**Effort:** 30 minutes  
**Issue:** No limit on request body size - DoS risk  

**Files to Modify:**
| File | Action |
|------|--------|
| `internal/middleware/bodylimit.go` | Create new file with body limit middleware |
| `internal/server/server.go` | Add body limit middleware to router |

**New Files to Create:**
| File | Purpose |
|------|---------|
| `internal/middleware/bodylimit.go` | Request body size limiter (1MB default) |

**Validation:**
- [ ] Send 2MB JSON body - should return 413 Request Entity Too Large
- [ ] Send normal request - should succeed

---

### Task 1.3: Add Server Timeouts

**Priority:** P1 - HIGH  
**Effort:** 30 minutes  
**Issue:** No read/write timeouts - resource exhaustion risk  

**Files to Modify:**
| File | Action |
|------|--------|
| `internal/server/server.go` | Replace `http.ListenAndServe` with `http.Server{}` |
| `internal/server/server.go` | Set `ReadTimeout: 15 * time.Second` |
| `internal/server/server.go` | Set `WriteTimeout: 15 * time.Second` |
| `internal/server/server.go` | Set `IdleTimeout: 60 * time.Second` |

**Validation:**
- [ ] Slow client times out after 15 seconds
- [ ] Normal requests complete successfully

---

### Task 1.4: Add Graceful Shutdown

**Priority:** P1 - HIGH  
**Effort:** 45 minutes  
**Issue:** Server doesn't handle SIGTERM/SIGINT  

**Files to Modify:**
| File | Action |
|------|--------|
| `cmd/nmslite/main.go` | Import `os/signal` and `syscall` |
| `cmd/nmslite/main.go` | Create signal channel for SIGINT/SIGTERM |
| `cmd/nmslite/main.go` | Run server in goroutine |
| `cmd/nmslite/main.go` | Wait for signal, then call shutdown with timeout |
| `internal/server/server.go` | Add `Shutdown(ctx context.Context) error` method |
| `internal/server/server.go` | Store `*http.Server` reference for shutdown |

**Validation:**
- [ ] Send SIGTERM - server logs "shutting down" and exits cleanly
- [ ] In-flight requests complete before shutdown
- [ ] Shutdown times out after 30 seconds if requests don't complete

---

### Task 1.5: Add CORS Middleware

**Priority:** P1 - HIGH  
**Effort:** 30 minutes  
**Issue:** No CORS - frontend integration blocked  

**Files to Modify:**
| File | Action |
|------|--------|
| `go.mod` | Add `github.com/go-chi/cors` dependency |
| `internal/server/server.go` | Import cors package |
| `internal/server/server.go` | Add CORS middleware with configurable origins |

**Validation:**
- [ ] OPTIONS request returns CORS headers
- [ ] Cross-origin request from allowed origin succeeds
- [ ] Cross-origin request from disallowed origin fails

---

## Phase 2: Medium Priority Fixes

### Task 2.1: Add Input Validation Helpers

**Priority:** P2 - MEDIUM  
**Effort:** 1 hour  

**Files to Modify:**
| File | Action |
|------|--------|
| `internal/handler/validation.go` | Create new file with validation functions |

**New Files to Create:**
| File | Purpose |
|------|---------|
| `internal/handler/validation.go` | IP, subnet, enum validation helpers |

**Validations to Add:**
- `ValidateIP(ip string) error` - validate IP address format
- `ValidateSubnet(cidr string) error` - validate CIDR notation
- `ValidateCredentialType(t string) error` - check against allowed types
- `ValidateDeviceStatus(s string) error` - check against allowed statuses
- `ValidatePollingInterval(i int) error` - check min/max bounds (10-3600)

---

### Task 2.2: Apply Input Validation to Handlers

**Priority:** P2 - MEDIUM  
**Effort:** 45 minutes  

**Files to Modify:**
| File | Action |
|------|--------|
| `internal/handler/device.go` | Use `ValidateIP()` in CreateDevice |
| `internal/handler/device.go` | Use `ValidateSubnet()` in DiscoverDevices |
| `internal/handler/device.go` | Use `ValidateDeviceStatus()` in UpdateDevice |
| `internal/handler/device.go` | Use `ValidatePollingInterval()` in CreateDevice, ProvisionDevice |
| `internal/handler/credential.go` | Use `ValidateCredentialType()` in CreateCredential |

---

### Task 2.3: Fix JSON Encoder Error Handling

**Priority:** P2 - MEDIUM  
**Effort:** 15 minutes  

**Files to Modify:**
| File | Action |
|------|--------|
| `internal/handler/helpers.go` | Check error from `json.NewEncoder().Encode()` |
| `internal/handler/helpers.go` | Log encoding errors |

---

### Task 2.4: Fix Metrics History Decode Error

**Priority:** P2 - MEDIUM  
**Effort:** 15 minutes  

**Files to Modify:**
| File | Action |
|------|--------|
| `internal/handler/metrics.go` | In `GetMetricsHistory()` - properly handle decode errors |
| `internal/handler/metrics.go` | Only allow empty body, reject malformed JSON |

---

## Phase 3: Low Priority Improvements

### Task 3.1: Add Security Headers Middleware

**Priority:** P3 - LOW  
**Effort:** 20 minutes  

**Files to Create:**
| File | Purpose |
|------|---------|
| `internal/middleware/security.go` | Security headers middleware |

**Headers to Add:**
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `X-XSS-Protection: 1; mode=block`
- `Content-Security-Policy: default-src 'self'`

---

### Task 3.2: Add Request ID Middleware

**Priority:** P3 - LOW  
**Effort:** 20 minutes  

**Files to Modify:**
| File | Action |
|------|--------|
| `internal/server/server.go` | Add chi's `middleware.RequestID` |
| `internal/handler/helpers.go` | Include request ID in error responses |

---

### Task 3.3: Change DELETE Response to 204

**Priority:** P3 - LOW  
**Effort:** 15 minutes  

**Files to Modify:**
| File | Action |
|------|--------|
| `internal/handler/credential.go` | `DeleteCredential()` - return 204 No Content |
| `internal/handler/device.go` | `DeleteDevice()` - return 204 No Content |

---

## Checklist Summary

### Phase 0 - Critical (3-4 hours)
- [ ] 0.1 Add CredentialID to Device
- [ ] 0.2 Fix nil pointer in RefreshToken
- [ ] 0.3 Move secrets to environment variables
- [ ] 0.4 Fix pointer leak in store
- [ ] 0.5 Validate refresh token
- [ ] 0.6 Improve refresh token entropy

### Phase 1 - High Priority (3-4 hours)
- [ ] 1.1 Enable JWT middleware
- [ ] 1.2 Add request body size limits
- [ ] 1.3 Add server timeouts
- [ ] 1.4 Add graceful shutdown
- [ ] 1.5 Add CORS middleware

### Phase 2 - Medium Priority (2-3 hours)
- [ ] 2.1 Add input validation helpers
- [ ] 2.2 Apply input validation to handlers
- [ ] 2.3 Fix JSON encoder error handling
- [ ] 2.4 Fix metrics history decode error

### Phase 3 - Low Priority (1 hour)
- [ ] 3.1 Add security headers middleware
- [ ] 3.2 Add request ID middleware
- [ ] 3.3 Change DELETE response to 204

---

## New Files Summary

| File | Phase | Purpose |
|------|-------|---------|
| `.env.example` | 0 | Environment variable template |
| `internal/middleware/auth.go` | 1 | JWT validation middleware |
| `internal/middleware/bodylimit.go` | 1 | Request body size limiter |
| `internal/handler/validation.go` | 2 | Input validation functions |
| `internal/middleware/security.go` | 3 | Security headers middleware |

---

## Testing After Each Phase

After completing each phase, run:

```bash
# 1. Build
go build -o nmslite ./cmd/nmslite

# 2. Run tests (when added)
go test ./...

# 3. Start server
./nmslite

# 4. Run Postman collection
# All 19 endpoints should still pass
```

---

## Definition of Done

Phase 0 complete when:
- [ ] All 6 critical tasks implemented
- [ ] All existing Postman tests pass
- [ ] No hardcoded secrets in codebase
- [ ] `go build` succeeds with no warnings

Ready for Phase 2 (Database) when:
- [ ] Phase 0 complete
- [ ] Phase 1 complete (recommended)
- [ ] Documentation updated
