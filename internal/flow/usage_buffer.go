package flow

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/crmmc/grokforge/internal/store"
)

// UsageLogBatchInserter defines the interface for batch-inserting usage logs.
type UsageLogBatchInserter interface {
	BatchInsert(ctx context.Context, logs []*store.UsageLog) error
}

// UsageBuffer implements UsageRecorder with an in-memory buffer and periodic DB flush.
type UsageBuffer struct {
	mu       sync.Mutex
	buf      []*store.UsageLog
	store    UsageLogBatchInserter
	interval time.Duration
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// NewUsageBuffer creates a new UsageBuffer.
func NewUsageBuffer(s UsageLogBatchInserter, interval time.Duration) *UsageBuffer {
	return &UsageBuffer{
		store:    s,
		interval: interval,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

// Record appends a usage log to the in-memory buffer (non-blocking).
func (b *UsageBuffer) Record(_ context.Context, log *store.UsageLog) error {
	b.mu.Lock()
	b.buf = append(b.buf, log)
	b.mu.Unlock()
	return nil
}

// Start launches the periodic flush loop in a background goroutine.
func (b *UsageBuffer) Start() {
	go b.flushLoop()
}

// Stop signals the flush loop to stop and waits for remaining records to be flushed.
func (b *UsageBuffer) Stop() {
	close(b.stopCh)
	<-b.doneCh
}

// flushLoop periodically flushes the buffer to the database.
func (b *UsageBuffer) flushLoop() {
	defer close(b.doneCh)
	ticker := time.NewTicker(b.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			b.flush()
		case <-b.stopCh:
			b.flush() // final flush on shutdown
			return
		}
	}
}

// flush drains the buffer and writes to the store. On failure, records are re-added.
func (b *UsageBuffer) flush() {
	b.mu.Lock()
	if len(b.buf) == 0 {
		b.mu.Unlock()
		return
	}
	records := b.buf
	b.buf = nil
	b.mu.Unlock()

	if err := b.store.BatchInsert(context.Background(), records); err != nil {
		slog.Error("usage buffer flush failed, re-queuing", "count", len(records), "error", err)
		// Re-add failed records to front of buffer for next attempt
		b.mu.Lock()
		b.buf = append(records, b.buf...)
		b.mu.Unlock()
		return
	}

	slog.Debug("usage buffer flushed", "count", len(records))
}
