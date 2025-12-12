package poller

import (
	"container/heap"
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nmslite/nmslite/internal/channels"
	"github.com/nmslite/nmslite/internal/config"
	"github.com/nmslite/nmslite/internal/credentials"
	"github.com/nmslite/nmslite/internal/database/db_gen"
	"github.com/nmslite/nmslite/internal/plugins"
)

// ScheduledMonitor wraps a db_gen.Monitor pointer with runtime scheduling state.
type ScheduledMonitor struct {
	Monitor             *db_gen.Monitor
	ConsecutiveFailures int
	NextPollDeadline    time.Time
	valid               bool // For lazy deletion
}

// PriorityQueue implements heap.Interface for *ScheduledMonitor
type PriorityQueue []*ScheduledMonitor

func (pq PriorityQueue) Len() int {
	return len(pq)
}

func (pq PriorityQueue) Less(i, j int) bool {
	// Earlier deadlines have higher priority
	return pq[i].NextPollDeadline.Before(pq[j].NextPollDeadline)
}

func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
}

func (pq *PriorityQueue) Push(x interface{}) {
	item := x.(*ScheduledMonitor)
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil // avoid memory leak
	*pq = old[0 : n-1]
	return item
}

// SchedulerImpl manages the scheduling and execution of monitor polling tasks
type SchedulerImpl struct {
	// Dependencies
	events         *channels.EventChannels
	db             *pgxpool.Pool
	querier        db_gen.Querier
	pluginExecutor *plugins.Executor
	pluginRegistry *plugins.Registry
	credService    *credentials.Service
	resultWriter   *ResultWriter
	logger         *slog.Logger

	// Configuration
	tickInterval    time.Duration
	livenessTimeout time.Duration
	pluginTimeout   time.Duration
	downThreshold   int

	// Priority queue
	heap     PriorityQueue
	heapMu   sync.Mutex
	monitors map[uuid.UUID]*ScheduledMonitor

	// Semaphores for concurrency control
	livenessSem chan struct{}
	pluginSem   chan struct{}

	// Lifecycle management
	running bool
	runMu   sync.Mutex
	done    chan struct{}
	wg      sync.WaitGroup
}

// NewSchedulerImpl creates a new SchedulerImpl instance
func NewSchedulerImpl(
	db *pgxpool.Pool,
	events *channels.EventChannels,
	pluginExecutor *plugins.Executor,
	pluginRegistry *plugins.Registry,
	credService *credentials.Service,
	resultWriter *ResultWriter,
	logger *slog.Logger,
	cfg config.SchedulerConfig,
) *SchedulerImpl {
	return &SchedulerImpl{
		db:              db,
		querier:         db_gen.New(db),
		events:          events,
		pluginExecutor:  pluginExecutor,
		pluginRegistry:  pluginRegistry,
		credService:     credService,
		resultWriter:    resultWriter,
		logger:          logger.With("component", "scheduler"),
		tickInterval:    cfg.GetTickInterval(),
		livenessTimeout: cfg.GetLivenessTimeout(),
		pluginTimeout:   cfg.GetPluginTimeout(),
		downThreshold:   cfg.DownThreshold,
		livenessSem:     make(chan struct{}, cfg.LivenessWorkers),
		pluginSem:       make(chan struct{}, cfg.PluginWorkers),
		heap:            make(PriorityQueue, 0),
		monitors:        make(map[uuid.UUID]*ScheduledMonitor),
		done:            make(chan struct{}),
	}
}

// Run starts the scheduler and blocks until context is cancelled
func (s *SchedulerImpl) Run(ctx context.Context) error {
	s.runMu.Lock()
	if s.running {
		s.runMu.Unlock()
		return fmt.Errorf("scheduler already running")
	}
	s.running = true
	s.runMu.Unlock()

	s.logger.Info("starting scheduler",
		"tick_interval", s.tickInterval,
		"liveness_timeout", s.livenessTimeout,
		"plugin_timeout", s.pluginTimeout,
		"down_threshold", s.downThreshold,
	)

	// Load active monitors from database
	if err := s.LoadActiveMonitors(ctx); err != nil {
		return fmt.Errorf("failed to load monitors: %w", err)
	}

	ticker := time.NewTicker(s.tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scheduler context cancelled, shutting down")
			s.shutdown()
			return ctx.Err()
		case <-s.done:
			s.logger.Info("scheduler done signal received")
			s.shutdown()
			return nil
		case <-ticker.C:
			s.tick(ctx)
		}
	}
}

// AddMonitor adds a new monitor to the scheduler (used by discovery/API)
// Sets NextPollDeadline to zero time so it's picked up on next tick
func (s *SchedulerImpl) AddMonitor(monitor *db_gen.Monitor) {
	s.heapMu.Lock()
	defer s.heapMu.Unlock()

	sm := &ScheduledMonitor{
		Monitor:          monitor,
		NextPollDeadline: time.Time{}, // Zero time = immediate pickup
		valid:            true,
	}

	s.monitors[monitor.ID] = sm
	heap.Push(&s.heap, sm)

	s.logger.Info("monitor added to scheduler",
		"monitor_id", monitor.ID,
		"next_poll", "immediate",
	)
}

// IsRunning returns whether the scheduler is currently running
func (s *SchedulerImpl) IsRunning() bool {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	return s.running
}

// LoadActiveMonitors loads all active monitors from the database at startup
func (s *SchedulerImpl) LoadActiveMonitors(ctx context.Context) error {
	s.logger.Info("Loading active monitors from database")

	monitors, err := s.querier.ListMonitors(ctx)
	if err != nil {
		return fmt.Errorf("failed to list monitors: %w", err)
	}

	s.heapMu.Lock()
	defer s.heapMu.Unlock()

	activeCount := 0
	for _, monitor := range monitors {
		if monitor.Status.Valid && monitor.Status.String == "active" {
			sm := &ScheduledMonitor{
				Monitor:          &monitor,
				NextPollDeadline: time.Now(),
				valid:            true,
			}
			s.monitors[monitor.ID] = sm
			heap.Push(&s.heap, sm)
			activeCount++
		}
	}

	s.logger.Info("Active monitors loaded",
		"total_monitors", len(monitors),
		"active_monitors", activeCount,
	)

	return nil
}

// tick processes all monitors that are due for polling
func (s *SchedulerImpl) tick(ctx context.Context) {
	now := time.Now()

	s.heapMu.Lock()

	// Collect all due monitors
	var dueMonitors []*ScheduledMonitor
	for len(s.heap) > 0 {
		sm := s.heap[0]
		if sm.NextPollDeadline.After(now) {
			break
		}

		// Pop from heap
		popped := heap.Pop(&s.heap).(*ScheduledMonitor)

		// Lazy deletion - skip invalid monitors
		if !popped.valid {
			s.logger.Debug("dropping invalid monitor from queue",
				"monitor_id", popped.Monitor.ID)
			continue
		}

		dueMonitors = append(dueMonitors, popped)

		// Reschedule immediately (will be pushed back to heap after processing)
		s.rescheduleUnlocked(popped)
	}

	s.heapMu.Unlock()

	// Process due monitors concurrently
	for _, sm := range dueMonitors {
		sm := sm // capture loop variable
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.processMonitor(ctx, sm)
		}()
	}

	if len(dueMonitors) > 0 {
		s.logger.Debug("tick processed due monitors", "count", len(dueMonitors))
	}
}

// processMonitor executes the full polling workflow for a monitor
func (s *SchedulerImpl) processMonitor(ctx context.Context, sm *ScheduledMonitor) {
	monitorID := sm.Monitor.ID
	logger := s.logger.With("monitor_id", monitorID)

	logger.Debug("processing monitor")

	// Acquire liveness semaphore
	select {
	case s.livenessSem <- struct{}{}:
		defer func() { <-s.livenessSem }()
	case <-ctx.Done():
		logger.Warn("context cancelled while waiting for liveness semaphore")
		return
	}

	// Check liveness
	alive := s.checkLiveness(ctx, sm)
	if !alive {
		logger.Info("monitor failed liveness check")
		s.handleFailure(sm, "liveness check failed")
		return
	}

	logger.Debug("monitor passed liveness check")

	// Acquire plugin semaphore
	select {
	case s.pluginSem <- struct{}{}:
		defer func() { <-s.pluginSem }()
	case <-ctx.Done():
		logger.Warn("context cancelled while waiting for plugin semaphore")
		return
	}

	// Execute plugin
	s.executePlugin(ctx, sm)
}

// checkLiveness performs a TCP SYN probe to verify the monitor is reachable
func (s *SchedulerImpl) checkLiveness(ctx context.Context, sm *ScheduledMonitor) bool {
	// Get port value, default to 0 if null
	port := int32(0)
	if sm.Monitor.Port.Valid {
		port = sm.Monitor.Port.Int32
	}

	target := fmt.Sprintf("%s:%d", sm.Monitor.IpAddress.String(), port)

	livenessCtx, cancel := context.WithTimeout(ctx, s.livenessTimeout)
	defer cancel()

	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(livenessCtx, "tcp", target)
	if err != nil {
		s.logger.Debug("liveness check failed",
			"monitor_id", sm.Monitor.ID,
			"target", target,
			"error", err,
		)
		return false
	}

	conn.Close()
	return true
}

// executePlugin executes the plugin collector for a monitor
func (s *SchedulerImpl) executePlugin(ctx context.Context, sm *ScheduledMonitor) {
	monitorID := sm.Monitor.ID
	logger := s.logger.With("monitor_id", monitorID)

	// Verify plugin exists
	_, ok := s.pluginRegistry.GetByID(sm.Monitor.PluginID)
	if !ok {
		logger.Error("plugin not found",
			"plugin", sm.Monitor.PluginID,
		)
		s.handleFailure(sm, fmt.Sprintf("plugin not found: %s", sm.Monitor.PluginID))
		return
	}

	// Get credential ID (now guaranteed to be non-null from DB constraint)
	credID := sm.Monitor.CredentialProfileID

	// Get credentials
	cred, err := s.credService.GetDecrypted(ctx, credID)
	if err != nil {
		logger.Error("failed to get credentials", "error", err)
		s.handleFailure(sm, fmt.Sprintf("credential error: %v", err))
		return
	}

	// Get port value
	port := int(0)
	if sm.Monitor.Port.Valid {
		port = int(sm.Monitor.Port.Int32)
	}

	// Build poll task
	task := plugins.PollTask{
		RequestID:   uuid.New().String(),
		Target:      sm.Monitor.IpAddress.String(),
		Port:        port,
		Credentials: *cred,
	}

	// Execute plugin with timeout
	pluginCtx, cancel := context.WithTimeout(ctx, s.pluginTimeout)
	defer cancel()

	logger.Debug("executing plugin", "plugin", sm.Monitor.PluginID)

	results, err := s.pluginExecutor.Poll(pluginCtx, sm.Monitor.PluginID, []plugins.PollTask{task})
	if err != nil {
		logger.Error("plugin execution failed",
			"plugin", sm.Monitor.PluginID,
			"error", err,
		)
		s.handleFailure(sm, fmt.Sprintf("plugin execution error: %v", err))
		return
	}

	// Check if we got results
	if len(results) == 0 {
		logger.Warn("plugin returned no results", "plugin", sm.Monitor.PluginID)
		s.handleFailure(sm, "plugin returned no results")
		return
	}

	result := results[0]

	if result.Status != "success" {
		logger.Warn("plugin returned failure",
			"plugin", sm.Monitor.PluginID,
			"status", result.Status,
			"error", result.Error,
		)
		s.handleFailure(sm, fmt.Sprintf("plugin error: %s", result.Error))
		return
	}

	// Success
	logger.Debug("plugin execution succeeded",
		"plugin", sm.Monitor.PluginID,
		"metric_count", len(result.Metrics),
	)
	s.handleSuccess(sm, results)
}

// handleSuccess processes a successful poll result
func (s *SchedulerImpl) handleSuccess(sm *ScheduledMonitor, results []plugins.PollResult) {
	s.heapMu.Lock()
	wasDown := sm.ConsecutiveFailures >= s.downThreshold
	sm.ConsecutiveFailures = 0
	s.heapMu.Unlock()

	// Write results using result writer
	s.resultWriter.Write(sm.Monitor.ID, results)

	s.logger.Info("monitor poll succeeded",
		"monitor_id", sm.Monitor.ID,
		"result_count", len(results),
	)

	// Handle recovery if monitor was down
	if wasDown {
		// Update DB status
		s.updateMonitorStatus(context.Background(), sm.Monitor.ID, "active")

		// Emit recovery event for external consumers
		select {
		case s.events.MonitorRecovered <- channels.MonitorRecoveredEvent{
			MonitorID: sm.Monitor.ID,
			IP:        sm.Monitor.IpAddress.String(),
			Timestamp: time.Now(),
		}:
			s.logger.Info("monitor recovered",
				"monitor_id", sm.Monitor.ID,
				"ip_address", sm.Monitor.IpAddress.String(),
			)
		default:
			s.logger.Warn("failed to emit monitor recovered event: channel full",
				"monitor_id", sm.Monitor.ID,
			)
		}
	}
}

// handleFailure processes a failed poll attempt
func (s *SchedulerImpl) handleFailure(sm *ScheduledMonitor, reason string) {
	s.heapMu.Lock()

	if !sm.valid {
		s.heapMu.Unlock()
		return
	}

	wasUp := sm.ConsecutiveFailures < s.downThreshold
	sm.ConsecutiveFailures++

	s.logger.Warn("monitor poll failed",
		"monitor_id", sm.Monitor.ID,
		"consecutive_failures", sm.ConsecutiveFailures,
		"reason", reason,
	)

	// Check if threshold reached
	if wasUp && sm.ConsecutiveFailures >= s.downThreshold {
		// Mark invalid for lazy deletion
		sm.valid = false
		delete(s.monitors, sm.Monitor.ID)

		s.heapMu.Unlock()

		// Update DB (outside lock)
		s.updateMonitorStatus(context.Background(), sm.Monitor.ID, "down")

		// Emit event for external consumers
		select {
		case s.events.MonitorDown <- channels.MonitorDownEvent{
			MonitorID: sm.Monitor.ID,
			IP:        sm.Monitor.IpAddress.String(),
			Failures:  sm.ConsecutiveFailures,
			Timestamp: time.Now(),
		}:
			s.logger.Warn("monitor is down",
				"monitor_id", sm.Monitor.ID,
				"ip_address", sm.Monitor.IpAddress.String(),
				"threshold", s.downThreshold,
			)
		default:
			s.logger.Warn("failed to emit monitor down event: channel full",
				"monitor_id", sm.Monitor.ID,
			)
		}
	} else {
		s.heapMu.Unlock()
	}
}

// reschedule computes the next poll deadline and adds monitor back to heap
func (s *SchedulerImpl) reschedule(sm *ScheduledMonitor) {
	s.heapMu.Lock()
	defer s.heapMu.Unlock()
	s.rescheduleUnlocked(sm)
}

// rescheduleUnlocked is the internal version that assumes heapMu is already locked
func (s *SchedulerImpl) rescheduleUnlocked(sm *ScheduledMonitor) {
	// Get polling interval, default to 60 seconds if null
	intervalSeconds := int32(60)
	if sm.Monitor.PollingIntervalSeconds.Valid {
		intervalSeconds = sm.Monitor.PollingIntervalSeconds.Int32
	}

	interval := time.Duration(intervalSeconds) * time.Second
	sm.NextPollDeadline = sm.NextPollDeadline.Add(interval)
	heap.Push(&s.heap, sm)

	s.logger.Debug("monitor rescheduled",
		"monitor_id", sm.Monitor.ID,
		"next_poll", sm.NextPollDeadline,
		"interval", interval,
	)
}

// shutdown performs graceful shutdown of the scheduler
func (s *SchedulerImpl) shutdown() {
	s.logger.Info("shutting down scheduler, waiting for workers to complete")

	// Wait for all workers to complete
	s.wg.Wait()

	s.runMu.Lock()
	s.running = false
	s.runMu.Unlock()

	s.logger.Info("scheduler shutdown complete")
}

// updateMonitorStatus updates the monitor's status in the database
func (s *SchedulerImpl) updateMonitorStatus(ctx context.Context, monitorID uuid.UUID, status string) {
	query := `
		UPDATE monitors
		SET status = $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`

	result, err := s.db.Exec(ctx, query, status, monitorID)
	if err != nil {
		s.logger.Error("Failed to update monitor status",
			"monitor_id", monitorID,
			"status", status,
			"error", err,
		)
		return
	}

	if result.RowsAffected() == 0 {
		s.logger.Warn("No rows updated when setting monitor status",
			"monitor_id", monitorID,
			"status", status,
		)
	}
}
