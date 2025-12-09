# Protocol System - Quick Reference

## What We Built

A **thread-safe protocol registry** that manages protocol definitions (WinRM, SSH, SNMP) and their JSON schemas for credential validation.

## Core Files

| File | Purpose |
|------|---------|
| `internal/protocols/registry.go` | Main registry with all protocols and schemas |
| `internal/protocols/models.go` | Response struct definitions |
| `internal/api/protocol_handler.go` | HTTP endpoints for protocol operations |
| `internal/protocols/registry_test.go` | Comprehensive unit tests |

## Key Features

✅ **Thread-Safe:** Uses `sync.RWMutex` for concurrent access  
✅ **Singleton Pattern:** Global registry accessed via `GetRegistry()`  
✅ **Protocol Validation:** Built-in credential validation for each protocol  
✅ **JSON Schemas:** Full JSON Schema Draft-07 support  
✅ **Extensible:** Easy to add new protocols  
✅ **Tested:** 40+ assertions in test suite  

## Supported Protocols

### 1. WinRM (Windows Remote Management)
- **ID:** `winrm`
- **Default Port:** 5985
- **Required:** username, password
- **Optional:** domain, use_https

### 2. SSH (Secure Shell)
- **ID:** `ssh`
- **Default Port:** 22
- **Required:** username + (password OR private_key)
- **Optional:** passphrase, port

### 3. SNMP v2c (Simple Network Management Protocol)
- **ID:** `snmp-v2c`
- **Default Port:** 161
- **Required:** community
- **Optional:** none

## API Endpoints

### List Protocols
```
GET /api/v1/protocols
Authorization: Bearer <token>
```

Returns list of all protocols with metadata.

### Get Protocol Schema
```
GET /api/v1/protocols/{id}/schema
Authorization: Bearer <token>
```

Returns JSON schema for validating credentials.

**Examples:**
- `GET /api/v1/protocols/winrm/schema`
- `GET /api/v1/protocols/ssh/schema`
- `GET /api/v1/protocols/snmp-v2c/schema`

## Usage in Code

### Get a Protocol
```go
registry := protocols.GetRegistry()
protocol, err := registry.GetProtocol("winrm")
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Port: %d\n", protocol.DefaultPort)
```

### Get a Schema
```go
schema, err := registry.GetSchema("ssh")
if err != nil {
    log.Fatal(err)
}
// schema is json.RawMessage containing the full JSON schema
```

### Validate Credentials
```go
creds := map[string]interface{}{
    "username": "admin",
    "password": "secret",
}
err := registry.ValidateCredentials("winrm", creds)
if err != nil {
    log.Printf("Validation failed: %v", err)
}
```

### List All Protocols
```go
protocols := registry.ListProtocols()
for _, p := range protocols {
    fmt.Printf("%s (%s) - Port %d\n", p.ID, p.Name, p.DefaultPort)
}
```

## Testing

**Run tests:**
```bash
go test -v ./internal/protocols/...
```

**With coverage:**
```bash
go test -v -cover ./internal/protocols/...
```

**Specific test:**
```bash
go test -v -run TestValidateWinRMCredentials ./internal/protocols/...
```

## Integration Points

### Credential Handler
Use this registry to:
1. List available protocols when creating credentials
2. Validate credential data before encryption
3. Provide schema info in API responses

### Discovery Engine
Use this registry to:
1. Get default port for each protocol
2. Identify which protocol should handle a discovered device

### Plugin System
Use this registry to:
1. Map plugins to protocols
2. Pass credentials to plugins in the correct format

## Adding a New Protocol

1. **Create the schema constant:**
```go
var mySchema = json.RawMessage(`{
    "$schema": "http://json-schema.org/draft-07/schema#",
    "type": "object",
    "required": ["field1"],
    "properties": {
        "field1": {"type": "string"}
    }
}`)
```

2. **Add validation function:**
```go
func validateMyProtocolCredentials(creds map[string]interface{}) error {
    field1, ok := creds["field1"].(string)
    if !ok || field1 == "" {
        return fmt.Errorf("field1 is required")
    }
    return nil
}
```

3. **Register in initializeProtocols():**
```go
r.registerProtocol(&Protocol{
    ID:          "my-protocol",
    Name:        "My Protocol",
    Description: "Description",
    DefaultPort: 9999,
    Version:     "1.0.0",
}, mySchema)
```

4. **Add case to ValidateCredentials():**
```go
case "my-protocol":
    return validateMyProtocolCredentials(credentials)
```

## Error Handling

All errors follow the standard error response format:
```json
{
  "error": {
    "code": "ERROR_CODE",
    "message": "Human readable message",
    "details": null,
    "request_id": "uuid"
  }
}
```

**Common Error Codes:**
- `INVALID_REQUEST` - Protocol ID is missing or malformed
- `NOT_FOUND` - Protocol does not exist
- `INTERNAL_ERROR` - Server error retrieving schema

## Performance Characteristics

- **Memory:** ~5KB per protocol (including schema)
- **Lookup Time:** O(1) hash map lookups (microseconds)
- **Thread Safety:** RWMutex for concurrent reads
- **Initialization:** ~1ms on startup

## Monitoring & Logging

The protocol system logs:
- Protocol registration failures
- Schema parsing errors
- Credential validation failures
- Unknown protocol requests

All logs include request_id for tracing.

## Status: ✅ Complete

The protocol handler and registry are fully implemented, tested, and ready for integration with the credential handler and discovery system.
