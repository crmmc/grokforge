package flow

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBatchInserter records BatchInsert calls for testing.
type mockBatchInserter struct {
	mu       sync.Mutex
	calls    [][]*store.UsageLog
	failNext int32 // atomic: number of calls to fail before succeeding
}

func (m *mockBatchInserter) BatchInsert(_ context.Context, logs []*store.UsageLog) error {
	if atomic.AddInt32(&m.failNext, -1) >= 0 {
		return errors.New("mock batch insert failure")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// Copy the slice to avoid data races on the backing array
	cp := make([]*store.UsageLog, len(logs))
	copy(cp, logs)
	m.calls = append(m.calls, cp)
	return nil
}

func (m *mockBatchInserter) totalRecords() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, c := range m.calls {
		n += len(c)
	}
	return n
}

func TestUsageBuffer_Record(t *testing.T) {
	mock := &mockBatchInserter{}
	buf := NewUsageBuffer(mock, time.Hour) // long interval so no auto-flush

	err := buf.Record(context.Background(), &store.UsageLog{TokenID: 1, Model: "grok-3"})
	require.NoError(t, err)

	err = buf.Record(context.Background(), &store.UsageLog{TokenID: 2, Model: "grok-3-mini"})
	require.NoError(t, err)

	// Records should be buffered, not flushed yet
	assert.Equal(t, 0, mock.totalRecords())

	// Verify buffer has 2 records
	buf.mu.Lock()
	assert.Len(t, buf.buf, 2)
	buf.mu.Unlock()
}

func TestUsageBuffer_PeriodicFlush(t *testing.T) {
	mock := &mockBatchInserter{}
	buf := NewUsageBuffer(mock, 10*time.Millisecond)
	buf.Start()
	defer buf.Stop()

	// Record some entries
	for i := 0; i < 5; i++ {
		require.NoError(t, buf.Record(context.Background(), &store.UsageLog{TokenID: uint(i + 1)}))
	}

	// Wait for at least one flush cycle
	time.Sleep(50 * time.Millisecond)

	assert.Equal(t, 5, mock.totalRecords())

	// Buffer should be empty after flush
	buf.mu.Lock()
	assert.Empty(t, buf.buf)
	buf.mu.Unlock()
}

func TestUsageBuffer_StopFlush(t *testing.T) {
	mock := &mockBatchInserter{}
	buf := NewUsageBuffer(mock, time.Hour) // long interval so no periodic flush
	buf.Start()

	// Record entries
	for i := 0; i < 3; i++ {
		require.NoError(t, buf.Record(context.Background(), &store.UsageLog{TokenID: uint(i + 1)}))
	}

	// Stop should flush remaining
	buf.Stop()

	assert.Equal(t, 3, mock.totalRecords())
}

func TestUsageBuffer_ConcurrentRecord(t *testing.T) {
	mock := &mockBatchInserter{}
	buf := NewUsageBuffer(mock, 10*time.Millisecond)
	buf.Start()

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			_ = buf.Record(context.Background(), &store.UsageLog{TokenID: uint(id + 1)})
		}(i)
	}
	wg.Wait()

	// Stop to flush all remaining
	buf.Stop()

	assert.Equal(t, goroutines, mock.totalRecords())
}

func TestUsageBuffer_FlushFailureRetry(t *testing.T) {
	mock := &mockBatchInserter{}
	atomic.StoreInt32(&mock.failNext, 1) // fail first call

	buf := NewUsageBuffer(mock, 10*time.Millisecond)
	buf.Start()

	require.NoError(t, buf.Record(context.Background(), &store.UsageLog{TokenID: 1}))

	// Wait for first (failing) flush + second (succeeding) flush
	time.Sleep(50 * time.Millisecond)

	// Records should eventually be flushed on retry
	buf.Stop()

	assert.Equal(t, 1, mock.totalRecords())
}
