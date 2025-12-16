package api

import (
	"context"

	"github.com/google/uuid"
	"github.com/nmslite/nmslite/internal/database/dbgen"
)

// MockQuerier is a mock implementation of dbgen.Querier
type MockQuerier struct {
	// Add hooks for methods we need to test
	CreateCredentialProfileFunc           func(ctx context.Context, arg dbgen.CreateCredentialProfileParams) (dbgen.CredentialProfile, error)
	GetCredentialProfileFunc              func(ctx context.Context, id uuid.UUID) (dbgen.CredentialProfile, error)
	ListCredentialProfilesFunc            func(ctx context.Context) ([]dbgen.CredentialProfile, error)
	UpdateCredentialProfileFunc           func(ctx context.Context, arg dbgen.UpdateCredentialProfileParams) (dbgen.CredentialProfile, error)
	DeleteCredentialProfileFunc           func(ctx context.Context, id uuid.UUID) error
	CreateMonitorFunc                     func(ctx context.Context, arg dbgen.CreateMonitorParams) (dbgen.Monitor, error)
	GetMonitorFunc                        func(ctx context.Context, id uuid.UUID) (dbgen.Monitor, error)
	ListMonitorsFunc                      func(ctx context.Context) ([]dbgen.Monitor, error)
	UpdateMonitorFunc                     func(ctx context.Context, arg dbgen.UpdateMonitorParams) (dbgen.Monitor, error)
	DeleteMonitorFunc                     func(ctx context.Context, id uuid.UUID) error
	GetExistingMonitorIDsFunc             func(ctx context.Context, ids []uuid.UUID) ([]uuid.UUID, error)
	GetLatestMetricsByDeviceAndPrefixFunc func(ctx context.Context, arg dbgen.GetLatestMetricsByDeviceAndPrefixParams) ([]dbgen.Metric, error)
	GetMetricsByDeviceAndPrefixFunc       func(ctx context.Context, arg dbgen.GetMetricsByDeviceAndPrefixParams) ([]dbgen.Metric, error)
}

// Implement specific methods used in tests
func (m *MockQuerier) CreateCredentialProfile(ctx context.Context, arg dbgen.CreateCredentialProfileParams) (dbgen.CredentialProfile, error) {
	if m.CreateCredentialProfileFunc != nil {
		return m.CreateCredentialProfileFunc(ctx, arg)
	}
	return dbgen.CredentialProfile{}, nil
}

func (m *MockQuerier) GetCredentialProfile(ctx context.Context, id uuid.UUID) (dbgen.CredentialProfile, error) {
	if m.GetCredentialProfileFunc != nil {
		return m.GetCredentialProfileFunc(ctx, id)
	}
	return dbgen.CredentialProfile{}, nil
}

func (m *MockQuerier) ListCredentialProfiles(ctx context.Context) ([]dbgen.CredentialProfile, error) {
	if m.ListCredentialProfilesFunc != nil {
		return m.ListCredentialProfilesFunc(ctx)
	}
	return nil, nil
}

func (m *MockQuerier) UpdateCredentialProfile(ctx context.Context, arg dbgen.UpdateCredentialProfileParams) (dbgen.CredentialProfile, error) {
	if m.UpdateCredentialProfileFunc != nil {
		return m.UpdateCredentialProfileFunc(ctx, arg)
	}
	return dbgen.CredentialProfile{}, nil
}

func (m *MockQuerier) DeleteCredentialProfile(ctx context.Context, id uuid.UUID) error {
	if m.DeleteCredentialProfileFunc != nil {
		return m.DeleteCredentialProfileFunc(ctx, id)
	}
	return nil
}

func (m *MockQuerier) CreateMonitor(ctx context.Context, arg dbgen.CreateMonitorParams) (dbgen.Monitor, error) {
	if m.CreateMonitorFunc != nil {
		return m.CreateMonitorFunc(ctx, arg)
	}
	return dbgen.Monitor{}, nil
}

func (m *MockQuerier) GetMonitor(ctx context.Context, id uuid.UUID) (dbgen.Monitor, error) {
	if m.GetMonitorFunc != nil {
		return m.GetMonitorFunc(ctx, id)
	}
	return dbgen.Monitor{}, nil
}

func (m *MockQuerier) ListMonitors(ctx context.Context) ([]dbgen.Monitor, error) {
	if m.ListMonitorsFunc != nil {
		return m.ListMonitorsFunc(ctx)
	}
	return nil, nil
}

func (m *MockQuerier) UpdateMonitor(ctx context.Context, arg dbgen.UpdateMonitorParams) (dbgen.Monitor, error) {
	if m.UpdateMonitorFunc != nil {
		return m.UpdateMonitorFunc(ctx, arg)
	}
	return dbgen.Monitor{}, nil
}

func (m *MockQuerier) DeleteMonitor(ctx context.Context, id uuid.UUID) error {
	if m.DeleteMonitorFunc != nil {
		return m.DeleteMonitorFunc(ctx, id)
	}
	return nil
}

func (m *MockQuerier) GetExistingMonitorIDs(ctx context.Context, ids []uuid.UUID) ([]uuid.UUID, error) {
	if m.GetExistingMonitorIDsFunc != nil {
		return m.GetExistingMonitorIDsFunc(ctx, ids)
	}
	return nil, nil
}

func (m *MockQuerier) GetLatestMetricsByDeviceAndPrefix(ctx context.Context, arg dbgen.GetLatestMetricsByDeviceAndPrefixParams) ([]dbgen.Metric, error) {
	if m.GetLatestMetricsByDeviceAndPrefixFunc != nil {
		return m.GetLatestMetricsByDeviceAndPrefixFunc(ctx, arg)
	}
	return nil, nil
}

func (m *MockQuerier) GetMetricsByDeviceAndPrefix(ctx context.Context, arg dbgen.GetMetricsByDeviceAndPrefixParams) ([]dbgen.Metric, error) {
	if m.GetMetricsByDeviceAndPrefixFunc != nil {
		return m.GetMetricsByDeviceAndPrefixFunc(ctx, arg)
	}
	return nil, nil
}

// Stub other methods to satisfy interface
func (m *MockQuerier) ClearDiscoveredDevices(ctx context.Context, discoveryProfileID uuid.NullUUID) error {
	return nil
}
func (m *MockQuerier) CreateDiscoveredDevice(ctx context.Context, arg dbgen.CreateDiscoveredDeviceParams) (dbgen.DiscoveredDevice, error) {
	return dbgen.DiscoveredDevice{}, nil
}
func (m *MockQuerier) CreateDiscoveryProfile(ctx context.Context, arg dbgen.CreateDiscoveryProfileParams) (dbgen.DiscoveryProfile, error) {
	return dbgen.DiscoveryProfile{}, nil
}
func (m *MockQuerier) DeleteDiscoveryProfile(ctx context.Context, id uuid.UUID) error { return nil }
func (m *MockQuerier) GetAllMetricNames(ctx context.Context, dollar_1 []uuid.UUID) ([]string, error) {
	return nil, nil
}
func (m *MockQuerier) GetDiscoveredDevice(ctx context.Context, id uuid.UUID) (dbgen.DiscoveredDevice, error) {
	return dbgen.DiscoveredDevice{}, nil
}
func (m *MockQuerier) GetDiscoveryProfile(ctx context.Context, id uuid.UUID) (dbgen.DiscoveryProfile, error) {
	return dbgen.DiscoveryProfile{}, nil
}
func (m *MockQuerier) ListActiveMonitorsWithCredentials(ctx context.Context) ([]dbgen.ListActiveMonitorsWithCredentialsRow, error) {
	return nil, nil
}
func (m *MockQuerier) ListDiscoveredDevices(ctx context.Context, discoveryProfileID uuid.NullUUID) ([]dbgen.DiscoveredDevice, error) {
	return nil, nil
}
func (m *MockQuerier) ListDiscoveryProfiles(ctx context.Context) ([]dbgen.DiscoveryProfile, error) {
	return nil, nil
}
func (m *MockQuerier) UpdateDiscoveredDeviceStatus(ctx context.Context, arg dbgen.UpdateDiscoveredDeviceStatusParams) error {
	return nil
}
func (m *MockQuerier) UpdateDiscoveryProfile(ctx context.Context, arg dbgen.UpdateDiscoveryProfileParams) (dbgen.DiscoveryProfile, error) {
	return dbgen.DiscoveryProfile{}, nil
}
func (m *MockQuerier) UpdateDiscoveryProfileStatus(ctx context.Context, arg dbgen.UpdateDiscoveryProfileStatusParams) error {
	return nil
}
func (m *MockQuerier) UpdateMonitorStatus(ctx context.Context, arg dbgen.UpdateMonitorStatusParams) error {
	return nil
}
