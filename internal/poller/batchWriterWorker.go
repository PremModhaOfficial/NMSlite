package poller

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nmslite/nmslite/internal/globals"
)

// MetricRecord represents a metric ready for database insertion (key-value format)
type MetricRecord struct {
	MonitorID int64
	Timestamp time.Time
	Name      string
	Value     float64
	Type      string // "gauge", "counter", "derive"
}

// BatchWriter handles bulk metric writes using pgx COPY protocol
type BatchWriter struct {
	pool   *pgxpool.Pool
	logger *slog.Logger
	cfg    *globals.MetricsConfig

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
func NewBatchWriter(pool *pgxpool.Pool) *BatchWriter {
	cfg := &globals.GetConfig().Metrics
	logger := slog.Default()

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
			if err := bw.flush(context.Background()); err != nil {
				bw.logger.Error("final flush failed", "error", err)
			}
			return ctx.Err()

		case record := <-bw.submitCh:
			bw.batchMu.Lock()
			bw.currentBatch = append(bw.currentBatch, record)
			currentSize := len(bw.currentBatch)
			bw.batchMu.Unlock()

			if currentSize >= bw.cfg.BatchSize {
				if err := bw.flush(ctx); err != nil {
					bw.logger.Error("flush on batch size failed", "error", err)
				}
			}

		case <-flushTicker.C:
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

	batch := bw.currentBatch
	bw.currentBatch = make([]MetricRecord, 0, bw.cfg.BatchSize)
	bw.batchMu.Unlock()

	bw.bufferMu.Lock()
	if len(bw.requeueBuffer) > 0 {
		requeuedCount := len(bw.requeueBuffer)
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

		bw.consecutiveFailures++

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

	tx, err := bw.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			bw.logger.Warn("failed to rollback transaction", "error", err)
		}
	}()

	// Use COPY protocol for bulk insert - key-value format with type
	copyCount, err := tx.Conn().CopyFrom(
		ctx,
		pgx.Identifier{"metrics"},
		[]string{"timestamp", "device_id", "name", "value", "type"},
		pgx.CopyFromSlice(len(batch), func(i int) ([]interface{}, error) {
			record := batch[i]
			return []interface{}{
				record.Timestamp,
				record.MonitorID,
				record.Name,
				record.Value,
				record.Type,
			}, nil
		}),
	)

	if err != nil {
		return fmt.Errorf("COPY operation failed: %w", err)
	}

	if copyCount != int64(len(batch)) {
		return fmt.Errorf("COPY count mismatch: expected %d, got %d", len(batch), copyCount)
	}

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
