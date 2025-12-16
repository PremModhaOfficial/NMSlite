package discovery

import (
	"fmt"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/masterzen/winrm"
	"github.com/nmslite/nmslite/internal/api/auth"
	"golang.org/x/crypto/ssh"
)

// HandshakeResult represents the outcome of a protocol handshake
type HandshakeResult struct {
	Success  bool
	Protocol string
	Error    string
	Hostname string
	Metadata map[string]interface{}
}

// ValidateSSH attempts SSH handshake with password or key auth
// Uses golang.org/x/crypto/ssh
func ValidateSSH(target string, port int, creds *auth.Credentials, timeout time.Duration) (*HandshakeResult, error) {
	address := fmt.Sprintf("%s:%d", target, port)

	// Build auth methods
	var authMethods []ssh.AuthMethod

	// Try password auth first if provided
	if creds.Password != "" {
		authMethods = append(authMethods, ssh.Password(creds.Password))
	}

	// Try key auth if provided
	if creds.PrivateKey != "" {
		var key ssh.Signer
		var err error

		if creds.Passphrase != "" {
			key, err = ssh.ParsePrivateKeyWithPassphrase([]byte(creds.PrivateKey), []byte(creds.Passphrase))
		} else {
			key, err = ssh.ParsePrivateKey([]byte(creds.PrivateKey))
		}

		if err != nil {
			return &HandshakeResult{
				Success:  false,
				Protocol: "ssh",
				Error:    fmt.Sprintf("failed to parse private key: %v", err),
			}, nil
		}

		authMethods = append(authMethods, ssh.PublicKeys(key))
	}

	if len(authMethods) == 0 {
		return &HandshakeResult{
			Success:  false,
			Protocol: "ssh",
			Error:    "no authentication method provided (password or private_key required)",
		}, nil
	}

	config := &ssh.ClientConfig{
		User:            creds.Username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Validation happens at connection time
		Timeout:         timeout,
	}

	client, err := ssh.Dial("tcp", address, config)
	if err != nil {
		return &HandshakeResult{
			Success:  false,
			Protocol: "ssh",
			Error:    fmt.Sprintf("SSH handshake failed: %v", err),
		}, nil
	}
	defer client.Close()

	// Try to fetch hostname
	var hostname string
	session, err := client.NewSession()
	if err == nil {
		defer session.Close()
		if out, err := session.Output("hostname"); err == nil {
			hostname = strings.TrimSpace(string(out))
		}
	}

	return &HandshakeResult{
		Success:  true,
		Protocol: "ssh",
		Hostname: hostname,
		Metadata: map[string]interface{}{
			"remote_version": string(client.ServerVersion()),
		},
	}, nil
}

// ValidateWinRM attempts WinRM handshake (NTLM or Basic)
// Uses github.com/masterzen/winrm
func ValidateWinRM(target string, port int, creds *auth.Credentials, timeout time.Duration) (*HandshakeResult, error) {
	endpoint := winrm.NewEndpoint(target, port, false, false, nil, nil, nil, timeout)

	client, err := winrm.NewClient(endpoint, creds.Username, creds.Password)
	if err != nil {
		return &HandshakeResult{
			Success:  false,
			Protocol: "winrm",
			Error:    fmt.Sprintf("WinRM client creation failed: %v", err),
		}, nil
	}

	// Attempt to create a shell for validation
	shell, err := client.CreateShell()
	if err != nil {
		return &HandshakeResult{
			Success:  false,
			Protocol: "winrm",
			Error:    fmt.Sprintf("WinRM shell creation failed: %v", err),
		}, nil
	}
	defer shell.Close()

	// Try to fetch hostname using the shell
	var hostname string
	// Using RunWithString to execute a simple command
	stdout, _, _, err := client.RunWithString("hostname", "")
	if err == nil {
		hostname = strings.TrimSpace(stdout)
	}

	return &HandshakeResult{
		Success:  true,
		Protocol: "winrm",
		Hostname: hostname,
		Metadata: map[string]interface{}{
			"shell_id": "unknown", // shell ID not exposed in current winrm API
		},
	}, nil
}

// ValidateSNMPv2c attempts SNMP v2c handshake with community string
// Uses github.com/gosnmp/gosnmp - UDP GetRequest to sysDescr OID
func ValidateSNMPv2c(target string, port int, creds *auth.Credentials, timeout time.Duration) (*HandshakeResult, error) {
	g := &gosnmp.GoSNMP{
		Target:    target,
		Port:      uint16(port),
		Version:   gosnmp.Version2c,
		Community: creds.Community,
		Timeout:   timeout,
	}

	err := g.Connect()
	if err != nil {
		return &HandshakeResult{
			Success:  false,
			Protocol: "snmp-v2c",
			Error:    fmt.Sprintf("SNMP connection failed: %v", err),
		}, nil
	}
	defer g.Conn.Close()

	// Perform a GetRequest to sysDescr (1.3.6.1.2.1.1.1.0) and sysName (1.3.6.1.2.1.1.5.0)
	oids := []string{"1.3.6.1.2.1.1.1.0", "1.3.6.1.2.1.1.5.0"}
	result, err := g.Get(oids)
	if err != nil {
		return &HandshakeResult{
			Success:  false,
			Protocol: "snmp-v2c",
			Error:    fmt.Sprintf("SNMP Get request failed: %v", err),
		}, nil
	}

	// Extract values
	var sysDescr, hostname string
	for _, variable := range result.Variables {
		switch variable.Name {
		case ".1.3.6.1.2.1.1.1.0":
			sysDescr = fmt.Sprintf("%v", variable.Value)
		case ".1.3.6.1.2.1.1.5.0":
			hostname = fmt.Sprintf("%v", variable.Value)
		}
	}

	return &HandshakeResult{
		Success:  true,
		Protocol: "snmp-v2c",
		Hostname: hostname,
		Metadata: map[string]interface{}{
			"sysDescr": sysDescr,
		},
	}, nil
}

// ValidateSNMPv3 attempts SNMP v3 handshake with USM auth
// Uses github.com/gosnmp/gosnmp - supports noAuthNoPriv, authNoPriv, authPriv
func ValidateSNMPv3(target string, port int, creds *auth.Credentials, timeout time.Duration) (*HandshakeResult, error) {
	g := &gosnmp.GoSNMP{
		Target:  target,
		Port:    uint16(port),
		Version: gosnmp.Version3,
		Timeout: timeout,
	}

	// Parse security level
	var securityLevel gosnmp.SnmpV3MsgFlags
	switch creds.SecurityLevel {
	case "noAuthNoPriv":
		securityLevel = gosnmp.NoAuthNoPriv
	case "authNoPriv":
		securityLevel = gosnmp.AuthNoPriv
	case "authPriv":
		securityLevel = gosnmp.AuthPriv
	default:
		return &HandshakeResult{
			Success:  false,
			Protocol: "snmp-v3",
			Error:    fmt.Sprintf("invalid security level: %s", creds.SecurityLevel),
		}, nil
	}

	// Parse auth protocol
	var authProto gosnmp.SnmpV3AuthProtocol
	switch creds.AuthProtocol {
	case "MD5":
		authProto = gosnmp.MD5
	case "SHA":
		authProto = gosnmp.SHA
	case "SHA224":
		authProto = gosnmp.SHA224
	case "SHA256":
		authProto = gosnmp.SHA256
	case "SHA384":
		authProto = gosnmp.SHA384
	case "SHA512":
		authProto = gosnmp.SHA512
	default:
		authProto = gosnmp.MD5 // default
	}

	// Parse privacy protocol
	var privProto gosnmp.SnmpV3PrivProtocol
	switch creds.PrivProtocol {
	case "DES":
		privProto = gosnmp.DES
	case "AES":
		privProto = gosnmp.AES
	case "AES192":
		privProto = gosnmp.AES192
	case "AES256":
		privProto = gosnmp.AES256
	default:
		privProto = gosnmp.NoPriv // default
	}

	// Build security parameters
	switch securityLevel {
	case gosnmp.NoAuthNoPriv:
		g.SecurityModel = gosnmp.UserSecurityModel
		g.MsgFlags = securityLevel
		g.SecurityParameters = &gosnmp.UsmSecurityParameters{
			UserName: creds.SecurityName,
		}
	case gosnmp.AuthNoPriv:
		g.SecurityModel = gosnmp.UserSecurityModel
		g.MsgFlags = securityLevel
		g.SecurityParameters = &gosnmp.UsmSecurityParameters{
			UserName:                 creds.SecurityName,
			AuthenticationProtocol:   authProto,
			AuthenticationPassphrase: creds.AuthPassword,
		}
		//goland:noinspection GoDfaConstantCondition
	case gosnmp.AuthPriv:
		g.SecurityModel = gosnmp.UserSecurityModel
		g.MsgFlags = securityLevel
		g.SecurityParameters = &gosnmp.UsmSecurityParameters{
			UserName:                 creds.SecurityName,
			AuthenticationProtocol:   authProto,
			AuthenticationPassphrase: creds.AuthPassword,
			PrivacyProtocol:          privProto,
			PrivacyPassphrase:        creds.PrivPassword,
		}
	}

	err := g.Connect()
	if err != nil {
		return &HandshakeResult{
			Success:  false,
			Protocol: "snmp-v3",
			Error:    fmt.Sprintf("SNMP v3 connection failed: %v", err),
		}, nil
	}
	defer g.Conn.Close()

	// Perform a GetRequest to sysDescr (1.3.6.1.2.1.1.1.0) and sysName (1.3.6.1.2.1.1.5.0)
	oids := []string{"1.3.6.1.2.1.1.1.0", "1.3.6.1.2.1.1.5.0"}
	result, err := g.Get(oids)
	if err != nil {
		return &HandshakeResult{
			Success:  false,
			Protocol: "snmp-v3",
			Error:    fmt.Sprintf("SNMP v3 Get request failed: %v", err),
		}, nil
	}

	// Extract values
	var sysDescr, hostname string
	for _, variable := range result.Variables {
		switch variable.Name {
		case ".1.3.6.1.2.1.1.1.0":
			sysDescr = fmt.Sprintf("%v", variable.Value)
		case ".1.3.6.1.2.1.1.5.0":
			hostname = fmt.Sprintf("%v", variable.Value)
		}
	}

	return &HandshakeResult{
		Success:  true,
		Protocol: "snmp-v3",
		Hostname: hostname,
		Metadata: map[string]interface{}{
			"sysDescr":       sysDescr,
			"security_level": creds.SecurityLevel,
		},
	}, nil
}
