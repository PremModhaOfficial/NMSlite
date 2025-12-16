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
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/nmslite/nmslite/internal/auth"
	"github.com/nmslite/nmslite/internal/channels"
	"github.com/nmslite/nmslite/internal/database/dbgen"
	"github.com/nmslite/nmslite/internal/globals"
	"github.com/nmslite/nmslite/internal/plugins"
)

// ScheduledMonitor wraps a dbgen.Monitor pointer with runtime scheduling state.
type ScheduledMonitor struct {
	Monitor             *dbgen.Monitor
	ConsecutiveFailures int
	NextPollDeadline    time.Time
	IsPolling           bool // True if a poll is currently in progress

	// Crypto/Cache (protected by SchedulerImpl.heapMu)
	EncryptedCredentials []byte               // Raw JSON from DB (eager loaded)
	Credentials          *plugins.Credentials // Decrypted on demand
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
	querier        dbgen.Querier
	pluginExecutor *plugins.Executor
	pluginRegistry *plugins.Registry
	credService    *auth.CredentialService
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
	querier dbgen.Querier,
	events *channels.EventChannels,
	pluginExecutor *plugins.Executor,
	pluginRegistry *plugins.Registry,
	credService *auth.CredentialService,
	resultWriter *ResultWriter,
) *SchedulerImpl {
	cfg := globals.GetConfig().Scheduler
	return &SchedulerImpl{
		querier:         querier,
		events:          events,
		pluginExecutor:  pluginExecutor,
		pluginRegistry:  pluginRegistry,
		credService:     credService,
		resultWriter:    resultWriter,
		logger:          slog.Default().With("component", "scheduler"),
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
		case event := <-s.events.CacheInvalidate:
			s.logger.Info("received cache invalidation event",
				"type", event.UpdateType,
				"update_count", len(event.Monitors),
				"delete_count", len(event.MonitorIDs),
			)
			if event.UpdateType == "update" {
				for _, row := range event.Monitors {
					s.updateMonitorCacheFromRow(row)
				}
			} else if event.UpdateType == "delete" {
				for _, id := range event.MonitorIDs {
					s.removeMonitorFromCache(id)
				}
			}
		}
	}
}

// LoadActiveMonitors loads all active monitors from the database at startup
func (s *SchedulerImpl) LoadActiveMonitors(ctx context.Context) error {
	s.logger.Info("Loading active monitors from database")

	// Use sqlc-generated query that joins monitors with credential_profiles
	rows, err := s.querier.ListActiveMonitorsWithCredentials(ctx)
	if err != nil {
		return fmt.Errorf("failed to query monitors: %w", err)
	}

	activeCount := 0
	for _, row := range rows {
		// Convert sqlc row to dbgen.Monitor
		m := &dbgen.Monitor{
			ID:                     row.ID,
			DisplayName:            row.DisplayName,
			Hostname:               row.Hostname,
			IpAddress:              row.IpAddress,
			PluginID:               row.PluginID,
			CredentialProfileID:    row.CredentialProfileID,
			DiscoveryProfileID:     row.DiscoveryProfileID,
			Port:                   row.Port,
			PollingIntervalSeconds: row.PollingIntervalSeconds,
			Status:                 row.Status,
			CreatedAt:              row.CreatedAt,
			UpdatedAt:              row.UpdatedAt,
		}

		sm := &ScheduledMonitor{
			Monitor:              m,
			EncryptedCredentials: row.Payload, // Already joined from credential_profiles
			NextPollDeadline:     time.Now(),
		}
		s.monitors[m.ID] = sm
		heap.Push(&s.heap, sm)
		activeCount++
	}

	s.logger.Info("Active monitors loaded",
		"active_monitors", activeCount,
	)

	return nil
}

// tick processes all monitors that are due for polling
func (s *SchedulerImpl) tick(ctx context.Context) {
	now := time.Now()
	nextTick := now.Add(s.tickInterval)

	// Step 1: Dequeue all due monitors
	dueMonitors := s.dequeueDueMonitors(nextTick)

	if len(dueMonitors) == 0 {
		return
	}

	// Step 2: Process monitors immediately
	// We no longer batch by second or use timers. We rely on the semaphores in processPluginBatch
	// to throttle execution.
	s.processMonitors(ctx, dueMonitors)
}

// dequeueDueMonitors removes all monitors due before nextTick from the heap,
// reschedules them for the future, and returns the list of monitors to process.
func (s *SchedulerImpl) dequeueDueMonitors(nextTick time.Time) []*ScheduledMonitor {
	s.heapMu.Lock()
	defer s.heapMu.Unlock()

	var dueItems []*ScheduledMonitor

	for len(s.heap) > 0 {
		sm := s.heap[0]
		if sm.NextPollDeadline.After(nextTick) {
			break
		}
		item := heap.Pop(&s.heap).(*ScheduledMonitor)
		dueItems = append(dueItems, item)

		// Reschedule immediately so they are ready for next cycle
		s.rescheduleUnlocked(item)
	}

	return dueItems
}

// processMonitors groups monitors by plugin and dispatches them to worker routines
func (s *SchedulerImpl) processMonitors(ctx context.Context, monitors []*ScheduledMonitor) {
	// Group by PluginID
	pluginBatches := make(map[string][]*ScheduledMonitor)

	for _, sm := range monitors {
		// Quick check if monitor is still valid and not polling
		s.heapMu.Lock()
		current, exists := s.monitors[sm.Monitor.ID]
		isValid := exists && current == sm
		isPolling := sm.IsPolling

		if isValid && !isPolling {
			sm.IsPolling = true
		}
		s.heapMu.Unlock()

		if !isValid {
			continue
		}
		if isPolling {
			s.logger.Debug("skipping poll, already in progress", "monitor_id", sm.Monitor.ID)
			continue
		}

		pluginBatches[sm.Monitor.PluginID] = append(pluginBatches[sm.Monitor.PluginID], sm)
	}

	// Dispatch batches
	for pluginID, batch := range pluginBatches {
		pluginID := pluginID
		batch := batch
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.processPluginBatch(ctx, pluginID, batch)
		}()
	}
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

// processPluginBatch processes a batch of monitors for the same plugin.
// It performs liveness checks in parallel, then calls the plugin once with all tasks.
// This is the only path for monitor polling - single monitors are just batches of 1.
func (s *SchedulerImpl) processPluginBatch(ctx context.Context, pluginID string, monitors []*ScheduledMonitor) {
	logger := s.logger.With("plugin_id", pluginID, "batch_size", len(monitors))
	logger.Debug("processing plugin batch")

	// Verify plugin exists
	_, ok := s.pluginRegistry.GetByID(pluginID)
	if !ok {
		logger.Error("plugin not found")
		for _, sm := range monitors {
			s.handleFailure(sm, fmt.Sprintf("plugin not found: %s", pluginID))
		}
		return
	}

	// Phase 1: Parallel liveness checks
	type livenessResult struct {
		sm    *ScheduledMonitor
		alive bool
	}
	resultsChan := make(chan livenessResult, len(monitors))

	var livenessWg sync.WaitGroup
	for _, sm := range monitors {
		sm := sm
		livenessWg.Add(1)
		go func() {
			defer livenessWg.Done()

			// Acquire liveness semaphore
			select {
			case s.livenessSem <- struct{}{}:
				defer func() { <-s.livenessSem }()
			case <-ctx.Done():
				resultsChan <- livenessResult{sm: sm, alive: false}
				// We rely on results loop to handle failure, but resultsChan read might be interrupted if we return early?
				// Wait, if ctx is done, resultChan send might block if buffer full?
				// Buffer size is len(monitors). Safe.
				return
			}

			alive := s.checkLiveness(ctx, sm)
			resultsChan <- livenessResult{sm: sm, alive: alive}
		}()
	}
	livenessWg.Wait()
	close(resultsChan)

	// Collect live monitors
	var liveMonitors []*ScheduledMonitor
	for result := range resultsChan {
		if result.alive {
			liveMonitors = append(liveMonitors, result.sm)
		} else {
			s.handleFailure(result.sm, "liveness check failed")
		}
	}

	if len(liveMonitors) == 0 {
		logger.Debug("no monitors passed liveness check")
		return
	}

	logger.Debug("liveness checks complete", "live_count", len(liveMonitors))

	// Acquire plugin semaphore (one slot for the batch)
	select {
	case s.pluginSem <- struct{}{}:
		defer func() { <-s.pluginSem }()
	case <-ctx.Done():
		logger.Warn("context cancelled while waiting for plugin semaphore")
		// Must fail all live monitors to reset IsPolling
		for _, sm := range liveMonitors {
			s.handleFailure(sm, "context cancelled")
		}
		return
	}

	// Phase 2: Build batch of poll tasks
	tasks := make([]plugins.PollTask, 0, len(liveMonitors))
	monitorByRequestID := make(map[string]*ScheduledMonitor, len(liveMonitors))

	for _, sm := range liveMonitors {
		// Lazy load credentials
		cred, err := s.ensureCredentials(sm)
		if err != nil {
			s.handleFailure(sm, fmt.Sprintf("credential error: %v", err))
			continue
		}

		// Get port value
		port := 0
		if sm.Monitor.Port.Valid {
			port = int(sm.Monitor.Port.Int32)
		}

		requestID := uuid.New().String()
		tasks = append(tasks, plugins.PollTask{
			RequestID:   requestID,
			Target:      sm.Monitor.IpAddress.String(),
			Port:        port,
			Credentials: *cred,
		})
		monitorByRequestID[requestID] = sm
	}

	if len(tasks) == 0 {
		logger.Debug("no valid tasks to execute")
		return
	}

	// Phase 3: Execute plugin batch
	pluginCtx, cancel := context.WithTimeout(ctx, s.pluginTimeout)
	defer cancel()

	logger.Debug("executing plugin batch", "task_count", len(tasks))

	results, err := s.pluginExecutor.Poll(pluginCtx, pluginID, tasks)
	if err != nil {
		logger.Error("plugin batch execution failed", "error", err)
		// Mark all as failed
		for _, sm := range monitorByRequestID {
			s.handleFailure(sm, fmt.Sprintf("plugin execution error: %v", err))
		}
		return
	}

	// Phase 4: Handle individual results
	handledRequests := make(map[string]bool)
	for _, result := range results {
		sm, ok := monitorByRequestID[result.RequestID]
		if !ok {
			logger.Warn("received result for unknown request", "request_id", result.RequestID)
			continue
		}

		handledRequests[result.RequestID] = true

		if result.Status != "success" {
			s.handleFailure(sm, fmt.Sprintf("plugin error: %s", result.Error))
		} else {
			s.handleSuccess(ctx, sm, []plugins.PollResult{result})
		}
	}

	// Fail any tasks that got no result
	for reqID, sm := range monitorByRequestID {
		if !handledRequests[reqID] {
			s.handleFailure(sm, "plugin execution returned no result")
		}
	}

	logger.Debug("plugin batch complete", "result_count", len(results))
}

// ensureCredentials lazily loads and caches credentials for a monitor.
// Caller should NOT hold heapMu - this function manages its own locking.
func (s *SchedulerImpl) ensureCredentials(sm *ScheduledMonitor) (*plugins.Credentials, error) {
	s.heapMu.Lock()
	cred := sm.Credentials
	if cred != nil {
		s.heapMu.Unlock()
		return cred, nil
	}

	if len(sm.EncryptedCredentials) == 0 {
		s.heapMu.Unlock()
		return nil, fmt.Errorf("missing encrypted credentials")
	}

	// Decrypt locally without DB call
	decrypted, err := s.credService.DecryptContainer(sm.EncryptedCredentials)
	if err != nil {
		s.heapMu.Unlock()
		return nil, fmt.Errorf("decryption error: %w", err)
	}
	sm.Credentials = decrypted
	s.heapMu.Unlock()

	return decrypted, nil
}

// handleSuccess processes a successful poll result
func (s *SchedulerImpl) handleSuccess(ctx context.Context, sm *ScheduledMonitor, results []plugins.PollResult) {
	s.heapMu.Lock()
	wasDown := sm.ConsecutiveFailures >= s.downThreshold
	sm.ConsecutiveFailures = 0
	sm.IsPolling = false
	s.heapMu.Unlock()

	// Write results using result writer
	s.resultWriter.Write(ctx, sm.Monitor.ID, results)

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
		case s.events.MonitorState <- channels.MonitorStateEvent{
			MonitorID: sm.Monitor.ID,
			IP:        sm.Monitor.IpAddress.String(),
			EventType: "recovered",
			Failures:  0,
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

	// Check if monitor is still valid/tracked
	if current, ok := s.monitors[sm.Monitor.ID]; !ok || current != sm {
		s.heapMu.Unlock()
		return
	}

	wasUp := sm.ConsecutiveFailures < s.downThreshold
	sm.ConsecutiveFailures++
	sm.IsPolling = false

	s.logger.Warn("monitor poll failed",
		"monitor_id", sm.Monitor.ID,
		"consecutive_failures", sm.ConsecutiveFailures,
		"reason", reason,
	)

	// Check if threshold reached
	if wasUp && sm.ConsecutiveFailures >= s.downThreshold {
		// Stop tracking (stops future polling)
		delete(s.monitors, sm.Monitor.ID)

		s.heapMu.Unlock()

		// Update DB (outside lock)
		s.updateMonitorStatus(context.Background(), sm.Monitor.ID, "down")

		// Emit event for external consumers
		select {
		case s.events.MonitorState <- channels.MonitorStateEvent{
			MonitorID: sm.Monitor.ID,
			IP:        sm.Monitor.IpAddress.String(),
			EventType: "down",
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

// rescheduleUnlocked computes the next poll deadline and adds monitor back to heap.
// Caller must hold heapMu lock.
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

// updateMonitorCacheFromRow updates a monitor in the cache from a pushed DB row
func (s *SchedulerImpl) updateMonitorCacheFromRow(row dbgen.GetMonitorWithCredentialsRow) {
	s.heapMu.Lock()
	defer s.heapMu.Unlock()

	// Check status
	if row.Status.String != "active" {
		if _, exists := s.monitors[row.ID]; exists {
			delete(s.monitors, row.ID)
			s.logger.Info("removed inactive monitor from scheduler cache", "monitor_id", row.ID)
		}
		return
	}

	// Reconstruct object
	monitor := dbgen.Monitor{
		ID:                     row.ID,
		DisplayName:            row.DisplayName,
		Hostname:               row.Hostname,
		IpAddress:              row.IpAddress,
		PluginID:               row.PluginID,
		CredentialProfileID:    row.CredentialProfileID,
		DiscoveryProfileID:     row.DiscoveryProfileID,
		Port:                   row.Port,
		PollingIntervalSeconds: row.PollingIntervalSeconds,
		Status:                 row.Status,
		CreatedAt:              row.CreatedAt,
		UpdatedAt:              row.UpdatedAt,
	}

	// Update or Create
	sm, exists := s.monitors[row.ID]
	if !exists {
		sm = &ScheduledMonitor{
			NextPollDeadline: time.Now(), // Schedule immediately
		}
		s.monitors[row.ID] = sm
		heap.Push(&s.heap, sm)
	}

	sm.Monitor = &monitor
	sm.EncryptedCredentials = row.Payload
	sm.Credentials = nil // Force re-decryption

	s.logger.Info("updated monitor in scheduler cache", "monitor_id", row.ID)
}

// removeMonitorFromCache removes a monitor from the cache
func (s *SchedulerImpl) removeMonitorFromCache(id uuid.UUID) {
	s.heapMu.Lock()
	defer s.heapMu.Unlock()

	if _, exists := s.monitors[id]; exists {
		delete(s.monitors, id)
		s.logger.Info("removed monitor from scheduler cache", "monitor_id", id)
	}
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
	err := s.querier.UpdateMonitorStatus(ctx, dbgen.UpdateMonitorStatusParams{
		ID:     monitorID,
		Status: pgtype.Text{String: status, Valid: true},
	})
	if err != nil {
		s.logger.Error("Failed to update monitor status",
			"monitor_id", monitorID,
			"status", status,
			"error", err,
		)
	}
}
