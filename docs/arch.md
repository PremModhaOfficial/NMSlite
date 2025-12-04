# NMSlite - Architecture Document

> **Version:** 1.0.0\
> **Last Updated:** December 2024\
> **Status:** Design Phase

------------------------------------------------------------------------

## Table of Contents

1.  [Overview](#1-overview)
2.  [System Context](#2-system-context)
3.  [High-Level Architecture](#3-high-level-architecture)
4.  [Service Architecture](#4-service-architecture)
5.  [Data Architecture](#5-data-architecture)
6.  [Message Queue Architecture](#6-message-queue-architecture)
7.  [Caching Architecture](#7-caching-architecture)
8.  [Security Architecture](#8-security-architecture)
9.  [API Specification](#9-api-specification)
10. [Configuration](#10-configuration)
11. [Technology Stack](#11-technology-stack)
12. [Non-Functional Requirements](#12-non-functional-requirements)

------------------------------------------------------------------------

## 1. Overview

### 1.1 Purpose

NMSlite is a lightweight Network Monitoring System designed to monitor
**Windows machines**, collecting performance metrics (CPU, memory, disk,
network, optionally processes) via WMI/WinRM protocols.

### 1.2 High-Level Goals

  -----------------------------------------------------------------------
  \#        Goal                Description
  --------- ------------------- -----------------------------------------
  1         **JWT-Secured API** Production-minded HTTP API server secured
                                with JWT

  2         **CRUD Operations** Credential profiles, device discovery,
                                monitor provisioning

  3         **Windows Metrics** Collect disk, memory, CPU, network
                                (optionally processes)

  4         **Event-Driven**    Message queue for decoupling discovery,
                                polling, storage

  5         **Caching**         Cache device metadata, credentials to
                                reduce DB load

  6         **API-First**       Query device details via Postman (clear
                                spec + examples)

  7         **Plugin            Plugin-style polling modules for
            Collectors**        extensibility

  8         **Testable**        Include tests (Phase 2)
  -----------------------------------------------------------------------

### 1.3 Scope

**In Scope:** - CRUD for credential profiles (WMI/WinRM) - Device
discovery and inventory management - Monitor provisioning via APIs -
Windows metrics collection (CPU, memory, disk, network, processes) -
Event-driven architecture with message queue - Caching layer for
frequently accessed data - JWT authentication with HTTPS/TLS -
Structured logging - Plugin-style collector architecture - Postman
collection for API testing

**Out of Scope (v1.0):** - Linux/SSH monitoring - Web UI / Dashboard -
Alerting system - Multi-tenancy - SNMP monitoring

------------------------------------------------------------------------

## 2. System Context

### 2.1 Context Diagram

                                        ┌─────────────────┐
                                        │    API Client   │
                                        │    (Postman)    │
                                        └────────┬────────┘
                                                 │
                                                 │ HTTPS + JWT
                                                 ▼
    ┌──────────────────────────────────────────────────────────────────────┐
    │                             NMSlite                                  │
    │                                                                      │
    │  ┌───────────┐    ┌───────────┐    ┌───────────┐    ┌───────────┐    │
    │  │  REST API │    │  Message  │    │  Polling  │    │  Storage  │    │
    │  │  Server   │───▶│   Queue   │───▶│  Workers  │───▶│  Writer   │    │
    │  └───────────┘    └───────────┘    └───────────┘    └───────────┘    │
    │        │                                                  │          │
    │        │          ┌───────────┐                           │          │
    │        └─────────▶│   Cache   │◀──────────────────────────┘          │
    │                   └───────────┘                                      │
    │                                                                      │
    │                   ┌───────────┐                                      │
    │                   │ PostgreSQL│                                      │
    │                   └───────────┘                                      │
    └──────────────────────────────────────────────────────────────────────┘
                                        │
                                        │ WMI / WinRM
                                        ▼
                                ┌───────────────┐
                                │    Windows    │
                                │    Targets    │
                                └───────────────┘

### 2.2 External Interfaces

  System            Protocol      Port            Purpose
  ----------------- ------------- --------------- ------------------------
  Windows Targets   WMI           135 + dynamic   Legacy Windows metrics
  Windows Targets   WinRM HTTP    5985            Modern Windows metrics
  Windows Targets   WinRM HTTPS   5986            Secure Windows metrics
  API Clients       HTTPS         8443            REST API access
  PostgreSQL        TCP           5432            Data persistence

------------------------------------------------------------------------

## 3. High-Level Architecture

### 3.1 Architecture Style

**Event-Driven Modular Monolith**

-   Single deployable binary with internal service boundaries
-   Message queue for decoupling components
-   Asynchronous processing for scalability

### 3.2 Component Overview

    ┌─────────────────────────────────────────────────────────────────────────────┐
    │                              NMSlite Binary                                 │
    ├─────────────────────────────────────────────────────────────────────────────┤
    │                                                                             │
    │  ┌────────────────────────────────────────────────────────────────────────┐ │
    │  │                           HTTP Layer                                   │ │
    │  │  ┌────────────┐  ┌────────────┐  ┌────────────┐  ┌────────────────┐    │ │
    │  │  │   Router   │  │    JWT     │  │    TLS     │  │  Rate Limiter  │    │ │
    │  │  │   (chi)    │  │ Middleware │  │ Middleware │  │  Middleware    │    │ │
    │  │  └────────────┘  └────────────┘  └────────────┘  └────────────────┘    │ │
    │  └────────────────────────────────────────────────────────────────────────┘ │
    │                                      │                                      │
    │                                      ▼                                      │
    │  ┌────────────────────────────────────────────────────────────────────────┐ │
    │  │                          API Handlers                                  │ │
    │  │  ┌────────────┐  ┌────────────┐  ┌────────────┐  ┌────────────────┐    │ │
    │  │  │    Auth    │  │   Device   │  │ Credential │  │    Metrics     │    │ │
    │  │  │  Handler   │  │  Handler   │  │  Handler   │  │    Handler     │    │ │
    │  │  └────────────┘  └────────────┘  └────────────┘  └────────────────┘    │ │
    │  └────────────────────────────────────────────────────────────────────────┘ │
    │                                      │                                      │
    │                                      ▼                                      │
    │  ┌────────────────────────────────────────────────────────────────────────┐ │
    │  │                         Service Layer                                  │ │
    │  │  ┌────────────┐  ┌────────────┐  ┌────────────┐  ┌────────────────┐    │ │
    │  │  │    Auth    │  │   Device   │  │ Credential │  │    Metrics     │    │ │
    │  │  │  Service   │  │  Service   │  │  Service   │  │    Service     │    │ │
    │  │  └────────────┘  └────────────┘  └────────────┘  └────────────────┘    │ │
    │  └────────────────────────────────────────────────────────────────────────┘ │
    │                                      │                                      │
    │          ┌───────────────────────────┼───────────────────────────┐          │
    │          │                           │                           │          │
    │          ▼                           ▼                           ▼          │
    │  ┌──────────────┐          ┌─────────────────┐          ┌──────────────┐    │
    │  │    Cache     │          │  Message Queue  │          │  Repository  │    │
    │  │  (In-Memory) │          │                 │          │    Layer     │    │
    │  └──────────────┘          └────────┬────────┘          └──────────────┘    │
    │                                     │                           │           │
    │                                     ▼                           │           │
    │  ┌───────────────────────────────────────────────────────────┐  │           │
    │  │                      Worker Pool                          │  │           │
    │  │  ┌─────────────────────────────────────────────────────┐  │  │           │
    │  │  │              Collector Registry (Plugin)            │  │  │           │
    │  │  │  ┌───────────┐  ┌───────────┐  ┌───────────────┐    │  │  │           │
    │  │  │  │    WMI    │  │   WinRM   │  │    Future     │    │  │  │           │
    │  │  │  │ Collector │  │ Collector │  │   Plugins     │    │  │  │           │
    │  │  │  └───────────┘  └───────────┘  └───────────────┘    │  │  │           │
    │  │  └─────────────────────────────────────────────────────┘  │  │           │
    │  └───────────────────────────────────────────────────────────┘  │           │
    │                                     │                           │           │
    │                                     ▼                           ▼           │
    │                            ┌──────────────┐           ┌──────────────┐      │
    │                            │   Storage    │──────────▶│  PostgreSQL  │      │
    │                            │   Writer     │           │              │      │
    │                            └──────────────┘           └──────────────┘      │
    │                                                                             │
    └─────────────────────────────────────────────────────────────────────────────┘

### 3.3 Data Flow

    ┌─────────────────────────────────────────────────────────────────────────────┐
    │                              Request Flow                                   │
    ├─────────────────────────────────────────────────────────────────────────────┤
    │                                                                             │
    │  1. API Request (Provision Device)                                          │
    │     ┌────────┐      ┌─────────┐      ┌─────────────┐      ┌─────────┐       │
    │     │ Client │─────▶│   API   │─────▶│   Service   │─────▶│   DB    │       │
    │     └────────┘      └─────────┘      └──────┬──────┘      └─────────┘       │
    │                                             │                               │
    │                                             ▼                               │
    │  2. Publish Poll Job                  ┌─────────────┐                       │
    │                                       │    Queue    │                       │
    │                                       │ (poll_jobs) │                       │
    │                                       └──────┬──────┘                       │
    │                                              │                              │
    │                                              ▼                              │
    │  3. Worker Consumes Job               ┌─────────────┐                       │
    │                                       │   Worker    │                       │
    │                                       └──────┬──────┘                       │
    │                                              │                              │
    │                                              ▼                              │
    │  4. Collect Metrics                   ┌─────────────┐      ┌─────────┐      │
    │                                       │  Collector  │─────▶│ Windows │      │
    │                                       │  (WMI/WinRM)│◀─────│  Target │      │
    │                                       └──────┬──────┘      └─────────┘      │
    │                                              │                              │
    │                                              ▼                              │
    │  5. Publish Metrics                   ┌─────────────┐                       │
    │                                       │    Queue    │                       │
    │                                       │ (metrics)   │                       │
    │                                       └──────┬──────┘                       │
    │                                              │                              │
    │                                              ▼                              │
    │  6. Store Metrics                     ┌─────────────┐      ┌─────────┐      │
    │                                       │   Storage   │─────▶│   DB    │      │
    │                                       │   Writer    │      │         │      │
    │                                       └─────────────┘      └─────────┘      │
    │                                                                             │
    └─────────────────────────────────────────────────────────────────────────────┘

------------------------------------------------------------------------

## 4. Service Architecture

### 4.1 Auth Service

**Responsibility:** JWT authentication and token management

::: {#cb4 .sourceCode}
``` {.sourceCode .go}
type AuthService interface {
    Login(ctx context.Context, username, password string) (*TokenPair, error)
    RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error)
    ValidateToken(ctx context.Context, accessToken string) (*Claims, error)
}

type TokenPair struct {
    AccessToken  string    `json:"access_token"`
    RefreshToken string    `json:"refresh_token"`
    ExpiresAt    time.Time `json:"expires_at"`
}

type Claims struct {
    UserID   int    `json:"user_id"`
    Username string `json:"username"`
    Role     string `json:"role"`
}
```
:::

------------------------------------------------------------------------

### 4.2 Device Service

**Responsibility:** Device CRUD and discovery

::: {#cb5 .sourceCode}
``` {.sourceCode .go}
type DeviceService interface {
    // CRUD
    Create(ctx context.Context, device *CreateDeviceRequest) (*Device, error)
    GetByID(ctx context.Context, id int) (*Device, error)
    List(ctx context.Context, filters *DeviceFilters) (*DeviceList, error)
    Update(ctx context.Context, id int, req *UpdateDeviceRequest) (*Device, error)
    Delete(ctx context.Context, id int) error
    
    // Discovery
    Discover(ctx context.Context, req *DiscoverRequest) (*DiscoverResult, error)
    
    // Provisioning (publishes to queue)
    Provision(ctx context.Context, deviceID int, credentialID int) error
    Deprovision(ctx context.Context, deviceID int) error
}

type Device struct {
    ID              int       `json:"id"`
    IP              string    `json:"ip"`
    Hostname        string    `json:"hostname"`
    OS              string    `json:"os"`
    Status          string    `json:"status"`  // discovered, provisioned, monitoring, unreachable
    PollingInterval int       `json:"polling_interval"`
    LastSeen        time.Time `json:"last_seen"`
    CreatedAt       time.Time `json:"created_at"`
}
```
:::

------------------------------------------------------------------------

### 4.3 Credential Service

**Responsibility:** Secure credential management for Windows auth

::: {#cb6 .sourceCode}
``` {.sourceCode .go}
type CredentialService interface {
    Create(ctx context.Context, cred *CreateCredentialRequest) (*Credential, error)
    GetByID(ctx context.Context, id int) (*Credential, error)
    List(ctx context.Context) ([]Credential, error)
    Update(ctx context.Context, id int, req *UpdateCredentialRequest) (*Credential, error)
    Delete(ctx context.Context, id int) error
    
    // Internal use only (for polling)
    GetDecrypted(ctx context.Context, id int) (*DecryptedCredential, error)
}

type Credential struct {
    ID             int       `json:"id"`
    Name           string    `json:"name"`
    CredentialType string    `json:"credential_type"`  // wmi, winrm_basic, winrm_ntlm
    Username       string    `json:"username"`
    Domain         string    `json:"domain,omitempty"`
    Port           int       `json:"port,omitempty"`
    UseSSL         bool      `json:"use_ssl"`
    CreatedAt      time.Time `json:"created_at"`
    // Password never returned in API responses
}
```
:::

**Credential Types:**

  Type            Auth Method   Default Port   Use Case
  --------------- ------------- -------------- ------------------------
  `wmi`           NTLM          135            Legacy Windows servers
  `winrm_basic`   Basic Auth    5985/5986      Standalone Windows
  `winrm_ntlm`    NTLM          5985/5986      Domain-joined Windows

------------------------------------------------------------------------

### 4.4 Metrics Service

**Responsibility:** Metrics retrieval (read operations)

::: {#cb7 .sourceCode}
``` {.sourceCode .go}
type MetricsService interface {
    GetLatest(ctx context.Context, deviceID int) (*DeviceMetrics, error)
    GetHistory(ctx context.Context, deviceID int, req *HistoryRequest) (*MetricsHistory, error)
}

type DeviceMetrics struct {
    DeviceID     int            `json:"device_id"`
    Timestamp    time.Time      `json:"timestamp"`
    CPU          CPUMetrics     `json:"cpu"`
    Memory       MemoryMetrics  `json:"memory"`
    Disk         DiskMetrics    `json:"disk"`
    Network      NetworkMetrics `json:"network"`
    ProcessCount int            `json:"process_count"`
}

type CPUMetrics struct {
    UsagePercent float64 `json:"usage_percent"`
}

type MemoryMetrics struct {
    TotalBytes   int64   `json:"total_bytes"`
    UsedBytes    int64   `json:"used_bytes"`
    UsagePercent float64 `json:"usage_percent"`
}

type DiskMetrics struct {
    TotalBytes   int64   `json:"total_bytes"`
    UsedBytes    int64   `json:"used_bytes"`
    UsagePercent float64 `json:"usage_percent"`
}

type NetworkMetrics struct {
    BytesSentPerSec   int64   `json:"bytes_sent_per_sec"`
    BytesRecvPerSec   int64   `json:"bytes_recv_per_sec"`
    UtilizationPercent float64 `json:"utilization_percent"`
    PacketsSent       int64   `json:"packets_sent"`
    PacketsRecv       int64   `json:"packets_recv"`
    Errors            int64   `json:"errors"`
    Dropped           int64   `json:"dropped"`
}
```
:::

------------------------------------------------------------------------

### 4.5 Polling Service (Event-Driven)

**Responsibility:** Orchestrate metric collection via message queue

::: {#cb8 .sourceCode}
``` {.sourceCode .go}
type PollingService interface {
    // Lifecycle
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    
    // Queue consumers
    HandlePollJob(ctx context.Context, job *PollJob) error
    HandleMetricsResult(ctx context.Context, result *MetricsResult) error
}

// Message types
type PollJob struct {
    JobID        string    `json:"job_id"`
    DeviceID     int       `json:"device_id"`
    CredentialID int       `json:"credential_id"`
    ScheduledAt  time.Time `json:"scheduled_at"`
}

type MetricsResult struct {
    JobID     string         `json:"job_id"`
    DeviceID  int            `json:"device_id"`
    Timestamp time.Time      `json:"timestamp"`
    Success   bool           `json:"success"`
    Error     string         `json:"error,omitempty"`
    Metrics   *DeviceMetrics `json:"metrics,omitempty"`
}
```
:::

------------------------------------------------------------------------

### 4.6 Collector Interface (Plugin Architecture)

**Responsibility:** Pluggable metric collection

::: {#cb9 .sourceCode}
``` {.sourceCode .go}
// Collector is the plugin interface for metric collection
type Collector interface {
    // Name returns the collector identifier
    Name() string
    
    // Supports checks if this collector can handle the credential type
    Supports(credentialType string) bool
    
    // Collect gathers metrics from the target device
    Collect(ctx context.Context, target *CollectorTarget) (*DeviceMetrics, error)
}

type CollectorTarget struct {
    IP         string
    Port       int
    Username   string
    Password   string  // Decrypted
    Domain     string
    UseSSL     bool
    Timeout    time.Duration
}

// Registry manages available collectors
type CollectorRegistry interface {
    Register(collector Collector)
    Get(credentialType string) (Collector, error)
    List() []string
}
```
:::

**Built-in Collectors:**

  Collector          Credential Types              Description
  ------------------ ----------------------------- -----------------------------
  `WMICollector`     `wmi`                         Uses go-ole for WMI queries
  `WinRMCollector`   `winrm_basic`, `winrm_ntlm`   Uses WinRM protocol

------------------------------------------------------------------------

## 5. Data Architecture

### 5.1 Entity Relationship Diagram

    ┌─────────────────────────────────────────────────────────────────────────────┐
    │                              AUTHENTICATION                                 │
    ├─────────────────────────────────────────────────────────────────────────────┤
    │                                                                             │
    │    ┌─────────────────┐              ┌──────────────────────┐                │
    │    │     users       │──────────────│   refresh_tokens     │                │
    │    │                 │    1 : N     │                      │                │
    │    │ id (PK)         │              │ id (PK)              │                │
    │    │ username        │              │ user_id (FK)         │                │
    │    │ password_hash   │              │ token_hash           │                │
    │    │ role            │              │ expires_at           │                │
    │    │ created_at      │              │ created_at           │                │
    │    │ updated_at      │              └──────────────────────┘                │
    │    └─────────────────┘                                                      │
    │                                                                             │
    └─────────────────────────────────────────────────────────────────────────────┘

    ┌─────────────────────────────────────────────────────────────────────────────┐
    │                               INVENTORY                                     │
    ├─────────────────────────────────────────────────────────────────────────────┤
    │                                                                             │
    │    ┌─────────────────────┐                    ┌─────────────────────┐       │
    │    │    credentials      │                    │      devices        │       │
    │    │                     │      N : N         │                     │       │
    │    │ id (PK)             │◄──────────────────▶│ id (PK)             │       │
    │    │ name                │                    │ ip                  │       │
    │    │ credential_type     │  device_credentials│ hostname            │       │
    │    │ username            │  ┌───────────────┐ │ os                  │       │
    │    │ encrypted_password  │  │device_id (FK) │ │ status              │       │
    │    │ domain              │  │cred_id (FK)   │ │ polling_interval    │       │
    │    │ port                │  │priority       │ │ last_seen           │       │
    │    │ use_ssl             │  │is_active      │ │ created_at          │       │
    │    │ created_at          │  │last_success   │ │ updated_at          │       │
    │    │ updated_at          │  └───────────────┘ │                     │       │
    │    └─────────────────────┘                    └──────────┬──────────┘       │
    │                                                          │                  │
    └──────────────────────────────────────────────────────────┼──────────────────┘
                                                               │
                                                               │ 1 : N
                                                               ▼
    ┌─────────────────────────────────────────────────────────────────────────────┐
    │                           TIME-SERIES METRICS                               │
    ├─────────────────────────────────────────────────────────────────────────────┤
    │                                                                             │
    │  ┌─────────────────────────────────────────────────────────────────────┐    │
    │  │                     metrics (unified table)                         │    │
    │  │                   (partitioned by timestamp)                        │    │
    │  │                  All values are system-wide aggregates              │    │
    │  │                                                                     │    │
    │  │  id (PK)                    - BIGSERIAL                             │    │
    │  │  device_id (FK)             - INTEGER, references devices           │    │
    │  │  timestamp                  - TIMESTAMPTZ                           │    │
    │  │                                                                     │    │
    │  │  -- CPU (system-wide) --                                            │    │
    │  │  cpu_usage_percent          - NUMERIC(5,2)                          │    │
    │  │                                                                     │    │
    │  │  -- Memory (system-wide) --                                         │    │
    │  │  memory_total_bytes         - BIGINT                                │    │
    │  │  memory_used_bytes          - BIGINT                                │    │
    │  │  memory_usage_percent       - NUMERIC(5,2)                          │    │
    │  │                                                                     │    │
    │  │  -- Disk (all drives combined) --                                   │    │
    │  │  disk_total_bytes           - BIGINT                                │    │
    │  │  disk_used_bytes            - BIGINT                                │    │
    │  │  disk_usage_percent         - NUMERIC(5,2)                          │    │
    │  │                                                                     │    │
    │  │  -- Network (all interfaces combined) --                            │    │
    │  │  net_bytes_sent_per_sec     - BIGINT                                │    │
    │  │  net_bytes_recv_per_sec     - BIGINT                                │    │
    │  │  net_utilization_percent    - NUMERIC(5,2)                          │    │
    │  │  net_packets_sent           - BIGINT                                │    │
    │  │  net_packets_recv           - BIGINT                                │    │
    │  │  net_errors                 - BIGINT                                │    │
    │  │  net_dropped                - BIGINT                                │    │
    │  │                                                                     │    │
    │  │  -- Process --                                                      │    │
    │  │  process_count              - INTEGER                               │    │
    │  │                                                                     │    │
    │  │  -- Metadata --                                                     │    │
    │  │  collection_duration_ms     - INTEGER                               │    │
    │  └─────────────────────────────────────────────────────────────────────┘    │
    │                                                                             │
    └─────────────────────────────────────────────────────────────────────────────┘

### 5.2 SQL Schema

::: {#cb11 .sourceCode}
``` {.sourceCode .sql}
-- Users
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    username VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    role VARCHAR(50) NOT NULL DEFAULT 'operator',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- Refresh Tokens
CREATE TABLE refresh_tokens (
    id SERIAL PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash VARCHAR(255) UNIQUE NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Credentials
CREATE TABLE credentials (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    credential_type VARCHAR(50) NOT NULL,  -- wmi, winrm_basic, winrm_ntlm
    username VARCHAR(255) NOT NULL,
    encrypted_password TEXT NOT NULL,
    domain VARCHAR(255),
    port INTEGER,
    use_ssl BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    
    CONSTRAINT valid_cred_type CHECK (credential_type IN ('wmi', 'winrm_basic', 'winrm_ntlm'))
);

-- Devices
CREATE TABLE devices (
    id SERIAL PRIMARY KEY,
    ip VARCHAR(45) UNIQUE NOT NULL,
    hostname VARCHAR(255),
    os VARCHAR(255),
    status VARCHAR(50) DEFAULT 'discovered',
    polling_interval INTEGER DEFAULT 60,
    last_seen TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    
    CONSTRAINT valid_status CHECK (status IN ('discovered', 'provisioned', 'monitoring', 'unreachable'))
);

-- Device-Credential Association
CREATE TABLE device_credentials (
    id SERIAL PRIMARY KEY,
    device_id INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    credential_id INTEGER NOT NULL REFERENCES credentials(id) ON DELETE CASCADE,
    priority INTEGER DEFAULT 1,
    is_active BOOLEAN DEFAULT TRUE,
    last_success TIMESTAMPTZ,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    
    UNIQUE(device_id, credential_id)
);

-- Unified Metrics Table (partitioned) - All values are system-wide aggregates
CREATE TABLE metrics (
    id BIGSERIAL,
    device_id INTEGER NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- CPU (system-wide)
    cpu_usage_percent NUMERIC(5,2),
    
    -- Memory (system-wide)
    memory_total_bytes BIGINT,
    memory_used_bytes BIGINT,
    memory_usage_percent NUMERIC(5,2),
    
    -- Disk (all drives combined)
    disk_total_bytes BIGINT,
    disk_used_bytes BIGINT,
    disk_usage_percent NUMERIC(5,2),
    
    -- Network (all interfaces combined)
    net_bytes_sent_per_sec BIGINT,
    net_bytes_recv_per_sec BIGINT,
    net_utilization_percent NUMERIC(5,2),
    net_packets_sent BIGINT,
    net_packets_recv BIGINT,
    net_errors BIGINT,
    net_dropped BIGINT,
    
    -- Process count
    process_count INTEGER,
    
    -- Collection metadata
    collection_duration_ms INTEGER,
    
    PRIMARY KEY (id, timestamp),
    FOREIGN KEY (device_id) REFERENCES devices(id) ON DELETE CASCADE
) PARTITION BY RANGE (timestamp);

-- Indexes
CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_refresh_tokens_hash ON refresh_tokens(token_hash);
CREATE INDEX idx_devices_ip ON devices(ip);
CREATE INDEX idx_devices_status ON devices(status);
CREATE INDEX idx_device_creds_device ON device_credentials(device_id);
CREATE INDEX idx_metrics_device_time ON metrics(device_id, timestamp DESC);
```
:::

### 5.3 Partitioning Strategy

Monthly partitions for time-series data:

::: {#cb12 .sourceCode}
``` {.sourceCode .sql}
-- Create partitions
CREATE TABLE metrics_y2025m01 PARTITION OF metrics
    FOR VALUES FROM ('2025-01-01') TO ('2025-02-01');

CREATE TABLE metrics_y2025m02 PARTITION OF metrics
    FOR VALUES FROM ('2025-02-01') TO ('2025-03-01');

-- Automated partition creation (via cron or app)
-- Retention: DROP old partitions after 90 days
```
:::

------------------------------------------------------------------------

## 6. Message Queue Architecture

### 6.1 Queue Selection: **NATS**

  -----------------------------------------------------------------------
  Criteria              NATS         RabbitMQ              Kafka
  --------------------- ------------ --------------------- --------------
  Simplicity            High         Medium                Low

  Single binary         Yes          No                    No

  Go native             Yes          Via AMQP              Via client

  Persistence           JetStream    Yes                   Yes

  Latency               Very Low     Low                   Medium

  Setup                 Embed or     Erlang runtime        ZooKeeper +
                        single                             brokers
                        binary                             
  -----------------------------------------------------------------------

**Justification:** NATS with JetStream provides: - Embeddable in Go (no
external process required) - Simple pub/sub and persistent queues - Low
latency for real-time polling - Built-in clustering (future scalability)

### 6.2 Queue Topics

    ┌─────────────────────────────────────────────────────────────────────────────┐
    │                              Message Queues                                 │
    ├─────────────────────────────────────────────────────────────────────────────┤
    │                                                                             │
    │  ┌─────────────────────────────────────────────────────────────────────┐    │
    │  │  nmslite.discovery.jobs                                             │    │
    │  │  - Trigger: API POST /discover                                      │    │
    │  │  - Consumer: Discovery Worker                                       │    │
    │  │  - Payload: { subnet: "192.168.1.0/24" }                            │    │
    │  └─────────────────────────────────────────────────────────────────────┘    │
    │                                                                             │
    │  ┌─────────────────────────────────────────────────────────────────────┐    │
    │  │  nmslite.poll.jobs                                                  │    │
    │  │  - Trigger: Scheduler tick OR API manual poll                       │    │
    │  │  - Consumer: Poll Workers (N concurrent)                            │    │
    │  │  - Payload: { device_id, credential_id, scheduled_at }              │    │
    │  └─────────────────────────────────────────────────────────────────────┘    │
    │                                                                             │
    │  ┌─────────────────────────────────────────────────────────────────────┐    │
    │  │  nmslite.metrics.results                                            │    │
    │  │  - Trigger: Poll Worker completion                                  │    │
    │  │  - Consumer: Storage Writer                                         │    │
    │  │  - Payload: { device_id, timestamp, metrics: {...} }                │    │
    │  └─────────────────────────────────────────────────────────────────────┘    │
    │                                                                             │
    │  ┌─────────────────────────────────────────────────────────────────────┐    │
    │  │  nmslite.device.events                                              │    │
    │  │  - Trigger: Status changes                                          │    │
    │  │  - Consumer: Cache invalidator                                      │    │
    │  │  - Payload: { device_id, event: "status_changed", new_status }      │    │
    │  └─────────────────────────────────────────────────────────────────────┘    │
    │                                                                             │
    └─────────────────────────────────────────────────────────────────────────────┘

### 6.3 Message Flow

    ┌──────────────────────────────────────────────────────────────────────────────┐
    │                           Polling Flow                                       │
    ├──────────────────────────────────────────────────────────────────────────────┤
    │                                                                              │
    │   Scheduler                  Queue                    Workers                │
    │      │                         │                         │                   │
    │      │  Every polling_interval │                         │                   │
    │      │─────────────────────────▶                         │                   │
    │      │   Publish PollJob       │                         │                   │
    │      │   to poll.jobs          │                         │                   │
    │      │                         │                         │                   │
    │      │                         │  Subscribe (N workers)  │                   │
    │      │                         │◀────────────────────────│                   │
    │      │                         │                         │                   │
    │      │                         │  Deliver job            │                   │
    │      │                         │─────────────────────────▶                   │
    │      │                         │                         │                   │
    │      │                         │                         │  Get credential   │
    │      │                         │                         │  from cache       │
    │      │                         │                         │        │          │
    │      │                         │                         │        ▼          │
    │      │                         │                         │  Connect to       │
    │      │                         │                         │  Windows target   │
    │      │                         │                         │        │          │
    │      │                         │                         │        ▼          │
    │      │                         │                         │  Collect metrics  │
    │      │                         │                         │        │          │
    │      │                         │                         │        ▼          │
    │      │                         │  Publish MetricsResult  │                   │
    │      │                         │◀────────────────────────│                   │
    │      │                         │  to metrics.results     │                   │
    │      │                         │                         │                   │
    │                                                                              │
    │   Storage Writer              Queue                                          │
    │      │                         │                                             │
    │      │  Subscribe              │                                             │
    │      │◀────────────────────────│                                             │
    │      │                         │                                             │
    │      │  Batch insert to DB     │                                             │
    │      │                         │                                             │
    │                                                                              │
    └──────────────────────────────────────────────────────────────────────────────┘

------------------------------------------------------------------------

## 7. Caching Architecture

### 7.1 Cache Strategy

**Type:** Pure In-Memory (Go `sync.RWMutex` + `map` with TTL expiration)

No external caching libraries. Simple, zero-dependency implementation
using standard library primitives.

  ------------------------------------------------------------------------
  Data          TTL        Invalidation                  Reason
  ------------- ---------- ----------------------------- -----------------
  Device        5 min      On update event               Frequently read
  metadata                                               by polling

  Decrypted     5 min      On update event               Avoid DB +
  credentials                                            decrypt on every
                                                         poll

  Latest        30 sec     On new metrics                API reads for
  metrics (per                                           "current status"
  device)                                                
  ------------------------------------------------------------------------

### 7.2 Cache Implementation

::: {#cb15 .sourceCode}
``` {.sourceCode .go}
// Pure stdlib implementation - no external dependencies
type Cache struct {
    mu      sync.RWMutex
    devices map[int]*cachedItem[Device]
    creds   map[int]*cachedItem[DecryptedCredential]
    metrics map[int]*cachedItem[DeviceMetrics]
}

type cachedItem[T any] struct {
    data      *T
    expiresAt time.Time
}

// Example methods
func (c *Cache) GetDevice(ctx context.Context, id int) (*Device, bool) {
    c.mu.RLock()
    defer c.mu.RUnlock()
    
    item, ok := c.devices[id]
    if !ok || time.Now().After(item.expiresAt) {
        return nil, false
    }
    return item.data, true
}

func (c *Cache) SetDevice(ctx context.Context, device *Device, ttl time.Duration) {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    c.devices[device.ID] = &cachedItem[Device]{
        data:      device,
        expiresAt: time.Now().Add(ttl),
    }
}

func (c *Cache) InvalidateDevice(ctx context.Context, id int) {
    c.mu.Lock()
    defer c.mu.Unlock()
    delete(c.devices, id)
}
```
:::

**Design Rationale:** - Zero external dependencies - Full control over
eviction logic - Sufficient for 100-500 devices scale - Easy to
understand, test, and debug - Background goroutine for periodic cleanup
of expired entries

### 7.3 Cache Invalidation Flow

    ┌─────────────────────────────────────────────────────────────────────────────┐
    │                           Cache Invalidation                                │
    ├─────────────────────────────────────────────────────────────────────────────┤
    │                                                                             │
    │   API Handler                Service               Cache         Queue      │
    │       │                         │                    │              │       │
    │       │  Update Device          │                    │              │       │
    │       │────────────────────────▶│                    │              │       │
    │       │                         │                    │              │       │
    │       │                         │  Update DB         │              │       │
    │       │                         │────────▶           │              │       │
    │       │                         │                    │              │       │
    │       │                         │  Publish event     │              │       │
    │       │                         │───────────────────────────────────▶       │
    │       │                         │  (device.events)   │              │       │
    │       │                         │                    │              │       │
    │       │                         │                    │  Subscribe   │       │
    │       │                         │                    │◀─────────────│       │
    │       │                         │                    │              │       │
    │       │                         │                    │  Invalidate  │       │
    │       │                         │                    │  cache entry │       │
    │       │                         │                    │              │       │
    │                                                                             │
    └─────────────────────────────────────────────────────────────────────────────┘

------------------------------------------------------------------------

## 8. Security Architecture

### 8.1 Authentication Flow

    ┌────────┐          ┌─────────┐          ┌──────────┐
    │ Client │          │   API   │          │ Database │
    └───┬────┘          └────┬────┘          └────┬─────┘
        │                    │                    │
        │ POST /auth/login   │                    │
        │ {user, password}   │                    │
        │───────────────────▶│                    │
        │                    │                    │
        │                    │ Verify bcrypt      │
        │                    │ Generate JWT       │
        │                    │                    │
        │ {access_token,     │                    │
        │  refresh_token,    │                    │
        │  expires_at}       │                    │
        │◀───────────────────│                    │
        │                    │                    │
        │ GET /api/devices   │                    │
        │ Auth: Bearer <jwt> │                    │
        │───────────────────▶│                    │
        │                    │                    │
        │                    │ Validate JWT       │
        │                    │ (signature+expiry) │
        │                    │                    │
        │ {devices: [...]}   │                    │
        │◀───────────────────│                    │

### 8.2 JWT Structure

::: {#cb18 .sourceCode}
``` {.sourceCode .json}
{
  "header": {
    "alg": "HS256",
    "typ": "JWT"
  },
  "payload": {
    "sub": "1",
    "username": "admin",
    "role": "admin",
    "iat": 1733000000,
    "exp": 1733000900
  }
}
```
:::

**Token Lifetimes:** - Access Token: 15 minutes - Refresh Token: 7 days

### 8.3 Credential Encryption

    ┌─────────────────────────────────────────────────────────────────┐
    │                    Credential Storage                           │
    ├─────────────────────────────────────────────────────────────────┤
    │                                                                 │
    │  Input                 Encryption              Storage          │
    │  ┌─────────────┐      ┌──────────────┐      ┌───────────────┐   │
    │  │ password:   │─────▶│ AES-256-GCM  │─────▶│ encrypted_    │   │
    │  │ "P@ssw0rd"  │      │ (master key) │      │ password      │   │
    │  └─────────────┘      └──────────────┘      └───────────────┘   │
    │                                                                 │
    │  Master Key Source: NMSLITE_MASTER_KEY environment variable     │
    │                                                                 │
    └─────────────────────────────────────────────────────────────────┘

### 8.4 TLS Configuration

::: {#cb20 .sourceCode}
``` {.sourceCode .yaml}
server:
  tls:
    enabled: true
    cert_file: /etc/nmslite/tls/server.crt
    key_file: /etc/nmslite/tls/server.key
    min_version: "1.2"
```
:::

### 8.5 Security Checklist

  Control                 Implementation
  ----------------------- -----------------------------
  Password hashing        bcrypt (cost 12)
  Credential encryption   AES-256-GCM
  Transport security      TLS 1.2+
  Token security          Short-lived JWT + refresh
  Secret storage          Environment variables
  SQL injection           Parameterized queries (pgx)

------------------------------------------------------------------------

## 9. API Specification

### 9.1 Endpoints Overview

    BASE URL: https://localhost:8443/api/v1

    Authentication
      POST   /auth/login              Login, get tokens
      POST   /auth/refresh            Refresh access token

    Credentials
      GET    /credentials             List all credentials
      POST   /credentials             Create credential
      GET    /credentials/:id         Get credential by ID
      PUT    /credentials/:id         Update credential
      DELETE /credentials/:id         Delete credential

    Devices
      GET    /devices                 List all devices
      POST   /devices                 Create device
      GET    /devices/:id             Get device by ID
      PUT    /devices/:id             Update device
      DELETE /devices/:id             Delete device
      POST   /devices/discover        Discover devices in subnet
      POST   /devices/:id/provision   Provision monitoring
      POST   /devices/:id/deprovision Deprovision monitoring

    Metrics
      GET    /devices/:id/metrics           Get latest metrics
      GET    /devices/:id/metrics/history   Get historical metrics

    Health
      GET    /health                  Health check (no auth)

### 9.2 Request/Response Examples

#### Login

``` http
POST /api/v1/auth/login
Content-Type: application/json

{
  "username": "admin",
  "password": "secret"
}
```

::: {#cb23 .sourceCode}
``` {.sourceCode .json}
{
  "success": true,
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "dGhpcyBpcyBhIHJlZnJlc2g...",
    "expires_at": "2025-12-03T10:15:00Z"
  }
}
```
:::

#### Create Credential

``` http
POST /api/v1/credentials
Authorization: Bearer <access_token>
Content-Type: application/json

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

::: {#cb25 .sourceCode}
``` {.sourceCode .json}
{
  "success": true,
  "data": {
    "id": 1,
    "name": "Windows Admin",
    "credential_type": "winrm_ntlm",
    "username": "administrator",
    "domain": "CORP",
    "port": 5985,
    "use_ssl": false,
    "created_at": "2025-12-03T10:00:00Z"
  }
}
```
:::

#### Provision Device

``` http
POST /api/v1/devices/1/provision
Authorization: Bearer <access_token>
Content-Type: application/json

{
  "credential_id": 1,
  "polling_interval": 60
}
```

::: {#cb27 .sourceCode}
``` {.sourceCode .json}
{
  "success": true,
  "data": {
    "device_id": 1,
    "status": "provisioned",
    "message": "Device provisioned for monitoring"
  }
}
```
:::

#### Get Metrics

``` http
GET /api/v1/devices/1/metrics
Authorization: Bearer <access_token>
```

::: {#cb29 .sourceCode}
``` {.sourceCode .json}
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
:::

#### Error Response

::: {#cb30 .sourceCode}
``` {.sourceCode .json}
{
  "success": false,
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "Invalid IP address format",
    "details": {
      "field": "ip",
      "value": "999.999.999.999"
    }
  }
}
```
:::

------------------------------------------------------------------------

## 10. Configuration

### 10.1 Configuration Sources (Priority Order)

1.  **Command-line flags** (highest priority)
2.  **Environment variables**
3.  **Configuration file** (lowest priority)

### 10.2 Configuration File

::: {#cb31 .sourceCode}
``` {.sourceCode .yaml}
# config.yaml

server:
  host: "0.0.0.0"
  port: 8443
  read_timeout: "30s"
  write_timeout: "30s"
  tls:
    enabled: true
    cert_file: "/etc/nmslite/tls/server.crt"
    key_file: "/etc/nmslite/tls/server.key"

database:
  host: "localhost"
  port: 5432
  name: "nmslite"
  user: "nmslite"
  password: "${DB_PASSWORD}"  # From env
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: "5m"

auth:
  jwt_secret: "${JWT_SECRET}"  # From env
  access_token_ttl: "15m"
  refresh_token_ttl: "168h"
  bcrypt_cost: 12

queue:
  type: "nats"
  url: "nats://localhost:4222"
  embedded: true  # Run embedded NATS server

cache:
  device_ttl: "5m"
  credential_ttl: "5m"
  metrics_ttl: "30s"
  cleanup_interval: "1m"  # Background cleanup of expired entries

polling:
  default_interval: "60s"
  worker_count: 10
  timeout: "30s"

encryption:
  master_key: "${NMSLITE_MASTER_KEY}"  # From env

logging:
  level: "info"
  format: "json"
  output: "stdout"
```
:::

### 10.3 Environment Variables

  Variable                Description                 Default
  ----------------------- --------------------------- -----------------
  `NMSLITE_CONFIG`        Config file path            `./config.yaml`
  `NMSLITE_SERVER_PORT`   Server port                 `8443`
  `NMSLITE_DB_PASSWORD`   Database password           \-
  `NMSLITE_JWT_SECRET`    JWT signing secret          \-
  `NMSLITE_MASTER_KEY`    Credential encryption key   \-
  `NMSLITE_LOG_LEVEL`     Log level                   `info`

### 10.4 Command-Line Flags

::: {#cb32 .sourceCode}
``` {.sourceCode .bash}
nmslite \
  --config /etc/nmslite/config.yaml \
  --server.port 8443 \
  --log.level debug
```
:::

------------------------------------------------------------------------

## 11. Technology Stack

### 11.1 Core Dependencies

  ---------------------------------------------------------------------------
  Component                Library                   Rationale
  ------------------------ ------------------------- ------------------------
  **Language**             Go 1.21+                  Performance,
                                                     concurrency, single
                                                     binary

  **HTTP Router**          chi                       Lightweight, middleware
                                                     support, idiomatic

  **Database**             PostgreSQL 15+            Partitioning, JSONB
                                                     support, reliability

  **DB Driver**            pgx/v5                    Native Postgres,
                                                     connection pooling

  **Migrations**           golang-migrate            Version-controlled
                                                     schema changes

  **JWT**                  golang-jwt/jwt/v5         Standard JWT
                                                     implementation

  **Config**               viper                     Multi-source config
                                                     (file, env, flags)

  **Validation**           go-playground/validator   Struct tag validation

  **Logging**              log/slog (stdlib)         Structured logging, zero
                                                     deps

  **Queue**                nats-io/nats.go           Embedded or standalone,
                                                     JetStream

  **WinRM**                masterzen/winrm           WinRM protocol client

  **Encryption**           crypto/aes (stdlib)       AES-256-GCM for
                                                     credentials

  **Cache**                sync.RWMutex + map        Pure in-memory, zero
                           (stdlib)                  external deps
  ---------------------------------------------------------------------------

### 11.2 Project Structure

    nmslite/
    ├── cmd/
    │   └── nmslite/
    │       └── main.go              # Entry point
    ├── internal/
    │   ├── config/
    │   │   └── config.go            # Configuration loading
    │   ├── server/
    │   │   └── server.go            # HTTP server setup
    │   ├── handler/
    │   │   ├── auth.go
    │   │   ├── device.go
    │   │   ├── credential.go
    │   │   └── metrics.go
    │   ├── service/
    │   │   ├── auth.go
    │   │   ├── device.go
    │   │   ├── credential.go
    │   │   ├── metrics.go
    │   │   └── polling.go
    │   ├── repository/
    │   │   ├── user.go
    │   │   ├── device.go
    │   │   ├── credential.go
    │   │   └── metrics.go
    │   ├── collector/
    │   │   ├── collector.go         # Interface
    │   │   ├── registry.go          # Plugin registry
    │   │   ├── wmi.go               # WMI collector
    │   │   └── winrm.go             # WinRM collector
    │   ├── queue/
    │   │   ├── queue.go             # Interface
    │   │   └── nats.go              # NATS implementation
    │   ├── cache/
    │   │   ├── cache.go             # Interface
    │   │   └── memory.go            # In-memory implementation
    │   ├── crypto/
    │   │   └── crypto.go            # Encryption utilities
    │   └── model/
    │       ├── user.go
    │       ├── device.go
    │       ├── credential.go
    │       └── metrics.go
    ├── migrations/
    │   ├── 000001_init_schema.up.sql
    │   └── 000001_init_schema.down.sql
    ├── api/
    │   └── postman/
    │       └── NMSlite.postman_collection.json
    ├── config/
    │   └── config.example.yaml
    ├── go.mod
    ├── go.sum
    └── README.md

------------------------------------------------------------------------

## 12. Non-Functional Requirements

### 12.1 Performance Targets

  Metric                Target
  --------------------- --------------------
  API response (p95)    \< 100ms
  Metrics query (p95)   \< 500ms
  Poll cycle            \< 30s per device
  Concurrent workers    10-50 configurable

### 12.2 Scalability

  Dimension                 Target
  ------------------------- ---------------------------
  Devices                   100-500 (single instance)
  Metrics retention         90 days
  Concurrent API requests   100/sec

### 12.3 Security Requirements

  Requirement           Implementation
  --------------------- ------------------------
  Authentication        JWT (access + refresh)
  Transport             HTTPS/TLS 1.2+
  Credentials at rest   AES-256-GCM encrypted
  Secrets               Environment variables

### 12.4 Observability

  Aspect            Implementation
  ----------------- --------------------------
  Logging           Structured JSON (slog)
  Log levels        debug, info, warn, error
  Request tracing   Request ID in logs
  Health check      GET /health endpoint

------------------------------------------------------------------------

## Appendix A: Glossary

  Term             Definition
  ---------------- -------------------------------------------
  **Device**       A Windows machine being monitored
  **Credential**   Authentication details for WMI/WinRM
  **Provision**    Enable monitoring for a device
  **Poll**         Single metrics collection cycle
  **Collector**    Plugin that gathers metrics from a device

------------------------------------------------------------------------

## Appendix B: Decision Log

  -----------------------------------------------------------------------
  Decision            Rationale              Alternatives
  ------------------- ---------------------- ----------------------------
  NATS queue          Embeddable, Go-native, RabbitMQ, Kafka
                      simple                 

  Pure in-memory      Zero deps, stdlib      ristretto, Redis
  cache               only, sufficient scale 

  PostgreSQL          Partitioning,          TimescaleDB, SQLite
                      reliability            

  chi router          Lightweight, stdlib    gin, echo
                      compatible             

  slog logging        Stdlib, structured,    zap, zerolog
                      zero deps              

  Flat metrics table  Single INSERT per      JSONB arrays, normalized
                      poll, all scalar       tables
                      values, simple queries 

  System-wide         Simpler collection,    Per-disk/per-interface
  aggregates          smaller data,          breakdown
                      sufficient for health  
                      monitoring             

  Process count only  Simpler, less data,    Full process snapshots
                      privacy-friendly       
  -----------------------------------------------------------------------

------------------------------------------------------------------------

**Document Revision History**

  Version   Date       Changes
  --------- ---------- ----------------------
  1.0.0     Dec 2024   Initial architecture
