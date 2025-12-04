package store

import (
	"sync"
	"time"

	"github.com/nmslite/nmslite/internal/model"
)

// MockStore holds all mock data in memory
type MockStore struct {
	mu          sync.RWMutex
	users       map[int]*model.User
	credentials map[int]*model.Credential
	devices     map[int]*model.Device
	metrics     map[int][]*model.DeviceMetrics
	nextUserID  int
	nextCredID  int
	nextDevID   int
}

// NewMockStore creates a new mock store with seed data
func NewMockStore() *MockStore {
	store := &MockStore{
		users:       make(map[int]*model.User),
		credentials: make(map[int]*model.Credential),
		devices:     make(map[int]*model.Device),
		metrics:     make(map[int][]*model.DeviceMetrics),
		nextUserID:  2,
		nextCredID:  2,
		nextDevID:   2,
	}

	// Seed with demo data
	store.users[1] = &model.User{
		ID:        1,
		Username:  "admin",
		Role:      "admin",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	store.credentials[1] = &model.Credential{
		ID:             1,
		Name:           "Default WinRM Credential",
		CredentialType: "winrm_basic",
		Username:       "administrator",
		Domain:         "WORKGROUP",
		Port:           5985,
		UseSSL:         false,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	store.devices[1] = &model.Device{
		ID:              1,
		IP:              "192.168.1.100",
		Hostname:        "SERVER-01",
		OS:              "Windows Server 2022",
		Status:          "discovered",
		PollingInterval: 60,
		LastSeen:        time.Now(),
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Seed metrics
	store.metrics[1] = []*model.DeviceMetrics{
		{
			DeviceID:  1,
			Timestamp: time.Now().Add(-5 * time.Minute),
			CPU:       model.CPUMetrics{UsagePercent: 45.5},
			Memory: model.MemoryMetrics{
				TotalBytes:   17179869184,
				UsedBytes:    8589934592,
				UsagePercent: 50.0,
			},
			Disk: model.DiskMetrics{
				TotalBytes:   500107862016,
				UsedBytes:    250053931008,
				UsagePercent: 50.0,
			},
			Network: model.NetworkMetrics{
				BytesSentPerSec:    1048576,
				BytesRecvPerSec:    2097152,
				UtilizationPercent: 25.0,
				PacketsSent:        1000000,
				PacketsRecv:        2000000,
				Errors:             0,
				Dropped:            5,
			},
			ProcessCount: 142,
		},
	}

	return store
}

// GetUser retrieves a user by ID
func (s *MockStore) GetUser(id int) *model.User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.users[id]
}

// GetUserByUsername retrieves a user by username
func (s *MockStore) GetUserByUsername(username string) *model.User {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, user := range s.users {
		if user.Username == username {
			return user
		}
	}
	return nil
}

// CreateCredential creates a new credential
func (s *MockStore) CreateCredential(cred *model.Credential) *model.Credential {
	s.mu.Lock()
	defer s.mu.Unlock()
	cred.ID = s.nextCredID
	cred.CreatedAt = time.Now()
	cred.UpdatedAt = time.Now()
	s.credentials[cred.ID] = cred
	s.nextCredID++
	return cred
}

// GetCredential retrieves a credential by ID
func (s *MockStore) GetCredential(id int) *model.Credential {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.credentials[id]
}

// ListCredentials returns all credentials
func (s *MockStore) ListCredentials() []*model.Credential {
	s.mu.RLock()
	defer s.mu.RUnlock()
	creds := make([]*model.Credential, 0, len(s.credentials))
	for _, cred := range s.credentials {
		creds = append(creds, cred)
	}
	return creds
}

// UpdateCredential updates an existing credential
func (s *MockStore) UpdateCredential(id int, updates *model.Credential) *model.Credential {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cred, ok := s.credentials[id]; ok {
		if updates.Name != "" {
			cred.Name = updates.Name
		}
		if updates.CredentialType != "" {
			cred.CredentialType = updates.CredentialType
		}
		if updates.Username != "" {
			cred.Username = updates.Username
		}
		if updates.Domain != "" {
			cred.Domain = updates.Domain
		}
		if updates.Port != 0 {
			cred.Port = updates.Port
		}
		cred.UseSSL = updates.UseSSL
		cred.UpdatedAt = time.Now()
		return cred
	}
	return nil
}

// DeleteCredential deletes a credential
func (s *MockStore) DeleteCredential(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.credentials[id]; ok {
		delete(s.credentials, id)
		return true
	}
	return false
}

// CreateDevice creates a new device
func (s *MockStore) CreateDevice(device *model.Device) *model.Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	device.ID = s.nextDevID
	device.Status = "discovered"
	device.LastSeen = time.Now()
	device.CreatedAt = time.Now()
	device.UpdatedAt = time.Now()
	s.devices[device.ID] = device
	s.nextDevID++
	return device
}

// GetDevice retrieves a device by ID
func (s *MockStore) GetDevice(id int) *model.Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.devices[id]
}

// ListDevices returns all devices
func (s *MockStore) ListDevices() []*model.Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	devices := make([]*model.Device, 0, len(s.devices))
	for _, dev := range s.devices {
		devices = append(devices, dev)
	}
	return devices
}

// UpdateDevice updates an existing device
func (s *MockStore) UpdateDevice(id int, updates *model.Device) *model.Device {
	s.mu.Lock()
	defer s.mu.Unlock()
	if device, ok := s.devices[id]; ok {
		if updates.Hostname != "" {
			device.Hostname = updates.Hostname
		}
		if updates.OS != "" {
			device.OS = updates.OS
		}
		if updates.Status != "" {
			device.Status = updates.Status
		}
		if updates.PollingInterval > 0 {
			device.PollingInterval = updates.PollingInterval
		}
		device.LastSeen = time.Now()
		device.UpdatedAt = time.Now()
		return device
	}
	return nil
}

// DeleteDevice deletes a device
func (s *MockStore) DeleteDevice(id int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.devices[id]; ok {
		delete(s.devices, id)
		delete(s.metrics, id)
		return true
	}
	return false
}

// AddMetrics adds new metrics for a device
func (s *MockStore) AddMetrics(deviceID int, metrics *model.DeviceMetrics) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.metrics[deviceID]; !ok {
		s.metrics[deviceID] = make([]*model.DeviceMetrics, 0)
	}
	s.metrics[deviceID] = append(s.metrics[deviceID], metrics)
}

// GetLatestMetrics returns the latest metrics for a device
func (s *MockStore) GetLatestMetrics(deviceID int) *model.DeviceMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if metrics, ok := s.metrics[deviceID]; ok && len(metrics) > 0 {
		return metrics[len(metrics)-1]
	}
	return nil
}

// GetMetricsHistory returns historical metrics for a device
func (s *MockStore) GetMetricsHistory(deviceID int, limit int) []*model.DeviceMetrics {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if metrics, ok := s.metrics[deviceID]; ok {
		if limit > len(metrics) {
			limit = len(metrics)
		}
		// Return last 'limit' entries
		return metrics[len(metrics)-limit:]
	}
	return nil
}
