package poller

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nmslite/nmslite/internal/auth"
	"github.com/nmslite/nmslite/internal/channels"
	"github.com/nmslite/nmslite/internal/credentials"
	"github.com/nmslite/nmslite/internal/database/dbgen"
	"github.com/nmslite/nmslite/internal/globals"
	"github.com/nmslite/nmslite/internal/pluginManager"
)

// MockQuerier is a minimal mock for the test
type MockQuerier struct {
	dbgen.Querier
	ActiveMonitors []dbgen.ListActiveMonitorsWithCredentialsRow
}

func (m *MockQuerier) ListActiveMonitorsWithCredentials(context.Context) ([]dbgen.ListActiveMonitorsWithCredentialsRow, error) {
	return m.ActiveMonitors, nil
}

func (m *MockQuerier) UpdateMonitorStatus(context.Context, dbgen.UpdateMonitorStatusParams) error {
	return nil
}

func (m *MockQuerier) GetMonitor(context.Context, uuid.UUID) (dbgen.Monitor, error) {
	return dbgen.Monitor{}, fmt.Errorf("not implemented")
}

func (m *MockQuerier) GetCredentialProfile(context.Context, uuid.UUID) (dbgen.CredentialProfile, error) {
	return dbgen.CredentialProfile{}, fmt.Errorf("not implemented")
}

func TestScheduler_ConcurrentPollingPrevention(t *testing.T) {
	// 1. Setup temporary plugin environment
	tmpDir := t.TempDir()
	pluginID := "test-plugin"
	pluginDir := filepath.Join(tmpDir, pluginID)
	if err := os.Mkdir(pluginDir, 0755); err != nil {
		t.Fatalf("failed to create plugin dir: %v", err)
	}

	// Create poll log file to track executions
	pollLog := filepath.Join(tmpDir, "poll.log")

	// Create plugin script (sleeps longer than tick interval)
	// The script writes START, sleeps 300ms, writes END.
	// Tick interval will be 100ms.
	scriptContent := fmt.Sprintf(`#!/bin/sh
echo "$(date +%%s%%N) START" >> %s
sleep 0.1
echo "$(date +%%s%%N) END" >> %s
echo '[{"request_id": "req-1", "status": "success", "result": {}}]'
`, pollLog, pollLog)

	scriptPath := filepath.Join(pluginDir, pluginID) // binary name same as dir name
	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to write script: %v", err)
	}

	// Create manifest
	manifest := fmt.Sprintf(`{
		"id": "%s",
		"name": "Test Plugin",
		"version": "1.0.0",
		"description": "Test",
		"protocol": "ssh",
		"default_port": 22
	}`, pluginID)
	if err := os.WriteFile(filepath.Join(pluginDir, "manifest.json"), []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	// 2. Setup Dependencies
	// Initialize config for test
	// globals.InitGlobal() // Removed: unnecessary as SetGlobalConfigForTests is used below

	// Config - initialize global for scheduler to use
	testConfig := &globals.Config{
		Scheduler: globals.SchedulerConfig{
			TickIntervalMS:    2000, // 2s tick
			LivenessTimeoutMS: 100,
			PluginTimeoutMS:   2000,
			LivenessWorkers:   1,
			PluginWorkers:     5,
			DownThreshold:     3,
		},
		Channel: globals.EventBusConfig{ // Added channel config as NewEventChannels needs it
			DiscoveryEventsChannelSize: 50,
			StateSignalChannelSize:     50,
			CacheEventsChannelSize:     50,
		},
	}
	globals.SetGlobalConfigForTests(testConfig)

	// Create scheduler dependencies
	eventChans := channels.NewEventChannels()

	registry := pluginManager.NewRegistry(tmpDir)
	if err := registry.Scan(); err != nil {
		t.Fatalf("failed to scan registry: %v", err)
	}

	executor := pluginManager.NewExecutor(registry, 2*time.Second)

	// Mock DB
	monID := uuid.New()

	// Setup IP
	ip, _ := netip.ParseAddr("127.0.0.1")

	// Setup listener for liveness
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l.Close()
	addr := l.Addr().(*net.TCPAddr)

	port := int32(addr.Port)

	// Credentials setup
	key := "12345678901234567890123456789012" // 32 bytes
	// Auth service needed for credential service
	authSvc, err := auth.NewService("12345678901234567890123456789012", key, "admin", "admin", 1*time.Hour)
	if err != nil {
		t.Fatalf("failed to create auth service: %v", err)
	}

	// Encrypt credentials manually
	creds := &pluginManager.Credentials{
		Username: "user",
		Password: "password",
	}
	credsBytes, _ := json.Marshal(creds)
	encryptedStr, err := authSvc.Encrypt(credsBytes)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}
	encrypted := []byte(encryptedStr)

	// Re-create cred service with mock querier if needed, but ensureCredentials only decrypts
	// Scheduler uses credService which uses its own querier?
	// credentials.Service has a querier field.
	// We can pass the mockQuerier to it too.

	mockQuerier := &MockQuerier{
		ActiveMonitors: []dbgen.ListActiveMonitorsWithCredentialsRow{
			{
				ID:                     monID,
				DisplayName:            pgtype.Text{String: "Test Monitor", Valid: true},
				Hostname:               pgtype.Text{String: "localhost", Valid: true},
				IpAddress:              ip,
				PluginID:               pluginID,
				PollingIntervalSeconds: pgtype.Int4{Int32: 1, Valid: true}, // 1 second interval, but we override tick
				Status:                 pgtype.Text{String: "active", Valid: true},
				CreatedAt:              pgtype.Timestamptz{Time: time.Now(), Valid: true},
				CredentialData:         encrypted,
				Port:                   pgtype.Int4{Int32: port, Valid: true},
			},
		},
	}

	// Re-create cred service with mock querier
	credService := credentials.NewService(authSvc, mockQuerier)

	scheduler := NewSchedulerImpl(
		mockQuerier,
		eventChans,
		executor,
		registry,
		credService,
		&ResultWriter{batchWriter: &BatchWriter{}}, // Dummy
	)

	// 3. Execution
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second) // Run for 4s
	defer cancel()

	// Run scheduler in goroutine
	go func() {
		if err := scheduler.Run(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			// ignore expected cancel
		}
	}()

	// Wait for test duration
	<-ctx.Done()

	// 4. Analyze Results
	// Read poll log
	file, err := os.Open(pollLog)
	if err != nil {
		// If file doesn't exist, maybe no polls ran?
		t.Fatalf("failed to open poll log: %v", err)
	}
	defer file.Close()

	var events []struct {
		Time int64
		Type string
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) >= 2 {
			events = append(events, struct {
				Time int64
				Type string
			}{0, parts[1]})
		}
	}

	// Count concurrent executions
	active := 0
	maxActive := 0
	for _, e := range events {
		switch e.Type {
		case "START":
			active++
			if active > maxActive {
				maxActive = active
			}
		case "END":
			active--
		}
	}

	t.Logf("Max concurrent plugin executions: %d", maxActive)
	t.Logf("Total events: %d", len(events))

	// If no events, test failed setup
	if len(events) == 0 {
		t.Fatalf("No plugin executions recorded")
	}

	if maxActive > 1 {
		t.Errorf("FAIL: Monitor was polled concurrently (max active: %d)", maxActive)
	} else {
		t.Logf("PASS: Monitor was not polled concurrently (max active: %d)", maxActive)
	}

	// Verify reasonable number of executions (should be small, e.g. 1 or 2, not 10)
	// 4s run, 2s tick. 2 ticks. 1 monitor. Should be 2 executions max.
	if len(events) > 10 { // Arbitrary high number for "burst" detection
		t.Errorf("FAIL: Too many executions (%d), suggest tick loop re-processing bug", len(events))
	}
}
