package poller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nmslite/nmslite/internal/config"
)

// MetricRecord represents a metric ready for database insertion
type MetricRecord struct {
	MonitorID   uuid.UUID
	Timestamp   time.Time
	MetricGroup string
	Tags        map[string]interface{}
	ValUsed     *float64
	ValTotal    *float64
}

// BatchWriter handles bulk metric writes using pgx COPY protocol
type BatchWriter struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
	cfg    *config.MetricsConfig

	// Buffering and flow control
	submitCh      chan MetricRecord
	requeueBuffer []MetricRecord
	bufferMu      sync.Mutex

	// Batch management
	currentBatch []MetricRecord
	batchMu      sync.Mutex
	lastFlush    time.Time

	// Failure tracking
	consecutiveFailures int
	maxConsecutiveFails int

	// Lifecycle management
	wg sync.WaitGroup
}

// NewBatchWriter creates a new BatchWriter instance
func NewBatchWriter(pool *pgxpool.Pool, cfg *config.MetricsConfig, logger *slog.Logger) *BatchWriter {
	// Set defaults if not configured
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 1000
	}

	flushIntervalMS := cfg.FlushIntervalMS
	if flushIntervalMS <= 0 {
		flushIntervalMS = 5000
	}

	maxBufferSize := batchSize * 10
	maxConsecutiveFails := 5
	submitChannelSize := batchSize * 2

	return &BatchWriter{
		pool:                pool,
		logger:              logger,
		cfg:                 cfg,
		submitCh:            make(chan MetricRecord, submitChannelSize),
		requeueBuffer:       make([]MetricRecord, 0, maxBufferSize),
		currentBatch:        make([]MetricRecord, 0, batchSize),
		lastFlush:           time.Now(),
		maxConsecutiveFails: maxConsecutiveFails,
	}
}

// Submit adds a metric record to the batch queue with backpressure
// This method blocks if the submit channel is full, providing natural backpressure
func (bw *BatchWriter) Submit(ctx context.Context, record MetricRecord) error {
	select {
	case bw.submitCh <- record:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("submit cancelled: %w", ctx.Err())
	}
}

// Run starts the batch writer's main processing loop
func (bw *BatchWriter) Run(ctx context.Context) error {
	bw.logger.Info("batch writer starting",
		"batch_size", bw.cfg.BatchSize,
		"flush_interval_ms", bw.cfg.FlushIntervalMS,
	)

	bw.wg.Add(1)
	defer bw.wg.Done()

	flushInterval := time.Duration(bw.cfg.FlushIntervalMS) * time.Millisecond
	flushTicker := time.NewTicker(flushInterval)
	defer flushTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			bw.logger.Info("batch writer shutting down, flushing remaining data")
			// Flush any remaining data before shutdown
			if err := bw.flush(context.Background()); err != nil {
				bw.logger.Error("final flush failed", "error", err)
			}
			return ctx.Err()

		case record := <-bw.submitCh:
			// Add record to current batch
			bw.batchMu.Lock()
			bw.currentBatch = append(bw.currentBatch, record)
			currentSize := len(bw.currentBatch)
			bw.batchMu.Unlock()

			// Flush if batch is full
			if currentSize >= bw.cfg.BatchSize {
				if err := bw.flush(ctx); err != nil {
					bw.logger.Error("flush on batch size failed", "error", err)
				}
			}

		case <-flushTicker.C:
			// Periodic flush based on time interval
			bw.batchMu.Lock()
			hasData := len(bw.currentBatch) > 0
			bw.batchMu.Unlock()

			if hasData {
				if err := bw.flush(ctx); err != nil {
					bw.logger.Error("periodic flush failed", "error", err)
				}
			}
		}
	}
}

// flush writes the current batch to the database
func (bw *BatchWriter) flush(ctx context.Context) error {
	bw.batchMu.Lock()
	if len(bw.currentBatch) == 0 {
		bw.batchMu.Unlock()
		return nil
	}

	// Swap current batch with a new one
	batch := bw.currentBatch
	bw.currentBatch = make([]MetricRecord, 0, bw.cfg.BatchSize)
	bw.batchMu.Unlock()

	// Include requeued items in this flush
	bw.bufferMu.Lock()
	if len(bw.requeueBuffer) > 0 {
		requeuedCount := len(bw.requeueBuffer) // Capture count before reset
		batch = append(bw.requeueBuffer, batch...)
		bw.requeueBuffer = make([]MetricRecord, 0, bw.cfg.BatchSize*10)
		bw.logger.Info("including requeued items in flush", "requeued_count", requeuedCount)
	}
	bw.bufferMu.Unlock()

	startTime := time.Now()
	err := bw.writeBatch(ctx, batch)
	duration := time.Since(startTime)

	if err != nil {
		bw.logger.Error("batch write failed",
			"error", err,
			"batch_size", len(batch),
			"duration_ms", duration.Milliseconds(),
		)

		// Track failure and requeue
		bw.consecutiveFailures++

		// Requeue if we haven't exceeded max failures
		if bw.consecutiveFailures < bw.maxConsecutiveFails {
			bw.requeue(batch)
		} else {
			bw.logger.Error("max consecutive failures reached, dropping batch",
				"consecutive_failures", bw.consecutiveFailures,
				"dropped_count", len(batch),
			)
		}

		return err
	}

	// Success - reset failure counter
	bw.consecutiveFailures = 0

	bw.logger.Debug("batch written successfully",
		"batch_size", len(batch),
		"duration_ms", duration.Milliseconds(),
	)

	bw.lastFlush = time.Now()
	return nil
}

// writeBatch performs the actual database write using COPY protocol
func (bw *BatchWriter) writeBatch(ctx context.Context, batch []MetricRecord) error {
	if len(batch) == 0 {
		return nil
	}

	// Use a transaction for COPY
	tx, err := bw.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			bw.logger.Warn("failed to rollback transaction", "error", err)
		}
	}()

	// Use COPY protocol for bulk insert
	copyCount, err := tx.Conn().CopyFrom(
		ctx,
		pgx.Identifier{"metrics"},
		[]string{"timestamp", "metric_group", "device_id", "tags", "val_used", "val_total"},
		pgx.CopyFromSlice(len(batch), func(i int) ([]interface{}, error) {
			record := batch[i]

			// Marshal tags to JSON
			var tagsJSON []byte
			if record.Tags != nil {
				tagsJSON, err = json.Marshal(record.Tags)
				if err != nil {
					return nil, fmt.Errorf("failed to marshal tags: %w", err)
				}
			}

			return []interface{}{
				record.Timestamp,
				record.MetricGroup,
				record.MonitorID,
				tagsJSON,
				record.ValUsed,
				record.ValTotal,
			}, nil
		}),
	)

	if err != nil {
		return fmt.Errorf("COPY operation failed: %w", err)
	}

	if copyCount != int64(len(batch)) {
		return fmt.Errorf("COPY count mismatch: expected %d, got %d", len(batch), copyCount)
	}

	// Commit the transaction
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// requeue adds failed batch back to the buffer for retry
func (bw *BatchWriter) requeue(batch []MetricRecord) {
	bw.bufferMu.Lock()
	defer bw.bufferMu.Unlock()

	maxBufferSize := bw.cfg.BatchSize * 10
	availableSpace := maxBufferSize - len(bw.requeueBuffer)

	if availableSpace <= 0 {
		bw.logger.Warn("requeue buffer full, dropping oldest items",
			"buffer_size", len(bw.requeueBuffer),
			"max_buffer_size", maxBufferSize,
			"dropping_count", len(batch),
		)
		return
	}

	// If batch is larger than available space, only requeue what fits
	toRequeue := batch
	if len(batch) > availableSpace {
		toRequeue = batch[:availableSpace]
		bw.logger.Warn("partial requeue due to buffer limit",
			"requested", len(batch),
			"requeued", len(toRequeue),
			"dropped", len(batch)-len(toRequeue),
		)
	}

	bw.requeueBuffer = append(bw.requeueBuffer, toRequeue...)

	bw.logger.Info("batch requeued for retry",
		"requeued_count", len(toRequeue),
		"buffer_size", len(bw.requeueBuffer),
	)
}
