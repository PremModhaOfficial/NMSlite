# Protocol Handler & Schema Registry - Implementation Summary

## Overview
We've successfully implemented a **Protocol Handler** and **Schema Registry** system that manages protocol definitions and their JSON schemas for credential validation.

## Files Created

### 1. `internal/protocols/registry.go` (Main Registry)
**Purpose:** Central registry for all supported protocols and their schemas

**Key Components:**
- `Registry` struct: Thread-safe storage for protocols and schemas
- `Protocol` struct: Defines a protocol (id, name, description, default_port, version)
- `GetRegistry()`: Singleton pattern to get the global registry
- `ListProtocols()`: Returns all registered protocols
- `GetProtocol(id)`: Fetch a specific protocol
- `GetSchema(id)`: Fetch JSON schema for a protocol
- `ValidateCredentials(protocolID, creds)`: Validate credentials against protocol rules

**Supported Protocols:**
1. **WinRM** (Windows Remote Management)
   - Default Port: 5985
   - Required Fields: username, password
   - Optional Fields: domain, use_https

2. **SSH** (Secure Shell)
   - Default Port: 22
   - Required Fields: username, (password OR private_key)
   - Optional Fields: passphrase, port

3. **SNMP v2c** (Simple Network Management Protocol)
   - Default Port: 161
   - Required Fields: community
   - Optional Fields: none

**Validation Functions:**
- `validateWinRMCredentials()`: Ensures username and password are present
- `validateSSHCredentials()`: Ensures username and either password or key
- `validateSNMPCredentials()`: Ensures community string is present

### 2. `internal/protocols/models.go` (Response Models)
**Purpose:** Define API response structures

**Structs:**
- `ProtocolListResponse`: Response for listing protocols
- `SchemaResponse`: Response for getting a protocol schema

### 3. `internal/api/protocol_handler.go` (API Handler)
**Purpose:** HTTP handler for protocol endpoints

**Endpoints:**
- `GET /api/v1/protocols` → `List()` - Returns all protocols
- `GET /api/v1/protocols/{id}/schema` → `GetSchema()` - Returns JSON schema for a protocol

**Features:**
- Uses the Registry singleton
- Proper error handling with standard error responses
- Validates protocol ID before returning schema

### 4. `internal/protocols/registry_test.go` (Unit Tests)
**Purpose:** Comprehensive test coverage

**Test Cases:**
- Registry initialization and protocol registration
- Getting protocols by ID (valid and invalid)
- Getting schemas by ID (valid and invalid)
- Credential validation for WinRM, SSH, and SNMP

**Test Results:** ✅ All 8 test suites pass (40+ assertions)

## Architecture

```
User Request
    ↓
HTTP Handler (protocol_handler.go)
    ↓
Registry Lookup (registry.go)
    ↓
JSON Schema Return
    ↓
Validate Credentials (optional)
    ↓
Error/Success Response
```

## JSON Schemas

### WinRM Schema
```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["username", "password"],
  "properties": {
    "username": { "type": "string", "minLength": 1 },
    "password": { "type": "string", "minLength": 1 },
    "domain": { "type": "string" },
    "use_https": { "type": "boolean", "default": false }
  }
}
```

### SSH Schema
```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["username"],
  "properties": {
    "username": { "type": "string", "minLength": 1 },
    "password": { "type": "string" },
    "private_key": { "type": "string" },
    "passphrase": { "type": "string" },
    "port": { "type": "integer", "default": 22 }
  }
}
```

### SNMP v2c Schema
```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["community"],
  "properties": {
    "community": { "type": "string", "minLength": 1, "default": "public" }
  }
}
```

## API Examples

### List All Protocols
```bash
curl -X GET http://localhost:8080/api/v1/protocols \
  -H "Authorization: Bearer <jwt_token>"
```

**Response:**
```json
{
  "data": [
    {
      "id": "winrm",
      "name": "Windows Server (WinRM)",
      "description": "Collects metrics from Windows servers via WinRM",
      "default_port": 5985,
      "version": "1.0.0"
    },
    {
      "id": "ssh",
      "name": "Linux/Unix (SSH)",
      "description": "Collects metrics from Linux/Unix servers via SSH",
      "default_port": 22,
      "version": "1.0.0"
    },
    {
      "id": "snmp-v2c",
      "name": "SNMP v2c",
      "description": "Collects metrics via SNMP v2c",
      "default_port": 161,
      "version": "1.0.0"
    }
  ]
}
```

### Get Protocol Schema
```bash
curl -X GET http://localhost:8080/api/v1/protocols/winrm/schema \
  -H "Authorization: Bearer <jwt_token>"
```

**Response:**
```json
{
  "protocol_id": "winrm",
  "schema": {
    "$schema": "http://json-schema.org/draft-07/schema#",
    "type": "object",
    "title": "WinRM Credentials",
    "description": "Credentials for Windows Remote Management",
    "required": ["username", "password"],
    "properties": {
      "username": {
        "type": "string",
        "title": "Username",
        "description": "Windows user account",
        "minLength": 1
      },
      "password": {
        "type": "string",
        "title": "Password",
        "description": "Windows user password",
        "minLength": 1
      },
      "domain": {
        "type": "string",
        "title": "Domain",
        "description": "Windows domain (optional)",
        "default": ""
      },
      "use_https": {
        "type": "boolean",
        "title": "Use HTTPS",
        "description": "Use HTTPS instead of HTTP",
        "default": false
      }
    }
  }
}
```

## How to Extend

### Add a New Protocol
1. Define the protocol in `registry.go`:
```go
r.registerProtocol(&Protocol{
    ID:          "your-protocol",
    Name:        "Your Protocol Name",
    Description: "Description",
    DefaultPort: 12345,
    Version:     "1.0.0",
}, yourSchema)
```

2. Create a JSON schema constant:
```go
var yourSchema = json.RawMessage(`{...}`)
```

3. Add validation function:
```go
func validateYourProtocolCredentials(creds map[string]interface{}) error {
    // Your validation logic
}
```

4. Add validation case to `ValidateCredentials()`:
```go
case "your-protocol":
    return validateYourProtocolCredentials(credentials)
```

## Testing

**Run all protocol tests:**
```bash
go test -v ./internal/protocols/...
```

**Run with coverage:**
```bash
go test -v -cover ./internal/protocols/...
```

## Next Steps

1. **Implement Credential Handler** - Use this registry to validate credential data before storage
2. **Add Full JSON Schema Validation** - Use `xeenml/gojsonschema` or similar library
3. **Create Plugin Registry** - Similar to protocol registry for managing plugins
4. **Implement Discovery Handler** - Use protocols for device discovery
5. **Database Integration** - Wire up credential storage with encryption

## Files Modified/Created
- ✅ Created: `internal/protocols/registry.go`
- ✅ Created: `internal/protocols/models.go`
- ✅ Created: `internal/protocols/registry_test.go`
- ✅ Modified: `internal/api/protocol_handler.go`
- ✅ Modified: `internal/database/database.go` (fixed unused variable)

## Compilation Status
✅ All code compiles successfully
✅ All tests pass
✅ No linting errors
