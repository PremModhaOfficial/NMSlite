package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nmslite/nmslite/internal/auth"
	"github.com/nmslite/nmslite/internal/channels"
	"github.com/nmslite/nmslite/internal/database/dbgen"
	"github.com/nmslite/nmslite/internal/globals"
)

// setupTest creates common dependencies for testing
func setupTest() (*auth.Service, *channels.EventChannels) {
	// Initialize globals
	dummyConfig := &globals.Config{
		Channel: globals.EventBusConfig{
			PollJobsChannelSize:        10,
			MetricResultsChannelSize:   10,
			CacheEventsChannelSize:     10,
			StateSignalChannelSize:     10,
			DiscoveryEventsChannelSize: 10,
			DeviceValidatedChannelSize: 10,
		},
	}
	globals.SetGlobalConfigForTests(dummyConfig)

	// 32-byte keys for testing
	jwtSecret := "12345678901234567890123456789012"
	encKey := "12345678901234567890123456789012"
	authService, _ := auth.NewService(jwtSecret, encKey, "admin", "admin", time.Hour)
	events := channels.NewEventChannels(context.Background())
	return authService, events
}

func TestHealthHandler(t *testing.T) {
	handler := NewHealthHandler()

	t.Run("Health", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health", nil)
		w := httptest.NewRecorder()
		handler.Health(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
		var resp HealthResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatal(err)
		}
		if resp.Status != "ok" {
			t.Errorf("expected status ok, got %s", resp.Status)
		}
	})

	t.Run("Ready", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/ready", nil)
		w := httptest.NewRecorder()
		handler.Ready(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})
}

func TestMonitorHandler_List(t *testing.T) {
	_, events := setupTest()
	mockQ := &MockQuerier{}
	handler := NewMonitorHandler(mockQ, events)

	expectedMonitors := []dbgen.Monitor{
		{
			ID:          uuid.New(),
			DisplayName: pgtype.Text{String: "Monitor 1", Valid: true},
			IpAddress:   netip.Addr{}, // zero value
		},
		{
			ID:          uuid.New(),
			DisplayName: pgtype.Text{String: "Monitor 2", Valid: true},
		},
	}

	mockQ.ListMonitorsFunc = func(ctx context.Context) ([]dbgen.Monitor, error) {
		return expectedMonitors, nil
	}

	req := httptest.NewRequest("GET", "/api/v1/monitors", nil)
	w := httptest.NewRecorder()

	handler.List(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Data  []dbgen.Monitor `json:"data"`
		Total int             `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	if resp.Total != 2 {
		t.Errorf("expected total 2, got %d", resp.Total)
	}
	if len(resp.Data) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.Data))
	}
}

func TestMonitorHandler_Get(t *testing.T) {
	_, events := setupTest()
	mockQ := &MockQuerier{}
	handler := NewMonitorHandler(mockQ, events)

	targetID := uuid.New()
	mockQ.GetMonitorFunc = func(ctx context.Context, id uuid.UUID) (dbgen.Monitor, error) {
		if id == targetID {
			return dbgen.Monitor{
				ID:          targetID,
				DisplayName: pgtype.Text{String: "Target Monitor", Valid: true},
			}, nil
		}
		return dbgen.Monitor{}, nil // Should return error in real db
	}

	// Chi router allows extracting params, but unit testing handler directly doesn't fill Chi context.
	// We need to wrap it in chi router or mock the param extraction.
	// The helpers.go `parseUUIDParam` uses `chi.URLParam`.
	// So we must route the request through chi or manually set context.

	r := chi.NewRouter()
	r.Get("/monitors/{id}", handler.Get)

	req := httptest.NewRequest("GET", "/monitors/"+targetID.String(), nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp dbgen.Monitor
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.ID != targetID {
		t.Errorf("expected ID %s, got %s", targetID, resp.ID)
	}
}

func TestCredentialHandler_Create(t *testing.T) {
	authService, events := setupTest()
	mockQ := &MockQuerier{}
	handler := NewCredentialHandler(mockQ, authService, events)

	input := map[string]interface{}{
		"name":            "Test Cred",
		"protocol":        "snmp-v2c",
		"description":     "Test Description",
		"credential_data": map[string]string{"community": "public", "version": "2c"},
	}
	body, _ := json.Marshal(input)

	mockQ.CreateCredentialProfileFunc = func(ctx context.Context, arg dbgen.CreateCredentialProfileParams) (dbgen.CredentialProfile, error) {
		return dbgen.CredentialProfile{
			ID:             uuid.New(),
			Name:           arg.Name,
			Protocol:       arg.Protocol,
			CredentialData: arg.CredentialData,
		}, nil
	}

	req := httptest.NewRequest("POST", "/api/v1/credentials", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.Create(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestDiscoveryHandler_Create_Validation(t *testing.T) {
	authService, events := setupTest()
	mockQ := &MockQuerier{}
	handler := NewDiscoveryHandler(mockQ, authService, events)

	// Missing Name
	input := map[string]interface{}{
		"target_value": "192.168.1.0/24",
	}
	body, _ := json.Marshal(input)

	req := httptest.NewRequest("POST", "/api/v1/discoveries", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.Create(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing name, got %d", w.Code)
	}
}

func TestMonitorHandler_Update(t *testing.T) {
	_, events := setupTest()
	mockQ := &MockQuerier{}
	handler := NewMonitorHandler(mockQ, events)

	targetID := uuid.New()
	existingMonitor := dbgen.Monitor{
		ID:       targetID,
		PluginID: "snmp",
		Status:   pgtype.Text{String: "active", Valid: true},
	}

	// Mock GetMonitor (called before update)
	mockQ.GetMonitorFunc = func(ctx context.Context, id uuid.UUID) (dbgen.Monitor, error) {
		if id == targetID {
			return existingMonitor, nil
		}
		return dbgen.Monitor{}, nil
	}

	// Mock UpdateMonitor
	mockQ.UpdateMonitorFunc = func(ctx context.Context, arg dbgen.UpdateMonitorParams) (dbgen.Monitor, error) {
		return dbgen.Monitor{
			ID:          arg.ID,
			DisplayName: arg.DisplayName,
			Status:      arg.Status,
		}, nil
	}

	input := map[string]interface{}{
		"display_name": "Updated Name",
		"status":       "maintenance",
	}
	body, _ := json.Marshal(input)

	// Use chi context hack or just context with URL param?
	// Using chi router is cleaner integration test style.
	r := chi.NewRouter()
	r.Patch("/monitors/{id}", handler.Update)

	req := httptest.NewRequest("PATCH", "/monitors/"+targetID.String(), bytes.NewReader(body))
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp dbgen.Monitor
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.DisplayName.String != "Updated Name" {
		t.Errorf("expected Updated Name, got %s", resp.DisplayName.String)
	}
	if resp.Status.String != "maintenance" {
		t.Errorf("expected maintenance, got %s", resp.Status.String)
	}
}

func TestMonitorHandler_QueryMetrics(t *testing.T) {
	_, events := setupTest()
	mockQ := &MockQuerier{}
	handler := NewMonitorHandler(mockQ, events)

	monitorID := uuid.New()

	// Mock validation
	mockQ.GetExistingMonitorIDsFunc = func(ctx context.Context, ids []uuid.UUID) ([]uuid.UUID, error) {
		return ids, nil
	}

	// Mock Metrics Query
	mockQ.GetMetricsByDeviceAndPrefixFunc = func(ctx context.Context, arg dbgen.GetMetricsByDeviceAndPrefixParams) ([]dbgen.Metric, error) {
		return []dbgen.Metric{
			{
				DeviceID:  monitorID,
				Name:      "cpu_usage",
				Value:     50.5,
				Timestamp: pgtype.Timestamptz{Time: time.Now(), Valid: true},
			},
		}, nil
	}

	input := MetricsQueryRequest{
		DeviceIDs: []uuid.UUID{monitorID},
		Start:     time.Now().Add(-1 * time.Hour),
		End:       time.Now(),
		Limit:     10,
	}
	body, _ := json.Marshal(input)

	req := httptest.NewRequest("POST", "/metrics/query", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.QueryMetrics(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp MetricsQueryResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}

	if resp.Count != 1 {
		t.Errorf("expected 1 metric, got %d", resp.Count)
	}
	if val, ok := resp.Data[monitorID.String()]["cpu_usage"]; !ok || val != 50.5 {
		t.Errorf("expected cpu_usage 50.5, got %v", val)
	}
}
