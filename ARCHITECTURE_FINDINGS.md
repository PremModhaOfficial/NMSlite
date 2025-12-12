# NMSlite Codebase Architecture - Discovery & Protocol Implementation

## Current Architecture Overview

The NMSlite system is built on a modular architecture with the following key components:

### 1. DISCOVERY IMPLEMENTATION (`internal/discovery/`)

#### Port Scanning Mechanism
- **Location**: `internal/discovery/worker.go` - `isPortOpen()` function
- **Implementation**: Simple TCP connection check (net.Dialer.DialContext)
- **Protocol**: TCP only (not protocol-specific handshakes)
- **Timeout**: Configurable per discovery profile (stored in `port_scan_timeout_ms`), defaults to 1 second
- **Process**:
  1. Target expansion (CIDR, IP range, or single IP)
  2. For each IP → For each configured port:
     - TCP dial attempt with timeout
     - If successful → Save to `discovered_devices` table
     - If auto_provision enabled → Create monitor

#### Target Expansion
- **Location**: `internal/discovery/network.go`
- Supports three formats:
  - CIDR notation: `192.168.1.0/24` (excludes network/broadcast for IPv4)
  - IP ranges: `192.168.1.1-192.168.1.50`
  - Single IPs: `192.168.1.100`
- Size limit: Maximum 65,536 IPs per expansion
- Includes validation and type detection functions

#### Discovery Profile Structure
Stored in PostgreSQL `discovery_profiles` table:
```sql
- id (UUID)
- name (VARCHAR)
- target_value (TEXT) - Encrypted using AES-256-GCM
- ports (JSONB) - Array of integers [22, 5985]
- port_scan_timeout_ms (INT) - TCP connection timeout
- credential_profile_ids (JSONB) - Array of credential UUIDs
- auto_provision (BOOLEAN) - Auto-create monitors if enabled
- last_run_status (VARCHAR) - 'success', 'partial', 'failed'
- devices_discovered (INT)
```

#### Discovered Devices Table
```sql
discovered_devices:
- id (UUID)
- discovery_profile_id (UUID)
- ip_address (INET)
- hostname (VARCHAR, nullable)
- port (INT) - The open port found
- status (VARCHAR) - 'new', 'provisioned', 'ignored'
```

---

## 2. PROTOCOL SUPPORT & REGISTRY (`internal/protocols/`)

### Registered Protocols
Three protocols currently supported:

#### A. WinRM (Windows Remote Management)
- **ID**: `winrm`
- **Name**: Windows Server (WinRM)
- **Default Port**: 5985 (HTTP) or 5986 (HTTPS)
- **Credentials**: `WinRMCredentials`
  ```go
  {
    "username": string (required),
    "password": string (required),
    "domain": string (optional)
  }
  ```
- **Connection Modes**:
  - Basic Auth: When domain is empty
  - NTLM Auth: When domain is provided

#### B. SSH (Linux/Unix)
- **ID**: `ssh`
- **Name**: Linux/Unix (SSH)
- **Default Port**: 22
- **Credentials**: `SSHCredentials`
  ```go
  {
    "username": string (required),
    "password": string (optional),
    "private_key": string (optional),
    "passphrase": string (optional),
    "port": int (optional)
  }
  ```
- **Validation**: Requires either password OR private_key

#### C. SNMP v2c
- **ID**: `snmp-v2c`
- **Name**: SNMP v2c
- **Default Port**: 161
- **Credentials**: `SNMPCredentials`
  ```go
  {
    "community": string (required)
  }
  ```

### Protocol Registry (`internal/protocols/registry.go`)
- **Type**: Singleton pattern (`GetRegistry()`)
- **Features**:
  - Protocol metadata (ID, name, description, default port, version)
  - Credential type mapping using Go's `reflect` package
  - Validation function: `ValidateCredentials(protocolID, json.RawMessage)` → Returns typed credential struct
  - Thread-safe with RWMutex

---

## 3. PLUGIN ARCHITECTURE (`internal/plugins/`)

### Plugin Registry
- **Location**: `internal/plugins/registry.go`
- **Scanning**: Scans `plugin_bins/` directory on startup
- **Plugin Discovery**: Loads `manifest.json` from each plugin subdirectory
- **Indexing**: Maintains index by plugin ID and by default port
- **Lookup Methods**:
  - `GetByID(id string)` → Single plugin
  - `GetByPort(port int)` → Multiple plugins (port may map to multiple protocols)
  - `GetByProtocol(protocol string)` → Single plugin (expects unique protocol handler)

### Plugin Manifest Structure
```json
{
  "id": "windows-winrm",
  "name": "Windows Server (WinRM)",
  "version": "1.0.0",
  "description": "Collects CPU, memory, disk, and network metrics from Windows servers via WinRM",
  "protocol": "winrm",
  "default_port": 5985,
  "supported_metrics": ["host.cpu", "host.memory", "host.storage", "net.interface"],
  "timeout_ms": 10000
}
```

### Plugin Executor (`internal/plugins/executor.go`)
- **Communication**: Via STDIN/STDOUT (JSON format)
- **Execution**: `exec.CommandContext()` with configurable timeout
- **Input Format** (PollTask):
  ```json
  {
    "request_id": string,
    "target": string (IP or hostname),
    "port": int,
    "credentials": {
      "username": string,
      "password": string,
      "domain": string
    }
  }
  ```
- **Output Format** (PollResult):
  ```json
  {
    "request_id": string,
    "status": "success" | "failed",
    "timestamp": RFC3339 string,
    "metrics": array,
    "error": string (if status is "failed")
  }
  ```

---

## 4. CREDENTIAL MANAGEMENT

### Credential Storage
**Table**: `credential_profiles`
```sql
- id (UUID)
- name (VARCHAR)
- description (TEXT)
- protocol (VARCHAR) - e.g., 'winrm', 'ssh', 'snmp-v2c'
- credential_data (JSONB) - AES-256-GCM encrypted
- created_at, updated_at, deleted_at (soft delete)
```

### Credential Service (`internal/credentials/service.go`)
- **GetDecrypted(ctx, profileID)**: Fetches and decrypts credentials
- **Decryption**: Uses `auth.Service.Decrypt()`
- **Protocol Validation**: Via `protocols.Registry.ValidateCredentials()`
- Returns: `*plugins.Credentials` (username, password, domain, use_https)

### Credential Usage in Discovery
1. Discovery profile contains array of credential_profile_ids (JSON)
2. During auto-provision, first credential in array is used
3. Credentials passed to plugin executor in each PollTask
4. Plugin (WinRM client) establishes connection using provided credentials

---

## 5. PROTOCOL-SPECIFIC IMPLEMENTATIONS

### WinRM Plugin (`plugins/windows-winrm/`)

#### Client Implementation (`winrm/client.go`)
Uses `github.com/masterzen/winrm` library:

**NewClient() Logic**:
1. Creates endpoint with target, port, HTTPS flag, timeout
2. Determines auth method:
   - If `domain != ""` → NTLM Auth with format `DOMAIN\USERNAME`
   - If `domain == ""` → Basic Auth
3. Returns wrapped Client struct

**PowerShell Execution**:
- `RunPowerShell(script)`: Wraps script in `powershell.exe -NoProfile -NonInteractive -Command`
- `RunPowerShellRaw(script)`: Executes raw without wrapping
- Captures stdout, stderr, exit code
- Returns trimmed stdout on success

#### Metric Collection
- **Location**: `collector/collector.go`
- Collects: CPU, Memory, Disk, Network metrics
- Outputs in MetricGroup format with tags (core, mount, interface, etc.)

#### Connection Handshake (No Explicit Validation)
- **Current**: Only checks if TCP port is open (liveness check)
- **Missing**: No protocol-specific handshake or credential validation during discovery
- **Actual Connection**: Happens during polling, not during discovery

---

## 6. CREDENTIAL USAGE DURING DISCOVERY - CURRENT GAP

### Current Process
1. **Discovery Phase**:
   - TCP port scan only (no credentials used)
   - Saves open ports to `discovered_devices` table
   - Status set to "new"

2. **Auto-Provision Phase** (if enabled):
   - Creates monitor with first credential
   - Stores in `monitors` table
   - Updates discovered_devices status to "provisioned"

3. **Polling Phase** (separate):
   - Fetches monitor configuration from database
   - Retrieves and decrypts credentials
   - Executor passes to plugin binary
   - Plugin establishes actual connection and collects metrics

### Key Gap
**Credentials are NOT validated during discovery**. They're only:
- Stored in discovery profile configuration
- Used during auto-provisioning to create monitors
- Validated/used during polling phase

**No Protocol Handshake/Validation During Discovery**:
- Current: Only TCP port liveness check
- Missing: No credential validation against actual protocol endpoints
- Implication: Can create monitors for ports that are open but don't support the protocol

---

## 7. WORKFLOW FLOW DIAGRAM

```
USER INPUT (API)
    ↓
Create Discovery Profile
    ├─ target_value (CIDR/range/IP) → Encrypted
    ├─ ports (array)
    ├─ credentials (array of UUIDs)
    └─ auto_provision (boolean)
    ↓
POST /api/v1/discoveries/{id}/run
    ↓
[DISCOVERY WORKER] (internal/discovery/worker.go)
    ├─ 1. Expand target → List of IPs
    ├─ 2. For each IP/Port pair:
    │   ├─ TCP dial check (1s timeout)
    │   └─ If successful: Save to discovered_devices
    │       ├─ if auto_provision:
    │       │   ├─ Find plugin by port
    │       │   ├─ Create monitor with first credential
    │       │   └─ Update discovered_devices status → "provisioned"
    │       └─ Else: Status remains "new"
    ├─ 3. Publish DiscoveryCompletedEvent
    └─ 4. Update profile last_run_status
    ↓
[POLLER SCHEDULER] (internal/poller/scheduler.go)
    ├─ Loads active monitors from database
    ├─ Schedules polling by interval
    └─ For each monitor:
        ├─ Fetch and decrypt credentials
        ├─ Create PollTask JSON
        ├─ Execute plugin binary (STDIN → STDOUT)
        └─ Parse and store results in metrics table
```

---

## 8. KEY FILES SUMMARY

| File | Purpose |
|------|---------|
| `internal/discovery/network.go` | Target expansion (CIDR, ranges) |
| `internal/discovery/worker.go` | Discovery execution, TCP scanning, auto-provision |
| `internal/protocols/registry.go` | Protocol definitions & credential validation |
| `internal/protocols/credentials.go` | Protocol-specific credential structs (WinRM, SSH, SNMP) |
| `internal/plugins/registry.go` | Plugin discovery & indexing |
| `internal/plugins/executor.go` | Plugin binary execution (STDIN/STDOUT) |
| `internal/credentials/service.go` | Credential storage & decryption |
| `plugins/windows-winrm/winrm/client.go` | WinRM connection implementation |
| `plugins/windows-winrm/collector/collector.go` | Metric collection from Windows |
| `internal/api/discovery_handler.go` | REST API endpoints for discovery |
| `internal/database/migrations/` | Schema for profiles & discovered devices |

---

## 9. DETECTED LIMITATIONS & ARCHITECTURAL ISSUES

1. **No Protocol Handshake Validation During Discovery**
   - Only TCP liveness check
   - Credentials not validated until polling phase
   - Can create monitors for incompatible protocol versions

2. **Credential Association**
   - Single credential array per discovery profile
   - Auto-provision uses only first credential
   - No credential-per-port or credential-per-IP mapping

3. **Plugin Port Mapping**
   - Single default port per plugin
   - Multiple plugins can handle same port (e.g., 5985 for WinRM)
   - No protocol negotiation if multiple plugins on same port

4. **Error Handling in Discovery**
   - TCP timeout is only metric (1 second default)
   - No retry logic
   - No protocol-specific error codes/reasons

5. **Credential Validation Timing**
   - Happens at API creation time (structure validation only)
   - Not tested against actual target until polling
   - No "test connection" during discovery phase
