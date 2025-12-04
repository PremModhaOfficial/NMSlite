package handler

import "encoding/json"

// Response is the standard API response envelope
type Response struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data,omitempty"`
	Error   *ErrorResponse  `json:"error,omitempty"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

// LoginRequest is the request body for login
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// CreateCredentialRequest is the request body for creating a credential
type CreateCredentialRequest struct {
	Name           string `json:"name"`
	CredentialType string `json:"credential_type"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	Domain         string `json:"domain,omitempty"`
	Port           int    `json:"port,omitempty"`
	UseSSL         bool   `json:"use_ssl"`
}

// UpdateCredentialRequest is the request body for updating a credential
type UpdateCredentialRequest struct {
	Name           string `json:"name,omitempty"`
	CredentialType string `json:"credential_type,omitempty"`
	Username       string `json:"username,omitempty"`
	Password       string `json:"password,omitempty"`
	Domain         string `json:"domain,omitempty"`
	Port           int    `json:"port,omitempty"`
	UseSSL         bool   `json:"use_ssl,omitempty"`
}

// CreateDeviceRequest is the request body for creating a device
type CreateDeviceRequest struct {
	IP              string `json:"ip"`
	Hostname        string `json:"hostname,omitempty"`
	OS              string `json:"os,omitempty"`
	PollingInterval int    `json:"polling_interval,omitempty"`
}

// UpdateDeviceRequest is the request body for updating a device
type UpdateDeviceRequest struct {
	Hostname        string `json:"hostname,omitempty"`
	OS              string `json:"os,omitempty"`
	Status          string `json:"status,omitempty"`
	PollingInterval int    `json:"polling_interval,omitempty"`
}

// ProvisionRequest is the request body for provisioning a device
type ProvisionRequest struct {
	CredentialID    int `json:"credential_id"`
	PollingInterval int `json:"polling_interval,omitempty"`
}

// DiscoverRequest is the request body for discovering devices
type DiscoverRequest struct {
	Subnet string `json:"subnet"`
}

// HistoryRequest is the request body for querying metrics history
type HistoryRequest struct {
	StartTime string `json:"start_time,omitempty"`
	EndTime   string `json:"end_time,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}
