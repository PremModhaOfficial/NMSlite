# NMSlite Protocol & Handshake Details

## Protocol-Specific Handshake Analysis

### 1. WinRM (Windows Remote Management)

#### Discovery Phase (Current)
```
┌─────────────────────────────────────────┐
│ 1. TCP Port Liveness Check               │
│    net.Dialer.DialContext(target:5985) │
│    ├─ Timeout: 1s (configurable)        │
│    ├─ Result: Open/Closed               │
│    └─ Action: Save to discovered_devices│
└─────────────────────────────────────────┘
```

**What's Missing**:
- No WinRM protocol handshake
- No credential validation
- No HTTP response parsing (WinRM uses HTTP(S) underneath)
- No version negotiation

#### Actual Connection Phase (Polling)
```
┌──────────────────────────────────────────────────────────────┐
│ Plugin Binary Execution (plugins/windows-winrm/main.go)       │
└──────────────────────────────────────────────────────────────┘
    ↓
┌──────────────────────────────────────────────────────────────┐
│ 1. Create WinRM Client (winrm/client.go:NewClient)          │
│    ├─ Creates endpoint (host, port, HTTPS, timeout)         │
│    ├─ Selects auth based on domain:                          │
│    │  ├─ With domain: NTLM (format: DOMAIN\USERNAME)        │
│    │  └─ No domain: Basic Auth                              │
│    └─ Returns masterzen/winrm.Client                         │
└──────────────────────────────────────────────────────────────┘
    ↓
┌──────────────────────────────────────────────────────────────┐
│ 2. Execute PowerShell Commands                               │
│    ├─ WinRM HTTP(S) Protocol Handshake:                      │
│    │  ├─ Connect to http(s)://target:port/wsman             │
│    │  ├─ Send WinRM SOAP envelope                           │
│    │  ├─ Authenticate (Basic/NTLM)                          │
│    │  └─ Receive SOAP response                              │
│    ├─ PowerShell script execution                            │
│    └─ Parse stdout/stderr                                   │
└──────────────────────────────────────────────────────────────┘
    ↓
┌──────────────────────────────────────────────────────────────┐
│ 3. Collect Metrics                                           │
│    ├─ CPU, Memory, Disk, Network metrics                    │
│    └─ Return JSON array of metrics                          │
└──────────────────────────────────────────────────────────────┘
```

#### WinRM Protocol Details
- **Transport**: HTTP/HTTPS
- **Port**: 5985 (HTTP) / 5986 (HTTPS)
- **Protocol**: WS-Management (WS-MAN)
- **Message Format**: SOAP XML
- **Authentication Methods**:
  - Basic Auth: Base64(username:password)
  - NTLM: NTLM handshake
  - Kerberos: (available but not currently used in code)
- **Certificate Validation**: Currently disabled (insecure=true in NewEndpoint)
- **Library Used**: github.com/masterzen/winrm
- **PowerShell Version**: No specific negotiation, assumes PS 2.0+

#### Credential Validation Requirements
For WinRM discovery to work properly, we need:
1. Port 5985 or 5986 open and responding to HTTP(S)
2. WinRM service enabled on target
3. Valid Windows credentials with remote access permissions
4. If NTLM used: Domain accessible from scanning machine
5. Network connectivity for SOAP message exchange

#### Currently NOT Validated During Discovery
- Credentials at all (TCP only)
- WinRM service availability
- Authentication method compatibility
- Certificate validity (if HTTPS)
- PowerShell availability on target
- Metric collection permissions

---

### 2. SSH (Linux/Unix)

#### Discovery Phase (Current)
```
┌─────────────────────────────────────────┐
│ TCP Port Liveness Check (Port 22)       │
│ net.Dialer.DialContext(target:22)       │
│ └─ No SSH protocol handshake            │
└─────────────────────────────────────────┘
```

#### Expected Handshake (During Polling - NOT IMPLEMENTED)
```
┌──────────────────────────────────────────────────────────────┐
│ SSH Protocol Handshake (RFC 4253)                             │
├──────────────────────────────────────────────────────────────┤
│ 1. TCP Connect                                               │
│    └─ TCP socket established                                 │
├──────────────────────────────────────────────────────────────┤
│ 2. Protocol Version Exchange                                 │
│    ├─ Server → Client: SSH-2.0-<ServerVersion>              │
│    └─ Client → Server: SSH-2.0-<ClientVersion>              │
├──────────────────────────────────────────────────────────────┤
│ 3. Key Exchange                                              │
│    ├─ Exchange algorithms (encryption, MAC, compression)    │
│    ├─ Diffie-Hellman (DH) or ECDH key exchange              │
│    └─ Compute session keys                                  │
├──────────────────────────────────────────────────────────────┤
│ 4. Service Request                                           │
│    └─ Client → Server: ssh-userauth                         │
├──────────────────────────────────────────────────────────────┤
│ 5. Authentication                                            │
│    ├─ Method 1: Password auth                               │
│    │  ├─ Client → Server: username + password               │
│    │  ├─ Server: Validate against PAM/etc                   │
│    │  └─ Server → Client: SSH_MSG_USERAUTH_SUCCESS          │
│    ├─ Method 2: Public Key auth                             │
│    │  ├─ Client → Server: username + public_key             │
│    │  ├─ Server: Check authorized_keys                      │
│    │  ├─ Client → Server: Signature proof                   │
│    │  └─ Server → Client: SSH_MSG_USERAUTH_SUCCESS          │
│    └─ Method 3: Other (Kerberos, etc.)                      │
├──────────────────────────────────────────────────────────────┤
│ 6. Channel Opening                                           │
│    ├─ Client → Server: SSH_MSG_CHANNEL_OPEN (type: session) │
│    ├─ Server → Client: SSH_MSG_CHANNEL_OPEN_CONFIRMATION    │
│    └─ Channel ID assigned                                   │
├──────────────────────────────────────────────────────────────┤
│ 7. Subsystem Request (for SFTP/Commands)                     │
│    ├─ If subsystem: SSH_MSG_CHANNEL_REQUEST (subsystem)     │
│    └─ If shell: SSH_MSG_CHANNEL_REQUEST (shell)             │
└──────────────────────────────────────────────────────────────┘
```

#### SSH Plugin Status
- **No SSH plugin currently exists** in the codebase
- Protocol registered but no implementation
- Would need to be added (using golang.org/x/crypto/ssh or similar)

#### Credential Validation for SSH
What should be validated:
1. Port 22 (or custom port) responding to SSH protocol
2. Banner exchange successful
3. Supported key exchange algorithms match
4. Authentication method available (password vs key)
5. Username/password or private key valid
6. User has shell access permissions

---

### 3. SNMP v2c (Simple Network Management Protocol)

#### Discovery Phase (Current)
```
┌─────────────────────────────────────────┐
│ TCP Port Liveness Check (Port 161)      │
│ net.Dialer.DialContext(target:161)      │
│ ├─ SNMP uses UDP, not TCP!              │
│ └─ This will FAIL for SNMP targets      │
└─────────────────────────────────────────┘
```

#### Correct SNMP Handshake (Should Be)
```
┌──────────────────────────────────────────────────────────────┐
│ SNMP v2c Protocol Handshake (RFC 1901/1905)                  │
├──────────────────────────────────────────────────────────────┤
│ Transport: UDP (not TCP)                                     │
│ Port: 161 (SNMP agent) or 162 (SNMP traps)                   │
├──────────────────────────────────────────────────────────────┤
│ Message Structure:                                           │
│ ├─ Version: 1 (SNMPv1) or 1 (SNMPv2c) [0=v1, 1=v2c]        │
│ ├─ Community: "public" (default, case-sensitive)            │
│ ├─ PDU Type: GetRequest (0xA0)                               │
│ ├─ Request ID: 1                                             │
│ ├─ Error Status: 0 (noError)                                 │
│ ├─ Error Index: 0                                            │
│ └─ Variable Bindings: [ sysDescr.0 = null ]                  │
│    (OID: 1.3.6.1.2.1.1.1.0)                                  │
├──────────────────────────────────────────────────────────────┤
│ Handshake Steps:                                             │
│ 1. Build SNMP GetRequest message                             │
│ 2. Encode with community string                              │
│ 3. Send UDP packet to target:161                             │
│ 4. Receive GetResponse within timeout (e.g., 2s)            │
│ 5. Validate:                                                 │
│    ├─ Response is GetResponse (0xA2)                         │
│    ├─ Request ID matches                                     │
│    ├─ Error status = 0                                       │
│    ├─ At least one variable binding in response              │
│    └─ Community string matches (for validation)              │
└──────────────────────────────────────────────────────────────┘
```

#### SNMP Plugin Status
- **No SNMP plugin currently exists** in the codebase
- Protocol registered but no implementation
- **Discovery will fail**: TCP check won't find UDP SNMP services
- Would need: UDP port scanner + SNMP GetRequest validation

#### Credential Validation for SNMP v2c
What should be validated:
1. UDP port 161 responding to SNMP
2. SNMP agent present and responding
3. Community string accepted
4. Specific OIDs are readable with provided community
5. No authentication failures (bad community = SNMP error)

---

## 4. Handshake Implementation Gap Summary

| Aspect | WinRM | SSH | SNMP v2c |
|--------|-------|-----|----------|
| Plugin Exists | Yes | No | No |
| Discovery Phase | TCP only | TCP only | TCP (wrong!) |
| Protocol Handshake | During polling | Not implemented | Not implemented |
| Credential Validation | During polling | N/A | N/A |
| Port Type | TCP | TCP | UDP |
| Transport | HTTP/HTTPS | SSH | UDP |
| Auth During Discovery | No | No | No (should be: community test) |

---

## 5. Code Locations

### Protocol Definitions
```
internal/protocols/credentials.go
├─ WinRMCredentials struct
├─ SSHCredentials struct (has custom Validate())
└─ SNMPCredentials struct
```

### Plugin Implementation
```
plugins/windows-winrm/
├─ main.go (entry point, processes PollTask array)
├─ manifest.json (metadata)
├─ winrm/client.go (WinRM HTTP(S) connection)
├─ collector/ (metric collection)
│  ├─ cpu.go
│  ├─ memory.go
│  ├─ disk.go
│  └─ network.go
└─ models/ (data structures)

plugins/ssh/ (DOES NOT EXIST)
plugins/snmp/ (DOES NOT EXIST)
```

### Discovery Logic
```
internal/discovery/worker.go
├─ executeDiscovery() - Main flow
├─ isPortOpen() - TCP dial check only
└─ plugins.Registry.GetByPort() - Find plugin to auto-provision
```

---

## 6. Recommended Improvements

### Phase 1: Protocol Handshake Validation
1. Add optional protocol handshake validation during discovery
2. Create HandshakeValidator interface per plugin
3. Implement for WinRM (test SOAP endpoint)
4. Implement UDP check for SNMP

### Phase 2: Credential Testing
1. Add test connection endpoint: POST /api/v1/credentials/{id}/test
2. Validate credentials before saving
3. Return detailed error messages

### Phase 3: Discovery Enhancements
1. Detect protocol type per port (e.g., SSH banner greeting)
2. Store handshake results in discovered_devices
3. Track failure reasons (auth failed vs. protocol unsupported)
4. Add retry logic with exponential backoff

### Phase 4: Plugin Completeness
1. Implement SSH plugin with key-based auth support
2. Implement SNMP v2c plugin with UDP scanning
3. Add SNMP v3 support
4. Add Kerberos auth for WinRM
