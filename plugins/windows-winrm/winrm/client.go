package winrm

import (
	"fmt"
	"strings"
	"time"

	"github.com/masterzen/winrm"
	"github.com/nmslite/plugins/windows-winrm/models"
)

// Client wraps the WinRM client for executing PowerShell commands
type Client struct {
	client *winrm.Client
	target string
}

// NewClient creates a WinRM client based on the provided credentials
// - If domain is empty, uses Basic Auth
// - If domain is provided, uses NTLM Auth
// - If use_https is true, uses HTTPS endpoint (typically port 5986)
func NewClient(target string, port int, creds models.Credentials, timeout time.Duration) (*Client, error) {
	endpoint := winrm.NewEndpoint(
		target,
		port,
		creds.UseHTTPS,
		true, // insecure - skip certificate verification
		nil,  // CA certificate
		nil,  // client certificate
		nil,  // client key
		timeout,
	)

	var client *winrm.Client
	var err error

	if creds.Domain != "" {
		// NTLM authentication with domain
		params := winrm.DefaultParameters
		params.TransportDecorator = func() winrm.Transporter {
			return &winrm.ClientNTLM{}
		}
		client, err = winrm.NewClientWithParameters(
			endpoint,
			fmt.Sprintf("%s\\%s", creds.Domain, creds.Username),
			creds.Password,
			params,
		)
	} else {
		// Basic authentication
		client, err = winrm.NewClient(endpoint, creds.Username, creds.Password)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create WinRM client: %w", err)
	}

	return &Client{
		client: client,
		target: target,
	}, nil
}

// RunPowerShell executes a PowerShell command and returns the stdout output
func (c *Client) RunPowerShell(script string) (string, error) {
	// Wrap script in PowerShell execution
	psCmd := fmt.Sprintf("powershell.exe -NoProfile -NonInteractive -Command \"%s\"",
		strings.ReplaceAll(script, "\"", "`\""))

	stdout, stderr, exitCode, err := c.client.RunWithString(psCmd, "")
	if err != nil {
		return "", fmt.Errorf("WinRM execution failed: %w", err)
	}

	if exitCode != 0 {
		return "", fmt.Errorf("PowerShell command failed (exit code %d): %s", exitCode, stderr)
	}

	return strings.TrimSpace(stdout), nil
}

// RunPowerShellRaw executes a raw PowerShell script without additional wrapping
func (c *Client) RunPowerShellRaw(script string) (string, error) {
	stdout, stderr, exitCode, err := c.client.RunWithString(script, "")
	if err != nil {
		return "", fmt.Errorf("WinRM execution failed: %w", err)
	}

	if exitCode != 0 {
		return "", fmt.Errorf("command failed (exit code %d): %s", exitCode, stderr)
	}

	return strings.TrimSpace(stdout), nil
}

// Target returns the target hostname/IP
func (c *Client) Target() string {
	return c.target
}

// Close is a no-op for the WinRM client as connections are per-request
// Kept for interface consistency
func (c *Client) Close() {
	// WinRM connections are stateless/per-request, no cleanup needed
}
