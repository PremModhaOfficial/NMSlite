// Package poller provides monitor polling and state management for the NMS Lite monitoring system.
package poller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nmslite/nmslite/internal/channels"
	"github.com/nmslite/nmslite/internal/database/db_gen"
)

// SchedulerInterface is the interface that the scheduler must implement
type SchedulerInterface interface {
	AddMonitor(sm *ScheduledMonitor)
}

// Scheduler is a placeholder type for the actual scheduler implementation
type Scheduler = SchedulerImpl

// MonitorCache represents an in-memory cache of active monitors being polled.
// It is protected by a mutex for concurrent access.
type MonitorCache struct {
	mu        sync.RWMutex
	monitors  map[uuid.UUID]*ScheduledMonitor
	scheduler SchedulerInterface
}

// ScheduledMonitor wraps a db_gen.Monitor pointer with runtime scheduling state.
// Runtime fields (ConsecutiveFailures, LastPollAt, NextPollDeadline) exist only
// in memory and are never persisted to the database.
type ScheduledMonitor struct {
	Monitor             *db_gen.Monitor // Pointer to actual monitor data
	ConsecutiveFailures int             // Runtime: failure count (reset on success)
	LastPollAt          time.Time       // Runtime: when last polled
	NextPollDeadline    time.Time       // Runtime: when next poll is due
	heapIndex           int             // Heap index for heap.Fix (-1 if not in heap)
}

// NewMonitorCache creates a new monitor cache.
func NewMonitorCache() *MonitorCache {
	return &MonitorCache{
		monitors: make(map[uuid.UUID]*ScheduledMonitor),
	}
}

// Add adds or updates a monitor in the cache.
func (mc *MonitorCache) Add(monitor *db_gen.Monitor) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	// Create ScheduledMonitor wrapper
	sm := &ScheduledMonitor{
		Monitor:          monitor,
		NextPollDeadline: time.Now(),
		heapIndex:        -1,
	}
	mc.monitors[monitor.ID] = sm

	// Add to scheduler if available
	if mc.scheduler != nil {
		mc.scheduler.AddMonitor(sm)
	}
}

// Remove removes a monitor from the cache.
func (mc *MonitorCache) Remove(monitorID uuid.UUID) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	delete(mc.monitors, monitorID)
}

// Get retrieves a monitor from the cache.
func (mc *MonitorCache) Get(monitorID uuid.UUID) (*db_gen.Monitor, bool) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	sm, exists := mc.monitors[monitorID]
	if !exists {
		return nil, false
	}
	return sm.Monitor, true
}

// GetAll returns a copy of all monitors currently in the cache.
func (mc *MonitorCache) GetAll() []*db_gen.Monitor {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	monitors := make([]*db_gen.Monitor, 0, len(mc.monitors))
	for _, sm := range mc.monitors {
		monitors = append(monitors, sm.Monitor)
	}
	return monitors
}

// Size returns the number of monitors in the cache.
func (mc *MonitorCache) Size() int {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return len(mc.monitors)
}

// SetScheduler sets the back-reference to scheduler (called once on startup).
// It also adds all cached monitors to the scheduler's heap.
func (mc *MonitorCache) SetScheduler(s SchedulerInterface) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.scheduler = s

	// Add all cached monitors to the scheduler's heap
	if mc.scheduler != nil {
		for _, sm := range mc.monitors {
			mc.scheduler.AddMonitor(sm)
		}
	}
}

// GetScheduled retrieves a scheduled monitor from the cache.
func (mc *MonitorCache) GetScheduled(monitorID uuid.UUID) (*ScheduledMonitor, bool) {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	sm, exists := mc.monitors[monitorID]
	return sm, exists
}

// IncrementFailures increments and returns new failure count.
func (mc *MonitorCache) IncrementFailures(monitorID uuid.UUID) int {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	if sm, exists := mc.monitors[monitorID]; exists {
		sm.ConsecutiveFailures++
		return sm.ConsecutiveFailures
	}
	return 0
}

// ResetFailures resets failure count to 0.
func (mc *MonitorCache) ResetFailures(monitorID uuid.UUID) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	if sm, exists := mc.monitors[monitorID]; exists {
		sm.ConsecutiveFailures = 0
	}
}

// Exists checks if monitor is in cache (for lazy deletion).
func (mc *MonitorCache) Exists(monitorID uuid.UUID) bool {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	_, exists := mc.monitors[monitorID]
	return exists
}

// StateHandler manages monitor state transitions and coordinates with the database
// and event channels. It handles state changes (monitor down/recovered) and updates the
// monitor cache accordingly.
type StateHandler struct {
	// events is the typed channel hub for monitor state events
	events *channels.EventChannels

	// db is the database connection for persisting state changes
	db *pgxpool.Pool

	// querier provides database operations
	querier db_gen.Querier

	// logger is used for structured logging
	logger *slog.Logger

	// cache maintains in-memory state of active monitors
	cache *MonitorCache

	// mu protects access to internal state
	mu sync.RWMutex

	// running tracks whether the handler is actively processing events
	running bool

	// done signals graceful shutdown
	done chan struct{}

	// wg tracks goroutines for graceful shutdown
	wg sync.WaitGroup
}

// NewStateHandler creates a new StateHandler with the specified dependencies.
//
// Parameters:
//   - events: the event channels for monitor state events
//   - db: the database connection for persisting state changes
//   - logger: the structured logger for event tracking
//
// Returns:
//   - *StateHandler: a new state handler instance ready to run
//
// Example:
//
//	handler := NewStateHandler(events, db, logger)
//	go handler.Run(ctx)
func NewStateHandler(events *channels.EventChannels, db *pgxpool.Pool, logger *slog.Logger) *StateHandler {
	return &StateHandler{
		events:  events,
		db:      db,
		querier: db_gen.New(db),
		logger:  logger,
		cache:   NewMonitorCache(),
		done:    make(chan struct{}),
	}
}

// Run starts the state handler event loop. It subscribes to monitor state change
// events (TopicMonitorState) and processes them concurrently.
//
// This method blocks until the context is canceled. It handles graceful shutdown
// by ensuring all pending events are processed before returning.
//
// Parameters:
//   - ctx: context for graceful shutdown (cancellation stops the event loop)
//
// Returns:
//   - error: returns nil on successful completion or context cancellation
//
// Example:
//
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//	if err := handler.Run(ctx); err != nil {
//		logger.Error("state handler failed", "error", err)
//	}
func (sh *StateHandler) Run(ctx context.Context) error {
	sh.mu.Lock()
	if sh.running {
		sh.mu.Unlock()
		return errors.New("state handler is already running")
	}
	sh.running = true
	sh.mu.Unlock()

	sh.logger.Info("State handler starting (channels-based)")

	// Start goroutines for each event type
	sh.wg.Add(2)
	go sh.processMonitorDownEvents(ctx)
	go sh.processMonitorRecoveredEvents(ctx)

	// Wait for context cancellation
	<-ctx.Done()

	// Signal shutdown
	close(sh.done)

	// Wait for event processing goroutines to complete
	sh.wg.Wait()

	sh.mu.Lock()
	sh.running = false
	sh.mu.Unlock()

	sh.logger.Info("State handler stopped")

	return nil
}

// processMonitorDownEvents continuously processes MonitorDownEvents from the typed channel.
func (sh *StateHandler) processMonitorDownEvents(ctx context.Context) {
	defer sh.wg.Done()

	for {
		select {
		case event, ok := <-sh.events.MonitorDown:
			if !ok {
				// Channel closed
				return
			}
			// Handle the down event - already typed!
			sh.handleMonitorDown(ctx, event)

		case <-sh.done:
			// Graceful shutdown signal
			return

		case <-ctx.Done():
			// Context cancelled
			return
		}
	}
}

// processMonitorRecoveredEvents continuously processes MonitorRecoveredEvents from the typed channel.
func (sh *StateHandler) processMonitorRecoveredEvents(ctx context.Context) {
	defer sh.wg.Done()

	for {
		select {
		case event, ok := <-sh.events.MonitorRecovered:
			if !ok {
				// Channel closed
				return
			}
			// Handle the recovered event - already typed!
			sh.handleMonitorRecovered(ctx, event)

		case <-sh.done:
			// Graceful shutdown signal
			return

		case <-ctx.Done():
			// Context cancelled
			return
		}
	}
}

// handleMonitorDown handles a monitor down event by updating the database status
// and removing the monitor from the polling cache.
//
// This method updates the monitor status to 'down' in the database, then removes
// it from the in-memory cache so it won't be polled until recovery.
//
// Parameters:
//   - ctx: context for database operations
//   - event: the monitor state event containing monitor details
func (sh *StateHandler) handleMonitorDown(ctx context.Context, event channels.MonitorDownEvent) {
	sh.logger.Info("Processing monitor down event",
		"monitor_id", event.MonitorID.String(),
		"ip_address", event.IP,
		"consecutive_failures", event.Failures,
		"timestamp", event.Timestamp.Format(time.RFC3339),
	)

	// Update monitor status in database
	if err := sh.updateMonitorStatus(ctx, event.MonitorID, "down"); err != nil {
		sh.logger.Error("Failed to update monitor status to down",
			"monitor_id", event.MonitorID.String(),
			"ip_address", event.IP,
			"error", err.Error(),
		)
		// Log but continue - state handler should not crash on DB errors
		return
	}

	// Remove from polling cache
	sh.cache.Remove(event.MonitorID)

	sh.logger.Info("Monitor transitioned to down state",
		"monitor_id", event.MonitorID.String(),
		"ip_address", event.IP,
		"consecutive_failures", event.Failures,
	)
}

// handleMonitorRecovered handles a monitor recovery event by updating the database
// status and re-adding the monitor to the polling cache.
//
// This method updates the monitor status to 'active' in the database and fetches
// the current monitor configuration to add it back to the cache for polling.
//
// Parameters:
//   - ctx: context for database operations
//   - event: the monitor state event containing monitor details
func (sh *StateHandler) handleMonitorRecovered(ctx context.Context, event channels.MonitorRecoveredEvent) {
	sh.logger.Info("Processing monitor recovered event",
		"monitor_id", event.MonitorID.String(),
		"ip_address", event.IP,
		"timestamp", event.Timestamp.Format(time.RFC3339),
	)

	// Update monitor status in database
	if err := sh.updateMonitorStatus(ctx, event.MonitorID, "active"); err != nil {
		sh.logger.Error("Failed to update monitor status to active",
			"monitor_id", event.MonitorID.String(),
			"ip_address", event.IP,
			"error", err.Error(),
		)
		// Log but continue - state handler should not crash on DB errors
		return
	}

	// Fetch the updated monitor configuration
	monitor, err := sh.querier.GetMonitor(ctx, event.MonitorID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			sh.logger.Warn("Monitor not found after recovery (may have been deleted)",
				"monitor_id", event.MonitorID.String(),
				"ip_address", event.IP,
			)
		} else {
			sh.logger.Error("Failed to fetch monitor after recovery",
				"monitor_id", event.MonitorID.String(),
				"ip_address", event.IP,
				"error", err.Error(),
			)
		}
		return
	}

	// Re-add to polling cache with updated configuration
	sh.cache.Add(&monitor)

	sh.logger.Info("Monitor transitioned to active state and re-added to cache",
		"monitor_id", event.MonitorID.String(),
		"ip_address", event.IP,
		"polling_interval_seconds", monitor.PollingIntervalSeconds,
	)
}

// updateMonitorStatus updates the monitor's status in the database.
// This is a database-only operation that doesn't use the standard UpdateMonitor
// query (which requires all fields). Instead, it uses a direct SQL query.
//
// Parameters:
//   - ctx: context for the database operation
//   - monitorID: the UUID of the monitor to update
//   - status: the new status ('active', 'down', 'maintenance')
//
// Returns:
//   - error: database operation error, or nil on success
func (sh *StateHandler) updateMonitorStatus(ctx context.Context, monitorID uuid.UUID, status string) error {
	query := `
		UPDATE monitors
		SET status = $1, updated_at = NOW()
		WHERE id = $2 AND deleted_at IS NULL
	`

	result, err := sh.db.Exec(ctx, query, status, monitorID)
	if err != nil {
		return fmt.Errorf("database exec failed: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("no rows updated: monitor may not exist or may be deleted")
	}

	return nil
}

// GetCache returns a reference to the monitor cache.
// This allows other components (like the poller) to access the cached monitors.
//
// Returns:
//   - *MonitorCache: the internal monitor cache
func (sh *StateHandler) GetCache() *MonitorCache {
	return sh.cache
}

// LoadActiveMonitors loads all active monitors from the database into the cache.
// This is typically called during startup to populate the cache with monitors
// that should be actively polled.
//
// Parameters:
//   - ctx: context for database operations
//
// Returns:
//   - error: database operation error, or nil on success
//
// Example:
//
//	if err := handler.LoadActiveMonitors(ctx); err != nil {
//		logger.Error("failed to load active monitors", "error", err)
//	}
func (sh *StateHandler) LoadActiveMonitors(ctx context.Context) error {
	sh.logger.Info("Loading active monitors from database")

	monitors, err := sh.querier.ListMonitors(ctx)
	if err != nil {
		return fmt.Errorf("failed to list monitors: %w", err)
	}

	// Filter for active monitors only
	// Down and maintenance monitors are excluded until explicitly re-enabled
	activeCount := 0
	for _, monitor := range monitors {
		// Only cache monitors with 'active' status
		if monitor.Status.Valid && monitor.Status.String == "active" {
			sh.cache.Add(&monitor)
			activeCount++
		}
	}

	sh.logger.Info("Active monitors loaded into cache",
		"total_monitors", len(monitors),
		"active_monitors", activeCount,
	)

	return nil
}

// IsRunning returns whether the state handler is currently running.
//
// Returns:
//   - bool: true if the handler is running, false otherwise
func (sh *StateHandler) IsRunning() bool {
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	return sh.running
}

// CacheSize returns the number of monitors currently in the cache.
//
// Returns:
//   - int: the count of cached monitors
func (sh *StateHandler) CacheSize() int {
	return sh.cache.Size()
}
