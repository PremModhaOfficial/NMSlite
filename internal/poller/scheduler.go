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
	"github.com/nmslite/nmslite/internal/channels"
	"github.com/nmslite/nmslite/internal/config"
	"github.com/nmslite/nmslite/internal/credentials"
	"github.com/nmslite/nmslite/internal/plugins"
)

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
	pq[i].heapIndex = i
	pq[j].heapIndex = j
}

func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*ScheduledMonitor)
	item.heapIndex = n
	*pq = append(*pq, item)
}

func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil      // avoid memory leak
	item.heapIndex = -1 // for safety
	*pq = old[0 : n-1]
	return item
}

// SchedulerImpl manages the scheduling and execution of monitor polling tasks
type SchedulerImpl struct {
	// Dependencies
	cache          *MonitorCache
	events         *channels.EventChannels
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
	heap   PriorityQueue
	heapMu sync.Mutex

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
	cache *MonitorCache,
	events *channels.EventChannels,
	pluginExecutor *plugins.Executor,
	pluginRegistry *plugins.Registry,
	credService *credentials.Service,
	resultWriter *ResultWriter,
	logger *slog.Logger,
	cfg config.SchedulerConfig,
) *SchedulerImpl {
	return &SchedulerImpl{
		cache:           cache,
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

	// Initialize heap from cache
	s.initHeap()

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

// AddMonitor adds or updates a monitor in the scheduler
func (s *SchedulerImpl) AddMonitor(sm *ScheduledMonitor) {
	s.heapMu.Lock()
	defer s.heapMu.Unlock()

	// If monitor already exists in heap, remove it first
	if sm.heapIndex >= 0 && sm.heapIndex < len(s.heap) {
		heap.Remove(&s.heap, sm.heapIndex)
	}

	// Add to heap
	heap.Push(&s.heap, sm)

	s.logger.Debug("monitor added to scheduler",
		"monitor_id", sm.Monitor.ID,
		"next_poll", sm.NextPollDeadline,
	)
}

// IsRunning returns whether the scheduler is currently running
func (s *SchedulerImpl) IsRunning() bool {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	return s.running
}

// initHeap rebuilds the priority queue from all monitors in cache
func (s *SchedulerImpl) initHeap() {
	s.heapMu.Lock()
	defer s.heapMu.Unlock()

	// Get all scheduled monitors from cache
	monitors := s.cache.GetAllScheduled()
	s.heap = make(PriorityQueue, 0, len(monitors))

	for _, sm := range monitors {
		// Initialize next poll deadline if not set
		if sm.NextPollDeadline.IsZero() {
			sm.NextPollDeadline = time.Now()
		}
		s.heap = append(s.heap, sm)
	}

	heap.Init(&s.heap)

	s.logger.Info("scheduler heap initialized", "monitor_count", len(s.heap))
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

	// Get credential ID, handle null case
	var credID uuid.UUID
	if sm.Monitor.CredentialProfileID.Valid {
		credID = sm.Monitor.CredentialProfileID.UUID
	} else {
		logger.Error("monitor has no credential profile")
		s.handleFailure(sm, "no credential profile configured")
		return
	}

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
	wasDown := sm.ConsecutiveFailures >= s.downThreshold

	// Reset failure count
	sm.ConsecutiveFailures = 0
	sm.LastPollAt = time.Now()

	// Write results using result writer
	s.resultWriter.Write(sm.Monitor.ID, results)

	s.logger.Info("monitor poll succeeded",
		"monitor_id", sm.Monitor.ID,
		"result_count", len(results),
	)

	// Emit recovery event if monitor was down
	if wasDown {
		// Send event to channel
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
	wasUp := sm.ConsecutiveFailures < s.downThreshold

	sm.ConsecutiveFailures++
	sm.LastPollAt = time.Now()

	s.logger.Warn("monitor poll failed",
		"monitor_id", sm.Monitor.ID,
		"consecutive_failures", sm.ConsecutiveFailures,
		"reason", reason,
	)

	// Emit down event if threshold reached
	if wasUp && sm.ConsecutiveFailures >= s.downThreshold {
		// Send event to channel
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
	sm.NextPollDeadline = time.Now().Add(interval)
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

// GetAllScheduled returns all scheduled monitors from cache
// This is a helper method that needs to be added to MonitorCache
func (mc *MonitorCache) GetAllScheduled() []*ScheduledMonitor {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	monitors := make([]*ScheduledMonitor, 0, len(mc.monitors))
	for _, sm := range mc.monitors {
		monitors = append(monitors, sm)
	}
	return monitors
}
