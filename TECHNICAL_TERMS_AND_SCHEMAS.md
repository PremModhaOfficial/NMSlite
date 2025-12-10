# Technical Terms & Schema Documentation

This document outlines the technical terminologies, their composition, and the data schemas for the production-minded HTTP API server. It adheres to the Motadata AIOps domain concepts.

---

## 1. Credential Profile

**What it is:**
A secure storage entity that holds authentication secrets.
*   **Relationship:** A **Plugin** (e.g., "Windows Server 2019") *requires* a specific **Protocol** (e.g., "WinRM").
*   **Schema Source:** The *Protocol* defines the structure of the secrets (e.g., WinRM needs `username/password`, AWS needs `access_key/secret`).

**How to get the Schema:**
2.  Query the Protocol: `GET /protocols/winrm/schema` -> returns the form fields.

**Schema (JSON):**
```json
{
  "id": "uuid (string)",
  "name": "Production Windows Cluster",
  "description": "Credentials for all Win2019 servers",
  "protocol": "winrm", // The actual schema type used
  "credential_details": {
    // Validated against the 'winrm' protocol schema
    "username": "admin_user",
    "password": "encrypted_secret_payload", 
    "domain": "internal.corp"
  }
}
```

---

## 2. Device Discovery (Profile)

**What it is:**
A definition of a "Scanning Job."
*   **Responsibility:** The **Core Discovery Engine** acts as the orchestrator. It manages the IP iteration and job scheduling.
*   **Delegation:** The Core asks registered **Plugins** to attempt a handshake. If a Plugin succeeds (e.g., "I found a Windows Server!"), the Core provisions the device.

**What it is comprised of:**
*   **Scope/Target:** The IP range, CIDR block, or specific hostname list to scan.
*   **Ports:** A list of TCP/UDP ports to check (e.g., `[22, 5985]`). This filters candidates *before* authentication is attempted.
*   **Credentials:** A Credential Profile ID to attempt during the handshake process.

**Execution Flow:**
1.  **Core** reads the Target Scope (e.g., `192.168.1.0/24`).
2.  **Core** iterates through each IP address.
3.  **Port Check:** Core attempts to connect to the defined `ports`. If closed, skip IP.
4.  **Core** iterates through `credential_profile_ids`.
5.  **Core** invokes `Plugin.Identify(ip, port, credentials)` for compatible plugins.
6.  **Result:** If a Plugin returns "Success", the Device is created.

**Schema (JSON):**
```json
{
  "id": "uuid (string)",
  "name": "Datacenter A - Subnet 10",
  "target_type": "cidr", // Options: "ip_range",  "cidr" , "ip"
  "target_value": "192.168.10.0/24",
  "ports": [22, 5985, 443], // The "Door Knock" list
  "port_scan_timeout_ms": 2000,
  "credential_profile_ids": ["uuid-cred-1", "uuid-cred-2"],
  "last_run_at": "timestamp"
}
```

---

## 3. Monitor Provisioning (Device)

**What it is:**
The authoritative record of a monitored asset. It represents the successful "binding" of a specific physical/virtual asset to a specific Plugin and Credential Profile. Monitors are exclusively provisioned through the Device Discovery process (see Section 2). This is the entity against which metrics are stored.

**What it is comprised of:**
*   **Identity:** Hostname, IP Address, and a system-generated UUID.
*   **Operational Config:** Which Plugin to use for polling (e.g., `windows-winrm-plugin`) and which Credential Profile to use for auth.
*   **Persisted State:** Only `status` (`active`, `maintenance`, `down`) is stored in the database - used for maintenance mode and determining which monitors to load into cache.
*   **Cached State:** Runtime fields (`consecutive_failures`, `last_poll_at`, `next_poll_deadline`) exist only in the scheduler's in-memory cache.

**How the Plugin is Selected:**
The association is determined during the **Discovery Phase** (see Section 2).
1.  **Identification:** The Core iterates through plugins, asking each to `Identify` the target IP/Port.
2.  **Binding:** The first plugin to return "Success" is assigned to the device.
3.  **Persistence:** The successful Plugin's ID is stored in the `plugin_id` field. All subsequent polling jobs use this specific plugin.

**State Transitions:**
| From | To | Trigger |
|------|-----|---------|
| `active` | `down` | N consecutive failed polls (configurable via `down_threshold`, default: 3) |
| `down` | `active` | Successful poll |
| `active`/`down` | `maintenance` | Manual API call: `PATCH /monitors/{id}` with `{"status": "maintenance"}` |
| `maintenance` | `active` | Manual API call: `PATCH /monitors/{id}` with `{"status": "active"}` |

**Maintenance Mode:**
*   Suppresses polling for the device.
*   Device is excluded from the scheduler heap while in this state.
*   Use case: Planned downtime, server upgrades.

**Deletion Strategy: Soft Delete**
*   Monitors are never hard-deleted. A `deleted_at` timestamp is set.
*   Historical metrics are retained (referenced by `device_id`).
*   Soft-deleted monitors are excluded from polling and default API queries.
*   Can be restored via `PATCH /monitors/{id}/restore`.

**Schema (JSON):**
```json
{
  "id": "uuid (string)",
  "display_name": "db-prod-01",
  "hostname": "db-prod-01.internal",
  "ip_address": "192.168.10.15",
  "plugin_id": "windows-winrm",
  "credential_profile_id": "uuid-cred-1",
  "discovery_profile_id": "uuid-discovery-1",
  "polling_interval_seconds": 60,
  "port": 15985, // The port on which the device was discovered and is monitored
  "status": "active",
  "deleted_at": null
}
```

**State Management (Database vs Cache vs Derived):**

| Field | Storage | Description |
|-------|---------|-------------|
| `status` | **Database** | Persisted. Values: `active`, `maintenance`, `down`. Required for maintenance mode and to know which monitors to load. |
| `consecutive_failures` | **Cache Only** | Runtime counter in scheduler cache. Reset on success, incremented on failure. |
| `last_poll_at` | **Cache Only** | Tracked in scheduler cache for deadline calculation. |
| `next_poll_deadline` | **Cache Only** | Computed as `last_poll_at + polling_interval_seconds`. Used by priority queue. |
| `last_successful_poll_at` | **Derived** | Query from `metrics`: `MAX(timestamp) WHERE device_id = ?` |

**Cache Architecture:**
*   **What's Cached:** Only monitors with `status = 'active'` are loaded into the in-memory scheduler cache.
*   **Cache Invalidation:** The cache is updated when CRUD operations occur on any profile (credentials, discovery, monitors).
*   **On Status Change:** When a monitor's status changes (e.g., to `maintenance` or `down`), it is removed from the cache. When changed back to `active`, it is re-added.
*   **On Restart:** Active monitors are loaded from DB, and `last_poll_at` is derived from the `metrics` table to compute initial deadlines.

**Polling Loop - No DB Writes:**
The automatic polling loop **NEVER writes to the monitors table**. State changes are handled via signals:
*   **Failure Threshold Exceeded:** When `consecutive_failures >= down_threshold`, the poller emits a signal (not a DB write). A separate handler processes this signal and updates `status = 'down'` via the standard CRUD path.
*   **Recovery:** When a previously-down monitor succeeds, a signal is emitted to update `status = 'active'`.

This approach eliminates write amplification from constant state updates during polling.

---

## 4. Metrics (Unified: TimescaleDB with Compression)

**Strategy:**
We utilize **TimescaleDB** (PostgreSQL extension) with a **Single Unified Hypertable** approach. This balances high-volume ingestion with query performance.

1.  **Promoted Columns:** High-volume data uses native `DOUBLE PRECISION` columns (`val_used`, `val_total`) for hardware-accelerated SQL aggregation.
2.  **Row Explosion:** Tabular data (e.g., Network Interfaces, Disks) is stored as multiple rows, distinguished by unique `tags` (acting as the SNMP Index).
3.  **Native Compression:** TimescaleDB automatically compresses older chunks, reducing storage by 90%+.
4.  **Automatic Retention:** Built-in retention policies handle data expiration.

**Metric Group Naming (Hybrid Standard):**
| Metric Group | SNMP MIB Equivalent | Tags Example |
|--------------|---------------------|--------------|
| `host.cpu` | HOST-RESOURCES-MIB::hrProcessorLoad | `{"core": "0"}` |
| `host.memory` | HOST-RESOURCES-MIB::hrStorageUsed (RAM) | `{}` |
| `host.storage` | HOST-RESOURCES-MIB::hrStorageUsed (Disk) | `{"mount": "/", "device": "sda1"}` |
| `net.interface` | IF-MIB::ifTable | `{"interface": "eth0", "direction": "in"}` |
| `host.process` | HOST-RESOURCES-MIB::hrSWRunTable | `{"pid": "1234", "name": "nginx"}` |

**Tags Cardinality & Storage Strategy:**
*   **Architecture:** TimescaleDB with Native Compression enabled.
*   **Handling High Cardinality (e.g., 1000 Disks):**
    *   *Ingestion:* The Go Poller uses the `pgx` COPY Protocol to efficiently stream thousands of rows per second.
    *   *Compression:* A background policy automatically compresses chunks older than 1 hour.
    *   *Segmentation:* Data is segmented by `device_id` and `metric_group`. This ensures all tag rows for a single poll are grouped.
*   **Limit:** No practical limit on unique tags per device due to compression deduplication.

**The Schema:**
```sql
CREATE TABLE metrics (
    timestamp    TIMESTAMPTZ NOT NULL,
    metric_group VARCHAR(50) NOT NULL, -- e.g., 'host.cpu', 'net.interface'
    device_id    UUID NOT NULL,
    tags         JSONB NOT NULL DEFAULT '{}', -- e.g., {"core": "0", "interface": "eth0"}
    
    -- Golden Signals (Native Types for Speed)
    val_used     DOUBLE PRECISION, -- % Usage, Bytes Used, InBits
    val_total    DOUBLE PRECISION, -- Capacity, Link Speed (NULL if gauge)
    
    extra_data   JSONB -- Overflow for rare attributes
);

-- Convert to TimescaleDB hypertable
SELECT create_hypertable('metrics', 'timestamp', chunk_time_interval => INTERVAL '1 day');

-- Compression (segmented for efficiency)
ALTER TABLE metrics SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'device_id, metric_group',
    timescaledb.compress_orderby = 'timestamp DESC'
);
SELECT add_compression_policy('metrics', INTERVAL '1 hour');

-- Retention (configurable via config.yaml: metrics.retention_days)
SELECT add_retention_policy('metrics', INTERVAL '90 days');
```

---

## 5. Plugin / Poller Module

**What it is:**
A self-contained, statically compiled Go binary responsible for fetching data from a specific device type. Plugins are atomic executables with no external dependencies.

**What it is comprised of:**
*   **Binary:** A single executable file (e.g., `./plugins/windows-winrm`)
*   **Manifest:** A `manifest.json` file in the same directory as the binary.

**Manifest Schema (`manifest.json`):**
```json
{
  "id": "windows-winrm",
  "name": "Windows Server (WinRM)",
  "version": "1.0.0",
  "description": "Collects metrics from Windows servers via WinRM",
  "protocol": "winrm",
  "default_port": 5985,
  "supported_metrics": ["host.cpu", "host.memory", "host.storage", "net.interface"],
  "timeout_ms": 10000
}
```

**Plugin Discovery:**
*   **Mechanism:** Directory scan of `plugins.directory` (from config)
*   **Frequency:** On startup + every `plugins.scan_interval_seconds`
*   **Registration:** Core reads `manifest.json` and registers the plugin in memory

**Supported Protocols (Initial):**
| Protocol | Plugin ID | Default Port |
|----------|-----------|--------------|
| WinRM | `windows-winrm` | 5985 |
| SSH | `linux-ssh` | 22 |
| SNMP v2c | `snmp-v2c` | 161 |

---

## 6. Schema Storage & Validation Strategy

**The Challenge:**
The `credential_details` field is dynamic. We need to validate it *before* saving it to the database to ensure "WinRM" profiles actually have a username and password, while "SNMP" profiles have a community string.

**1. Storage (The "Registry"):**
*   **Location:** Application Code (e.g., `internal/protocols/registry.go`).
*   **Mechanism:** A Map linking `protocol_id` → `JSON Schema Definition`.
*   *Why?* Protocols are static standards. They don't need to be edited by users in the DB.

**2. Protocol Schemas:**

**WinRM Schema:**
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

**SSH Schema:**
```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["username"],
  "oneOf": [
    { "required": ["password"] },
    { "required": ["private_key"] }
  ],
  "properties": {
    "username": { "type": "string", "minLength": 1 },
    "password": { "type": "string" },
    "private_key": { "type": "string" },
    "passphrase": { "type": "string" }
  }
}
```

**SNMP v2c Schema:**
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

**3. Validation Flow:**
1.  **Receive:** POST request with `protocol: "winrm"` and a JSON payload.
2.  **Lookup:** Fetch the predefined JSON Schema for `"winrm"` from the Registry.
3.  **Validate:** Compare the incoming JSON payload against the Schema.
    *   *If Invalid:* Return `400 Bad Request` with validation details.
    *   *If Valid:* Encrypt and save to the `credential_profiles` table.

**4. Encryption Order:**
```
[API Input] → [Schema Validation] → [Encrypt] → [Store in DB]
[DB Read] → [Decrypt] → [Return to Plugin via STDIN]
```

---

## 7. Polling Architecture (Dispatcher-Worker Pattern)

**Strategy:**
We use a **"Pull-Based" Scheduler** with a **"Worker Pool"** executor. This avoids creating a persistent goroutine for every device (which doesn't scale) and decoupling scheduling from execution.

**Critical Design Principle: No DB Writes in Polling Loop**
The automatic polling loop **NEVER writes to the database**. All state changes are:
1. Tracked in the in-memory cache
2. Communicated via signals to a separate handler
3. Persisted through the standard CRUD layer (which also invalidates/updates the cache)

**1. Components:**
*   **In-Memory Scheduler Cache:** A `sync.Map` holding only `active` monitors with their runtime state (`consecutive_failures`, `last_poll_at`, `next_poll_deadline`).
*   **Priority Queue (Min-Heap):** Orders cached monitors by `next_poll_deadline`. Polled every **10 seconds** to extract due tasks.
*   **Liveness Gatekeeper:** A high-concurrency pool (configurable via `liveness_pool_size`, default: 500) with a short timeout (configurable via `liveness_timeout_ms`) to filter out down devices.
*   **Job Queue:** A buffered channel (`chan PollingTask`) for heavy plugin tasks.
*   **Worker Pool:** A fixed number of goroutines (configurable via `worker_pool_size`, default: 50) that execute the plugins.
*   **State Signal Channel:** A channel for emitting state change signals (e.g., `monitor.down`, `monitor.recovered`).
*   **Batch Writer:** A dedicated service that aggregates metrics and bulk-inserts them to `metrics` only.

**2. Liveness Check (TCP SYN):**
*   **Method:** TCP SYN probe to the plugin's primary port (e.g., 5985 for WinRM, 22 for SSH).
*   **Why TCP SYN?** Avoids requiring root privileges (unlike ICMP) and verifies the actual service port is open.
*   **Implementation:** `net.DialTimeout("tcp", ip:port, liveness_timeout_ms)`
*   **Timeout:** Configurable via `liveness_timeout_ms` (default: 2000ms).

**3. Why a Worker Pool? (vs. Raw Goroutines)**
*   **Resource Control:** Caps concurrent heavy tasks (Plugins).
*   **Backpressure:** Prevents system crash under load.

**4. Execution Flow:**

1.  **Schedule Check (Every 10 Seconds):**
    *   Poll the Priority Queue (Min-Heap).
    *   While `NextPollDeadline <= NOW()`:
        *   Pop task from heap.
        *   **Reschedule in Cache:** `NextPollDeadline = NOW() + Interval`. Push back to Heap.
        *   **Accumulate:** Add task to a local **Batch Buffer** (configurable via `liveness_batch_size`, default: 50).
        *   **Flush:** When buffer full (or every `batch_flush_interval_ms`, default: 100ms), push `Batch` to `PingQueue`.

2.  **Batch Gatekeeper (Liveness):**
    *   **Action:** Worker picks up a `Batch` of IPs.
    *   **Check:** TCP SYN probe all targets concurrently (timeout: `liveness_timeout_ms`).
    *   **Filter:**
        *   **Qualified (UP):** Push to `JobQueue` for Plugin collection.
        *   **Disqualified (DOWN):** Increment `consecutive_failures` **in cache only**.
    *   **Threshold Check:** If `consecutive_failures >= down_threshold`:
        *   Emit signal to `StateSignalChannel`: `{type: "monitor.down", monitor_id: "..."}`
        *   **No direct DB write** - signal handler processes this asynchronously.

3.  **Execute (Plugin Worker):**
    *   Worker pops `task` from `JobQueue`.
    *   **Context Timeout:** Creates a context with a hard deadline (`plugin_timeout_ms`, default: 10000ms).
    *   **Plugin Call:** `subprocess.Run("./plugins/{plugin_id}", stdin=JSON)`.

4.  **Result Handling (Cache Updates Only):**
    *   **Success:** 
        *   Reset `consecutive_failures` to 0 **in cache**.
        *   Update `last_poll_at` **in cache**.
        *   Send metrics to `ResultQueue`.
        *   If monitor was previously down, emit signal: `{type: "monitor.recovered", monitor_id: "..."}`
    *   **Failure:** 
        *   Increment `consecutive_failures` **in cache**.
        *   Log error. Retry on next scheduled interval only.

5.  **State Signal Handler (Separate Goroutine):**
    *   Consumes from `StateSignalChannel`.
    *   Processes state transitions via the standard CRUD layer:
        *   `monitor.down` → Updates `status = 'down'` in DB, removes from cache.
        *   `monitor.recovered` → Updates `status = 'active'` in DB, re-adds to cache.
    *   CRUD layer handles cache invalidation automatically.

6.  **Storage (Batch Writer):**
    *   Consumes from `ResultQueue`.
    *   Buffers metrics until `metric_batch_size` reached OR `metric_flush_interval_ms` elapsed.
    *   Uses PostgreSQL `COPY` protocol (via `pgx`) for high-throughput bulk insert to `metrics` **only**.
    *   TimescaleDB handles compression of chunks older than `compression_after` (default: 1 hour).

**5. Cache Lifecycle:**

```
[Startup]
    |
    v
Load active monitors from DB (WHERE status = 'active' AND deleted_at IS NULL)
    |
    v
For each monitor: derive last_poll_at from metrics (MAX timestamp)
    |
    v
Compute next_poll_deadline = last_poll_at + polling_interval_seconds
    |
    v
Insert into Priority Queue
    |
    v
[Normal Operation - Every 10s poll the queue]
    |
    v
[CRUD Operation on any profile?] ---> Invalidate/Update cache
```

**6. Cache Invalidation Triggers:**

| Operation | Cache Action |
|-----------|--------------|
| Create Monitor (status=active) | Add to cache with computed deadline |
| Update Monitor config | Update cache entry, recompute deadline if interval changed |
| Update Monitor status → maintenance/down | Remove from cache |
| Update Monitor status → active | Add to cache |
| Delete Monitor (soft) | Remove from cache |
| Restore Monitor | Add to cache if status=active |
| Update Credential Profile | Refresh affected monitors in cache |
| Delete Credential Profile | Remove affected monitors from cache |
| Update Discovery Profile | No cache impact (discovery is separate from polling) |
## 8. Plugin Architecture (Pure CLI + Batching)

**Strategy:**
Plugins are **Ephemeral One-Shot CLIs** that follow the Unix Philosophy. They are executed per job (or per batch of jobs), utilizing `STDIN`/`STDOUT` for secure communication.
*   **Reasoning:** Minimizes memory footprint (no idle daemons) and simplifies plugin development.
*   **Optimization:** To avoid `fork()` overhead, plugins accept **Batched Targets** in a single execution.

**1. The Contract (Secure Stream):**
*   **Execution:** `subprocess.Run("./plugins/windows-plugin")`
*   **Input (STDIN):** A JSON Array of tasks.
    *   *Security:* Credentials are passed via STDIN, never via CLI args or Envars.
*   **Output (STDOUT):** A JSON Array of results (NDJSON is also acceptable).

**2. Input Example (STDIN):**
```json
[
  {
    "request_id": "req_1",
    "target": "192.168.1.10",
    "credentials": {"username": "admin", "password": "..."}
  },
  {
    "request_id": "req_2",
    "target": "192.168.1.11",
    "credentials": {"username": "admin", "password": "..."}
  }
]
```

**3. Output Example (STDOUT):**
```json
[
  {
    "request_id": "req_1",
    "status": "success",
    "timestamp": "2023-12-07T12:00:00Z",
    "metrics": [
      {
        "metric_group": "host.cpu",
        "tags": {"core": "0"},
        "val_used": 15.5,
        "val_total": 100.0
      },
      {
        "metric_group": "host.memory",
        "tags": {},
        "val_used": 8589934592,
        "val_total": 17179869184
      },
      {
        "metric_group": "host.storage",
        "tags": {"mount": "C:", "device": "disk0"},
        "val_used": 107374182400,
        "val_total": 536870912000
      },
      {
        "metric_group": "net.interface",
        "tags": {"interface": "Ethernet0", "direction": "in"},
        "val_used": 1234567890,
        "val_total": 1000000000
      }
    ]
  },
  {
    "request_id": "req_2",
    "status": "failed",
    "error": "Connection timeout"
  }
]
```

**4. Core Logic:**

The core orchestrator is responsible for:
1.  Scanning the plugin directory for available plugins
2.  Matching tasks to plugins based on `plugin_id`
3.  Batching tasks destined for the same plugin
4.  Executing the plugin binary with batched input
5.  Parsing output and routing metrics to the Batch Writer

**Execution Pseudocode:**
```go
func executePluginBatch(pluginID string, tasks []PollingTask) {
    // 1. Build input payload
    input := make([]PluginInput, len(tasks))
    for i, task := range tasks {
        input[i] = PluginInput{
            RequestID:   task.ID,
            Target:      task.IPAddress,
            Port:        task.Port,
            Credentials: decrypt(task.CredentialData),
        }
    }
    
    // 2. Execute plugin with timeout
    ctx, cancel := context.WithTimeout(ctx, pluginTimeout)
    defer cancel()
    
    cmd := exec.CommandContext(ctx, pluginPath)
    cmd.Stdin = json.NewEncoder(inputJSON)
    output, err := cmd.Output()
    
    // 3. Handle timeout
    if ctx.Err() == context.DeadlineExceeded {
        // SIGKILL already sent by CommandContext
        log.Error("plugin timeout", "plugin", pluginID)
        markTasksFailed(tasks)
        return
    }
    
    // 4. Parse output and route metrics
    var results []PluginOutput
    json.Unmarshal(output, &results)
    
    for _, result := range results {
        if result.Status == "success" {
            metricsQueue <- result.Metrics
            resetFailureCount(result.RequestID)
        } else {
            incrementFailureCount(result.RequestID)
        }
    }
}
```

---

## 9. Core Database Schema

These tables manage the configuration and state of the system.

**1. Table: `credential_profiles`**
```sql
CREATE TABLE credential_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    protocol VARCHAR(50) NOT NULL, -- e.g., 'winrm', 'ssh', 'snmp-v2c'
    credential_data JSONB NOT NULL, -- Encrypted secrets (AES-256-GCM)
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ DEFAULT NULL -- Soft delete
);

CREATE INDEX idx_credential_profiles_protocol ON credential_profiles(protocol);
CREATE INDEX idx_credential_profiles_deleted_at ON credential_profiles(deleted_at) WHERE deleted_at IS NULL;
```

**2. Table: `discovery_profiles`**
```sql
CREATE TABLE discovery_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    target_type VARCHAR(50) NOT NULL, -- 'cidr', 'range', 'ip'
    target_value TEXT NOT NULL,
    ports JSONB NOT NULL, -- e.g. [22, 5985]
    port_scan_timeout_ms INT DEFAULT 1000,
    credential_profile_ids JSONB NOT NULL, -- Array of UUIDs: ["id1", "id2"]
    last_run_at TIMESTAMPTZ,
    last_run_status VARCHAR(50), -- 'success', 'partial', 'failed'
    devices_discovered INT DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ DEFAULT NULL -- Soft delete
);

CREATE INDEX idx_discovery_profiles_deleted_at ON discovery_profiles(deleted_at) WHERE deleted_at IS NULL;
```

**3. Table: `monitors` (Provisioned Devices)**
```sql
CREATE TABLE monitors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    display_name VARCHAR(255),
    hostname VARCHAR(255),
    ip_address INET NOT NULL,
    
    -- Configuration
    plugin_id VARCHAR(100) NOT NULL, -- Logic driver (e.g., 'windows-winrm')
    credential_profile_id UUID REFERENCES credential_profiles(id),
    discovery_profile_id UUID REFERENCES discovery_profiles(id) ON DELETE RESTRICT,
    polling_interval_seconds INT DEFAULT 60,
    
    -- Persisted State (only status - for maintenance mode and cache loading)
    status VARCHAR(50) DEFAULT 'active', -- 'active', 'maintenance', 'down'
    
    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    deleted_at TIMESTAMPTZ DEFAULT NULL -- Soft delete
);

-- Runtime state (consecutive_failures, last_poll_at, next_poll_deadline) exists
-- ONLY in the scheduler cache. The polling loop NEVER writes to this table.
-- Status changes are handled via signals processed through the CRUD layer.

-- Performance indexes
CREATE INDEX idx_monitors_ip_address ON monitors(ip_address);
CREATE INDEX idx_monitors_status ON monitors(status) WHERE deleted_at IS NULL;
CREATE INDEX idx_monitors_plugin_id ON monitors(plugin_id);
CREATE INDEX idx_monitors_deleted_at ON monitors(deleted_at) WHERE deleted_at IS NULL;
-- Index for loading active monitors into cache on startup
CREATE INDEX idx_monitors_active ON monitors(id) WHERE status = 'active' AND deleted_at IS NULL;
```

**4. Metric Tables (TimescaleDB Hypertable)**
```sql
-- 1. Master Table (TimescaleDB Hypertable)
CREATE TABLE metrics (
    timestamp TIMESTAMPTZ NOT NULL,
    metric_group VARCHAR(50) NOT NULL, -- e.g., 'host.cpu', 'host.memory', 'net.interface'
    device_id UUID NOT NULL,
    tags JSONB NOT NULL DEFAULT '{}', -- e.g., {"core": "0", "mount": "/", "interface": "eth0"}
    val_used DOUBLE PRECISION,
    val_total DOUBLE PRECISION,
    extra_data JSONB
);

-- Convert to TimescaleDB hypertable (chunk interval: 1 day)
SELECT create_hypertable('metrics', 'timestamp', chunk_time_interval => INTERVAL '1 day');

-- Indexes for query performance
CREATE INDEX idx_metrics_device_time ON metrics(device_id, timestamp DESC);
CREATE INDEX idx_metrics_group_time ON metrics(metric_group, timestamp DESC);
CREATE INDEX idx_metrics_tags ON metrics USING GIN (tags);

-- Compression policy (compress chunks older than 1 hour)
ALTER TABLE metrics SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'device_id, metric_group',
    timescaledb.compress_orderby = 'timestamp DESC'
);
SELECT add_compression_policy('metrics', INTERVAL '1 hour');

-- Retention policy (configurable, default 90 days)
SELECT add_retention_policy('metrics', INTERVAL '90 days');
```

**5. Supported Metric Groups (Initial Scope):**
| Metric Group | Tags | Description | Scope |
|--------------|------|-------------|-------|
| `host.cpu` | `{"core": "0"}` | CPU utilization per core | v1.0 |
| `host.memory` | `{}` | System memory usage | v1.0 |
| `host.storage` | `{"mount": "/", "device": "sda1"}` | Disk usage per mount point | v1.0 |
| `net.interface` | `{"interface": "eth0"}` | Network interface statistics | v1.0 |
| `host.process` | `{"pid": "1234", "name": "nginx"}` | Process-level metrics | Future |

---

## 10. Configuration Strategy

**Source:**
The application reads from `config.yaml` at startup. Values can be overridden by Environment Variables (prefix `NMS_`). Command-line flags take highest precedence.

**Precedence Order:** CLI Flags > Environment Variables > config.yaml > Defaults

**Startup Validation:**
The application validates all required configuration at startup and fails fast with a clear error message if any required values are missing or invalid.

**Structure (`config.yaml`):**

```yaml
# =============================================================================
# NMS Lite Configuration
# =============================================================================
# Environment variable override: NMS_<SECTION>_<KEY> (e.g., NMS_SERVER_PORT)
# =============================================================================

server:
  host: "0.0.0.0"
  port: 8080
  read_timeout_ms: 30000
  write_timeout_ms: 30000

# TLS Configuration (Required for production)
tls:
  enabled: false
  cert_file: "/etc/nms/certs/server.crt"
  key_file: "/etc/nms/certs/server.key"

# CORS Configuration
cors:
  enabled: true
  allowed_origins: ["http://localhost:3000"] # Use ["*"] for development only
  allowed_methods: ["GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"]
  allowed_headers: ["Authorization", "Content-Type"]
  max_age_seconds: 3600

database:
  host: "localhost"
  port: 5432
  user: "postgres"
  password: "password"           # Env Override: NMS_DATABASE_PASSWORD
  dbname: "nms_lite"
  ssl_mode: "disable"            # Options: disable, require, verify-ca, verify-full
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime_minutes: 30

# Authentication & Security
auth:
  admin_username: "admin"
  admin_password: "changeme"     # Env Override: NMS_AUTH_ADMIN_PASSWORD (REQUIRED)
  jwt_secret: ""                 # Env Override: NMS_AUTH_JWT_SECRET (REQUIRED, 32+ chars)
  jwt_expiry_hours: 24           # Token validity duration
  encryption_key: ""             # Env Override: NMS_AUTH_ENCRYPTION_KEY (REQUIRED, 32 bytes for AES-256)

# Poller Configuration
poller:
  worker_pool_size: 50           # Concurrent plugin executions
  liveness_pool_size: 500        # Concurrent TCP SYN checks
  liveness_timeout_ms: 2000      # TCP SYN probe timeout
  liveness_batch_size: 50        # IPs per liveness batch
  batch_flush_interval_ms: 100   # Max wait before flushing batch
  plugin_timeout_ms: 10000       # Plugin execution timeout
  down_threshold: 3              # Consecutive failures before marking 'down'

# Metrics Storage
metrics:
  batch_size: 100                # Metrics buffered before bulk insert
  flush_interval_ms: 1000        # Max wait before flushing metrics
  retention_days: 90             # Data retention period
  compression_after_hours: 1     # TimescaleDB compression delay

# Discovery Configuration  
discovery:
  max_discovery_workers: 100     # Concurrent IP scans during discovery
  default_port_timeout_ms: 1000  # TCP port check timeout

# Plugin Configuration
plugins:
  directory: "./plugins"         # Plugin binary directory
  scan_interval_seconds: 60      # How often to scan for new plugins

# Logging
logging:
  level: "info"                  # Options: debug, info, warn, error
  format: "json"                 # Options: json, text
  output: "stdout"               # Options: stdout, stderr, file
  file_path: "/var/log/nms/nms.log"  # Only used if output=file

# Queue Configuration (Message Broker)
queue:
  type: "channel"                # Go channels (in-process)
  buffer_size: 1000              # Channel buffer size

# Cache Configuration
cache:
  type: "memory"                 # In-memory cache (sync.Map based)
  ttl_seconds: 300               # Default cache TTL
  max_size_mb: 100               # In-memory cache size limit
```

**Queue Choice Justification: Go Channels**

We use **Go's native buffered channels** as our message queue for the following reasons:

| Criteria | Go Channels | External MQ (NATS/Redis) |
|----------|-------------|--------------------------|
| **Simplicity** | Built-in, zero dependencies | Requires separate service |
| **Latency** | Nanoseconds (in-process) | Microseconds (network hop) |
| **Deployment** | Single binary | Multiple processes |
| **Persistence** | Not needed (metrics are idempotent) | Overkill for our use case |
| **Scalability** | Single-node sufficient | Multi-node (not required) |

**Why not external MQ?**
*   This is a single-node monitoring system, not a distributed one.
*   Metrics collection is idempotent - if the process crashes, the next poll cycle will collect fresh data.
*   No need for message durability or cross-process communication.
*   Reduces operational complexity (no Redis/NATS to maintain).

**Channel Architecture:**
```
[Scheduler] ---> pingQueue (chan []Task) ---> [Liveness Workers]
                                                    |
                                                    v
                                          jobQueue (chan Task) ---> [Plugin Workers]
                                                                          |
                                                                          v
                                                              resultQueue (chan Metrics) ---> [Batch Writer]
```

**Required Environment Variables (Production):**
```bash
# These MUST be set in production
export NMS_AUTH_JWT_SECRET="your-32-char-minimum-secret-key-here"
export NMS_AUTH_ENCRYPTION_KEY="your-32-byte-aes-256-key-here!!"
export NMS_AUTH_ADMIN_PASSWORD="strong-admin-password"
export NMS_DATABASE_PASSWORD="db-password"
```
## 11. Security & Authentication

**Strategy:**
The API is secured using **JWT (JSON Web Tokens)**. Since this is a single-user system, we authenticate against a configured admin password.

**1. Authentication Flow:**
*   **Endpoint:** `POST /api/v1/login`
*   **Request:** `{"username": "admin", "password": "..."}`
*   **Verification:** Checks against the values in `config.yaml` (or `NMS_ADMIN_PASSWORD` env var).
*   **Response:** Returns a signed JWT valid for a specific duration (e.g., 24 hours).

**2. Data Encryption (At Rest):**
*   **Target:** The `credential_data` field in the database.
*   **Algorithm:** AES-256-GCM (Galois/Counter Mode).
*   **Key:** Derived from `NMS_ENCRYPTION_KEY` (32-byte env var).
*   **Implementation:**
    *   *Write:* `Encrypt(json_bytes) -> base64_string`
    *   *Read:* `Decrypt(base64_string) -> json_bytes`
    *   **Note:** The Database *never* sees plaintext passwords.

**3. Authorization Middleware:**
*   All protected routes (e.g., `POST /credentials`, `GET /devices`) require the HTTP Header:
    `Authorization: Bearer <your_jwt_token>`
*   The middleware validates the signature using the `jwt_secret` from the configuration.
*   **Invalid Token:** Returns `401 Unauthorized`.

---

## 12. Error Handling Strategy

**Standard Error Response Schema:**
All API errors follow a consistent JSON structure for easy client-side handling.

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Human-readable error description",
    "details": [
      {
        "field": "ip_address",
        "reason": "Invalid CIDR notation"
      }
    ],
    "request_id": "req_abc123"
  }
}
```

**Error Codes & HTTP Status Mapping:**
| HTTP Status | Error Code | Description |
|-------------|------------|-------------|
| 400 | `VALIDATION_ERROR` | Request payload failed schema validation |
| 400 | `INVALID_REQUEST` | Malformed JSON or missing required fields |
| 401 | `UNAUTHORIZED` | Missing or invalid JWT token |
| 403 | `FORBIDDEN` | Valid token but insufficient permissions |
| 404 | `NOT_FOUND` | Resource does not exist |
| 409 | `CONFLICT` | Resource already exists (e.g., duplicate IP) |
| 422 | `UNPROCESSABLE_ENTITY` | Business logic validation failed |
| 429 | `RATE_LIMITED` | Too many requests (future use) |
| 500 | `INTERNAL_ERROR` | Unexpected server error |
| 503 | `SERVICE_UNAVAILABLE` | Database or dependency unavailable |

**Implementation:**
*   All errors are logged with full context (request_id, user, endpoint).
*   Internal errors (500) never expose stack traces to clients.
*   Each request is assigned a unique `request_id` for tracing.

---

## 13. Logging Strategy

**Library:** Go standard library `log/slog` (Go 1.21+)

**Format:** Structured JSON logs for machine parsing and aggregation.

**Log Levels:**
| Level | Usage |
|-------|-------|
| `debug` | Detailed debugging info (disabled in production) |
| `info` | Normal operational events (startup, shutdown, requests) |
| `warn` | Recoverable issues (retries, deprecation warnings) |
| `error` | Failures requiring attention (DB errors, plugin crashes) |

**Standard Fields:**
```json
{
  "time": "2023-12-07T12:00:00.000Z",
  "level": "INFO",
  "msg": "Request completed",
  "request_id": "req_abc123",
  "method": "GET",
  "path": "/api/v1/monitors",
  "status": 200,
  "duration_ms": 45,
  "user": "admin"
}
```

**Event Logging (For Alerting Integration):**
Critical events are logged with a special `event_type` field for future alerting integration:
```json
{
  "level": "WARN",
  "msg": "Monitor marked as down",
  "event_type": "monitor.down",
  "monitor_id": "uuid-123",
  "ip_address": "192.168.1.10",
  "consecutive_failures": 3
}
```

**Event Types:**
| Event Type | Description |
|------------|-------------|
| `monitor.down` | Monitor transitioned to down state |
| `monitor.recovered` | Monitor transitioned from down to active |
| `discovery.completed` | Discovery job finished |
| `plugin.timeout` | Plugin execution exceeded timeout |
| `plugin.error` | Plugin returned an error |

---

## 14. Health Check Endpoints

**Purpose:** Enable orchestration systems (Kubernetes, load balancers) to determine service health.

**Endpoints:**

**1. Liveness Probe: `GET /health`**
*   **Purpose:** Is the process alive and able to respond?
*   **Auth:** None required (public endpoint)
*   **Response (200 OK):**
```json
{
  "status": "ok",
  "timestamp": "2023-12-07T12:00:00Z"
}
```
*   **Failure:** Returns 503 if the HTTP server is unresponsive.

**2. Readiness Probe: `GET /ready`**
*   **Purpose:** Is the service ready to accept traffic?
*   **Auth:** None required (public endpoint)
*   **Checks:**
    *   Database connection is active
    *   Required queues are accessible
    *   Plugin directory is readable
*   **Response (200 OK):**
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
*   **Response (503 Service Unavailable):**
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

**Kubernetes Example:**
```yaml
livenessProbe:
  httpGet:
    path: /health
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /ready
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 5
```

---

## 15. API Endpoints Summary

**Base URL:** `/api/v1`

**Authentication:**
| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/login` | POST | None | Authenticate and receive JWT |

**Credential Profiles:**
| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/credentials` | GET | JWT | List all credential profiles |
| `/credentials` | POST | JWT | Create new credential profile |
| `/credentials/{id}` | GET | JWT | Get credential profile by ID |
| `/credentials/{id}` | PUT | JWT | Update credential profile |
| `/credentials/{id}` | DELETE | JWT | Soft delete credential profile |

**Discovery Profiles:**
| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/discoveries` | GET | JWT | List all discovery profiles |
| `/discoveries` | POST | JWT | Create new discovery profile |
| `/discoveries/{id}` | GET | JWT | Get discovery profile by ID |
| `/discoveries/{id}` | PUT | JWT | Update discovery profile |
| `/discoveries/{id}` | DELETE | JWT | Soft delete discovery profile |
| `/discoveries/{id}/run` | POST | JWT | Trigger discovery (async, returns job_id) |
| `/discoveries/{id}/jobs/{job_id}` | GET | JWT | Get discovery job status |

**Discovery Job (Async Execution):**

*Request:* `POST /api/v1/discoveries/{id}/run`

*Response (202 Accepted):*
```json
{
  "job_id": "job_uuid_123",
  "status": "running",
  "started_at": "2023-12-07T12:00:00Z",
  "discovery_profile_id": "uuid-discovery-1"
}
```

*Poll Status:* `GET /api/v1/discoveries/{id}/jobs/{job_id}`

*Response (200 OK - In Progress):*
```json
{
  "job_id": "job_uuid_123",
  "status": "running",
  "started_at": "2023-12-07T12:00:00Z",
  "progress": {
    "total_ips": 256,
    "scanned": 128,
    "discovered": 15
  }
}
```

*Response (200 OK - Completed):*
```json
{
  "job_id": "job_uuid_123",
  "status": "completed",
  "started_at": "2023-12-07T12:00:00Z",
  "completed_at": "2023-12-07T12:05:00Z",
  "result": {
    "total_ips": 256,
    "scanned": 256,
    "discovered": 47,
    "failed": 0
  },
  "devices_created": ["uuid-1", "uuid-2", "..."]
}
```

**Monitors (Devices):**
| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/monitors` | GET | JWT | List all monitors (excludes soft-deleted) |
| `/monitors/{id}` | GET | JWT | Get monitor by ID |
| `/monitors/{id}` | PATCH | JWT | Update monitor (including status) |
| `/monitors/{id}` | DELETE | JWT | Soft delete monitor |
| `/monitors/{id}/restore` | PATCH | JWT | Restore soft-deleted monitor |
| `/monitors/{id}/metrics` | GET | JWT | Get metrics for a monitor |

**Protocols:**
| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/protocols` | GET | JWT | List supported protocols |
| `/protocols/{id}/schema` | GET | JWT | Get JSON schema for protocol |

**Health:**
| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/health` | GET | None | Liveness probe |
| `/ready` | GET | None | Readiness probe |
