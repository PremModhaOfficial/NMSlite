# NMSlite - Architecture Diagrams & Flows

> **Version:** 2.2.0  
> **Last Updated:** December 2024

---

## Architecture Decisions Summary

| Area | Decision |
|------|----------|
| **Databases** | PostgreSQL (state/CRUD) + TimescaleDB (metrics/time-series) |
| **Metrics Writer** | Separate goroutine consuming from MetricResults channel |
| **Credential Profiles** | Reusable authentication configs; created first, linked by discovery profiles |
| **Discovery Profiles** | Link to credential profiles + define IP range/subnet for scanning |
| **Credential Decryption** | On use by Poller (not at cache time) |
| **Metrics Fields** | Store `total` + `used` only; derive `%` at query time |
| **Cache Scope** | Credential profiles only (not monitors or devices) |
| **Error Handling** | Retry 2x with backoff, then update `last_error` fields |
| **Monitor State** | Two fields: `last_polled` (success) + `last_attempted` (any) |
| **Deletion Rules** | No cascades; must delete monitor before credential profile/device |
| **Discovery Profiles** | Devices remain independent after profile deletion |
| **Plugins** | Metric collectors assigned to monitors; extensible interface |
| **Metrics Retention** | Keep forever (no automatic cleanup) |
| **Alerting** | None (MVP scope) |
| **Authentication** | Stateless JWT; single admin user from env vars |

---

## 1. System Architecture

```mermaid
flowchart TB
    subgraph Client
        User[User / Postman]
    end

    subgraph API Layer
        API[API Server<br/>HTTPS + JWT]
    end

    subgraph Core Services
        Discovery[Discovery Engine]
        Scheduler[Scheduler]
        Poller[Poller Pool]
        Writer[Metrics Writer]
        Cache[Credential Cache]
    end

    subgraph Data Stores
        PG[(PostgreSQL<br/>State DB)]
        TS[(TimescaleDB<br/>Metrics DB)]
    end

    subgraph Targets
        Windows[Windows Targets]
    end

    subgraph Channels
        PollCh{{PollJobs<br/>chan 100}}
        MetricCh{{MetricResults<br/>chan 100}}
        CacheCh{{CacheEvents<br/>chan 50}}
    end

    User -->|HTTPS + JWT| API
    API -->|CRUD| PG
    API -->|Query| TS
    API -->|Run| Discovery

    Discovery -->|Scan| Windows
    Discovery -->|Read Profile| PG
    Discovery -->|Write Devices| PG

    Scheduler -->|Read Monitors| PG
    Scheduler --> PollCh
    PollCh --> Poller

    Poller -->|Get Cached Creds| Cache
    Cache -->|Miss: Fetch + Decrypt| PG
    Poller -->|Collect Metrics| Windows
    Poller --> MetricCh
    Poller -->|Update last_attempted| PG

    MetricCh --> Writer
    Writer -->|Batch Insert| TS
    Writer -->|Update last_polled| PG

    API --> CacheCh
    CacheCh --> Cache
```

---

## 2. Database Architecture

```mermaid
flowchart LR
    subgraph PostgreSQL [PostgreSQL - State DB]
        CREDP[credential_profiles]
        DP[discovery_profiles]
        DEV[devices]
        MON[monitors]
        PLUG[plugins]
    end

    subgraph TimescaleDB [TimescaleDB - Metrics DB]
        MET[metrics<br/>hypertable]
    end

    CREDP -->|linked by| DP
    DP -->|creates| DEV
    CREDP -->|used by| MON
    DEV -->|monitored by| MON
    PLUG -->|assigned to| MON
    MON -->|generates| MET
```

---

## 3. Database Schema

### 3.1 PostgreSQL (State DB)

```mermaid
erDiagram
    credential_profiles {
        int id PK
        string name
        string type
        string username
        bytes encrypted_pass
        string domain
        int port
        bool use_ssl
        timestamp created_at
    }

    discovery_profiles {
        int id PK
        string name
        int credential_profile_id FK
        string subnet
        bool port_scan
        timestamp created_at
    }

    devices {
        int id PK
        string ip
        string hostname
        string os
        int discovery_id FK
        timestamp created_at
    }

    plugins {
        int id PK
        string name
        string type
        string description
        json config
        timestamp created_at
    }

    monitors {
        int id PK
        int device_id FK
        int credential_profile_id FK
        int plugin_id FK
        int polling_interval
        bool enabled
        timestamp last_polled
        timestamp last_attempted
        string last_error
        timestamp last_error_at
        timestamp created_at
    }

    credential_profiles ||--o{ discovery_profiles : "linked by"
    credential_profiles ||--o{ monitors : "used by"
    discovery_profiles ||--o{ devices : creates
    devices ||--o{ monitors : "monitored by"
    plugins ||--o{ monitors : "assigned to"
```

### 3.2 TimescaleDB (Metrics DB)

```mermaid
erDiagram
    metrics {
        int id PK
        int monitor_id FK
        timestamp timestamp
        float cpu_percent
        bigint memory_total
        bigint memory_used
        bigint disk_total
        bigint disk_used
        bigint net_sent_ps
        bigint net_recv_ps
        int process_count
    }
```

> **Note:** Percentage values (`memory_%`, `disk_%`, `net_%`) are derived at query time from `total` and `used` fields.

---

## 4. User Journey

```mermaid
sequenceDiagram
    autonumber
    participant U as User
    participant A as API
    participant PG as PostgreSQL
    participant D as Discovery
    participant W as Windows
    participant S as Scheduler
    participant P as Poller
    participant Wr as Writer
    participant TS as TimescaleDB

    Note over U,TS: Step 1: Authentication
    U->>A: POST /auth/login {username, password}
    A->>A: Validate against env config
    A-->>U: {token: "eyJ..."}

    Note over U,TS: Step 2: Create Credential Profile (reusable auth config)
    U->>A: POST /credential-profiles
    A->>A: Encrypt password
    A->>PG: INSERT credential_profile
    A-->>U: {id: 1, name: "WindowsAdmin"}

    Note over U,TS: Step 3: Create Discovery Profile (links to credential profile)
    U->>A: POST /discovery-profiles {credential_profile_id: 1, subnet: "..."}
    A->>PG: Validate credential_profile exists
    A->>PG: INSERT discovery_profile
    A-->>U: {id: 1, name: "Office", credential_profile_id: 1}

    Note over U,TS: Step 4: Run Discovery (scans network, finds devices)
    U->>A: POST /discovery-profiles/1/run
    A->>D: Run(profile)
    D->>PG: Get linked credential_profile
    D->>W: Scan subnet (ping, ports 135/5985/5986)
    W-->>D: Responses
    D->>PG: INSERT devices
    D-->>A: {found: 5}
    A-->>U: {found: 5, devices: [...]}

    Note over U,TS: Step 5: Provision Monitor (select discovered devices)
    U->>A: POST /monitors {device_id, credential_profile_id, plugin_id, interval}
    A->>PG: Validate device + credential_profile + plugin exist
    A->>PG: INSERT monitor (enabled=true)
    A-->>U: {id: 1, enabled: true}

    Note over U,TS: Step 6: Automatic Polling (Background)
    loop Every tick (10s)
        S->>PG: SELECT monitors WHERE due
        S->>P: chan <- PollJob
        P->>PG: Update last_attempted
        P->>PG: Get plugin for monitor
        P->>W: Collect metrics (via plugin: WMI/WinRM)
        alt Success
            P->>Wr: chan <- MetricResult
            Wr->>TS: Batch INSERT metrics
            Wr->>PG: Update last_polled
        else Failure (retry 2x)
            P->>PG: Update last_error, last_error_at
        end
    end

    Note over U,TS: Step 7: Query Metrics
    U->>A: GET /monitors/1/metrics
    A->>TS: SELECT latest metrics
    A-->>U: {cpu, memory, disk, network}
```

---

## 5. Authentication Flow

```mermaid
sequenceDiagram
    participant U as User
    participant A as API
    participant Env as Config/Env

    U->>A: POST /auth/login<br/>{username, password}
    A->>Env: Get AUTH_USERNAME, AUTH_PASSWORD
    
    alt Credentials Match
        A->>A: Generate JWT<br/>Sign with JWT_SECRET<br/>Set expiry (24h)
        A-->>U: {token: "eyJ...", expires_at: "..."}
    else Credentials Invalid
        A-->>U: 401 Unauthorized
    end

    Note over U,A: Subsequent Requests
    U->>A: GET /monitors<br/>Authorization: Bearer <jwt>
    A->>A: Verify JWT signature<br/>Check expiry
    
    alt Valid Token
        A-->>U: {monitors: [...]}
    else Invalid/Expired
        A-->>U: 401 Unauthorized
    end
```

> **Note:** Single admin user. Credentials from env vars. No users table. No refresh tokens. Fully stateless.

---

## 6. Discovery Flow

```mermaid
sequenceDiagram
    participant U as User
    participant A as API
    participant PG as PostgreSQL
    participant D as Discovery Engine
    participant W as Windows Targets

    U->>A: POST /discovery-profiles/:id/run
    A->>PG: SELECT profile WHERE id = :id
    PG-->>A: Profile {subnet, port_scan, credential_profile_id}
    
    A->>PG: SELECT credential_profile WHERE id = credential_profile_id
    PG-->>A: CredentialProfile {username, encrypted_pass, domain, port}
    
    A->>D: Run(profile, credentialProfile)
    
    loop For each IP in subnet
        D->>W: Ping
        alt Host responds
            D->>W: Port scan (135, 5985, 5986)
            W-->>D: Open ports
            D->>D: Identify as Windows host
        end
    end
    
    D->>PG: INSERT devices (batch)
    D-->>A: {found: N, devices: [...]}
    A-->>U: 200 OK {found: N, devices: [...]}
```

---

## 7. Provisioning Flow

```mermaid
sequenceDiagram
    participant U as User
    participant A as API
    participant PG as PostgreSQL

    U->>A: POST /monitors<br/>{device_id, credential_profile_id, plugin_id, polling_interval}
    
    A->>PG: SELECT device WHERE id = device_id
    alt Device not found
        A-->>U: 404 Device not found
    end
    
    A->>PG: SELECT credential_profile WHERE id = credential_profile_id
    alt Credential Profile not found
        A-->>U: 404 Credential Profile not found
    end
    
    A->>PG: SELECT plugin WHERE id = plugin_id
    alt Plugin not found
        A-->>U: 404 Plugin not found
    end
    
    A->>PG: INSERT monitor<br/>(device_id, credential_profile_id,<br/>plugin_id, polling_interval, enabled=true)
    PG-->>A: Monitor record
    
    A-->>U: 201 Created<br/>{id, device_id, credential_profile_id, plugin_id, enabled: true}
    
    Note over U,PG: Monitor now active<br/>Scheduler will pick it up on next tick
```

---

## 8. Polling Flow (Background)

```mermaid
sequenceDiagram
    participant S as Scheduler
    participant PG as PostgreSQL
    participant PC as PollJobs Chan
    participant P as Poller
    participant C as Cache
    participant W as Windows Target
    participant MC as MetricResults Chan
    participant Wr as Writer
    participant TS as TimescaleDB

    Note over S,TS: Scheduler Tick (every 10s)
    S->>PG: SELECT monitors<br/>WHERE enabled = true<br/>AND last_attempted + interval < now()
    PG-->>S: monitors[]

    loop For each monitor
        S->>PC: PollJob{monitor_id, device_ip, cred_id}
    end

    Note over S,TS: Poller Worker (from pool)
    PC-->>P: PollJob
    P->>PG: UPDATE monitors<br/>SET last_attempted = now()<br/>WHERE id = monitor_id
    
    P->>C: Get credential (cred_id)
    alt Cache hit
        C-->>P: Credential (decrypted)
    else Cache miss
        C->>PG: SELECT credential WHERE id = cred_id
        PG-->>C: Encrypted credential
        C->>C: Decrypt password
        C->>C: Store in cache
        C-->>P: Credential (decrypted)
    end

    P->>P: Get plugin for cred.type
    
    loop Retry up to 3 times
        P->>W: Collect metrics (WMI/WinRM)
        alt Success
            W-->>P: Raw metrics
            P->>MC: MetricResult{monitor_id, timestamp, metrics}
            Note over P: Break retry loop
        else Failure
            W-->>P: Error
            P->>P: Backoff wait
        end
    end

    alt All retries failed
        P->>PG: UPDATE monitors<br/>SET last_error = error,<br/>last_error_at = now()
    end

    Note over S,TS: Writer Goroutine
    MC-->>Wr: MetricResult
    Wr->>Wr: Buffer results
    
    alt Buffer full OR timeout
        Wr->>TS: Batch INSERT metrics
        Wr->>PG: UPDATE monitors<br/>SET last_polled = now()<br/>WHERE id IN (...)
    end
```

---

## 9. Metrics Query Flow

```mermaid
sequenceDiagram
    participant U as User
    participant A as API
    participant TS as TimescaleDB

    Note over U,TS: Latest Metrics
    U->>A: GET /monitors/:id/metrics
    A->>TS: SELECT * FROM metrics<br/>WHERE monitor_id = :id<br/>ORDER BY timestamp DESC<br/>LIMIT 1
    TS-->>A: Metrics record
    A->>A: Calculate percentages<br/>memory_% = used/total*100<br/>disk_% = used/total*100
    A-->>U: {monitor_id, timestamp,<br/>cpu, memory, disk, network}

    Note over U,TS: Historical Metrics
    U->>A: GET /monitors/:id/metrics/history<br/>?from=...&to=...
    A->>TS: SELECT * FROM metrics<br/>WHERE monitor_id = :id<br/>AND timestamp BETWEEN :from AND :to
    TS-->>A: Metrics records[]
    A->>A: Calculate percentages for each
    A-->>U: {data: [...]}
```

---

## 10. Channel Architecture

```mermaid
flowchart LR
    subgraph Producers
        S[Scheduler]
        P[Poller]
        API[API Services]
    end

    subgraph Channels
        PJ[["PollJobs<br/>chan(100)"]]
        MR[["MetricResults<br/>chan(100)"]]
        CE[["CacheEvents<br/>chan(50)"]]
    end

    subgraph Consumers
        PW[Poller Workers]
        W[Writer]
        CI[Cache Invalidator]
    end

    S --> PJ --> PW
    P --> MR --> W
    API --> CE --> CI
```

### Message Types

```mermaid
classDiagram
    class PollJob {
        +int MonitorID
        +string DeviceIP
        +int CredID
        +Duration Timeout
    }

    class MetricResult {
        +int MonitorID
        +Time Timestamp
        +DeviceMetrics Metrics
        +error Error
    }

    class CacheEvent {
        +string Type
        +int EntityID
    }

    class DeviceMetrics {
        +float64 CPUPercent
        +int64 MemoryTotal
        +int64 MemoryUsed
        +int64 DiskTotal
        +int64 DiskUsed
        +int64 NetSentPS
        +int64 NetRecvPS
        +int ProcessCount
    }

    MetricResult --> DeviceMetrics
```

---

## 11. Component Responsibilities

| Component | Responsibility |
|-----------|----------------|
| **API** | HTTP handlers, JWT auth, validation, routes, CRUD operations |
| **Discovery Engine** | Scans subnets, identifies Windows hosts, saves to DB |
| **PostgreSQL** | State storage: profiles, devices, credentials, monitors |
| **TimescaleDB** | Time-series metrics storage with automatic partitioning |
| **Cache** | In-memory credential cache, invalidated on update events |
| **Scheduler** | Reads due monitors from DB, dispatches PollJobs via channel |
| **Poller** | Worker pool, consumes jobs, decrypts creds, runs plugins |
| **Plugins** | WMI/WinRM collectors (extensible interface) |
| **Writer** | Consumes MetricResults, batch inserts to TimescaleDB |

---

## 12. Deletion Rules & Dependencies

```mermaid
flowchart TD
    subgraph Entities
        CREDP[credential_profiles]
        DP[discovery_profiles]
        DEV[devices]
        PLUG[plugins]
        MON[monitors]
        MET[metrics]
    end

    DP -->|blocks deletion of| CREDP
    MON -->|blocks deletion of| DEV
    MON -->|blocks deletion of| CREDP
    MON -->|blocks deletion of| PLUG
    MON -->|cascade deletes| MET

    DP -.->|no dependency| DEV
```

### Deletion Behavior

| Entity | Rule | On Violation |
|--------|------|--------------|
| **Monitor** | Can always delete | Cascade deletes associated metrics |
| **Credential Profile** | Cannot delete if referenced by any monitor or discovery profile | `400 Bad Request: Credential Profile in use by monitor(s) or discovery profile(s)` |
| **Device** | Cannot delete if referenced by any monitor | `400 Bad Request: Device in use by monitor(s)` |
| **Discovery Profile** | Can always delete | Devices remain (independent once discovered) |
| **Plugin** | Cannot delete if referenced by any monitor | `400 Bad Request: Plugin in use by monitor(s)` |

### Deletion Flow

```mermaid
sequenceDiagram
    participant U as User
    participant A as API
    participant PG as PostgreSQL

    Note over U,PG: Delete Credential Profile (blocked by monitor)
    U->>A: DELETE /credential-profiles/1
    A->>PG: SELECT COUNT(*) FROM monitors<br/>WHERE credential_profile_id = 1
    PG-->>A: count = 2
    A-->>U: 400 Bad Request<br/>"Credential Profile in use by 2 monitor(s)"

    Note over U,PG: Delete Credential Profile (blocked by discovery profile)
    U->>A: DELETE /credential-profiles/1
    A->>PG: SELECT COUNT(*) FROM monitors<br/>WHERE credential_profile_id = 1
    PG-->>A: count = 0
    A->>PG: SELECT COUNT(*) FROM discovery_profiles<br/>WHERE credential_profile_id = 1
    PG-->>A: count = 1
    A-->>U: 400 Bad Request<br/>"Credential Profile in use by 1 discovery profile(s)"

    Note over U,PG: Delete Device (blocked)
    U->>A: DELETE /devices/1
    A->>PG: SELECT COUNT(*) FROM monitors<br/>WHERE device_id = 1
    PG-->>A: count = 1
    A-->>U: 400 Bad Request<br/>"Device in use by 1 monitor(s)"

    Note over U,PG: Correct Order
    U->>A: DELETE /monitors/1
    A->>PG: DELETE FROM metrics WHERE monitor_id = 1
    A->>PG: DELETE FROM monitors WHERE id = 1
    A-->>U: 204 No Content

    U->>A: DELETE /credential-profiles/1
    A->>PG: SELECT COUNT(*) FROM monitors<br/>WHERE credential_profile_id = 1
    PG-->>A: count = 0
    A->>PG: SELECT COUNT(*) FROM discovery_profiles<br/>WHERE credential_profile_id = 1
    PG-->>A: count = 0
    A->>PG: DELETE FROM credential_profiles WHERE id = 1
    A-->>U: 204 No Content

    Note over U,PG: Discovery Profile (no dependency on devices)
    U->>A: DELETE /discovery-profiles/1
    A->>PG: DELETE FROM discovery_profiles WHERE id = 1
    A-->>U: 204 No Content
    Note over U,PG: Devices discovered by this profile remain intact
```

---

## 13. Error Handling Strategy

```mermaid
flowchart TD
    A[Poller receives PollJob] --> B[Update last_attempted]
    B --> C[Attempt collection]
    C --> D{Success?}
    
    D -->|Yes| E[Send to MetricResults channel]
    E --> F[Writer batch inserts]
    F --> G[Update last_polled]
    
    D -->|No| H{Retry count < 3?}
    H -->|Yes| I[Backoff wait]
    I --> C
    H -->|No| J[Update last_error + last_error_at]
    J --> K[Log warning]
    K --> L[Skip this poll cycle]
```

---

## 14. API Endpoints Summary

```
Base: https://localhost:8443/api/v1
```

### Authentication (Stateless)
| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/auth/login` | Get JWT token |

### Step 1: Credential Profiles (create first - reusable auth config)
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/credential-profiles` | List all (no password) |
| POST | `/credential-profiles` | Create credential profile |
| GET | `/credential-profiles/:id` | Get credential profile |
| PUT | `/credential-profiles/:id` | Update credential profile |
| DELETE | `/credential-profiles/:id` | Delete (if no monitors/discovery profiles) |

### Step 2: Discovery Profiles (links to credential profile + defines IP range)
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/discovery-profiles` | List all profiles |
| POST | `/discovery-profiles` | Create profile (requires credential_profile_id) |
| GET | `/discovery-profiles/:id` | Get profile |
| PUT | `/discovery-profiles/:id` | Update profile |
| DELETE | `/discovery-profiles/:id` | Delete profile (devices remain) |
| POST | `/discovery-profiles/:id/run` | Run discovery |

### Step 3: Devices (created by discovery run)
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/devices` | List all devices |
| GET | `/devices/:id` | Get device |
| DELETE | `/devices/:id` | Delete device (if no monitors) |

### Step 4: Monitors (provision discovered devices)
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/monitors` | List all monitors |
| POST | `/monitors` | Create monitor (provision) - requires device_id, credential_profile_id, plugin_id |
| GET | `/monitors/:id` | Get monitor |
| PUT | `/monitors/:id` | Update monitor |
| DELETE | `/monitors/:id` | Delete monitor + its metrics |
| GET | `/monitors/:id/metrics` | Get latest metrics |
| GET | `/monitors/:id/metrics/history` | Get historical metrics |

### Step 5: Plugins (metric collectors assigned to monitors)
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/plugins` | List all plugins |
| POST | `/plugins` | Create plugin |
| GET | `/plugins/:id` | Get plugin |
| PUT | `/plugins/:id` | Update plugin |
| DELETE | `/plugins/:id` | Delete plugin (if no monitors) |

### Health
| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/health` | Health check (no auth) |
