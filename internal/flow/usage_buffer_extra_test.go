package flow

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// alwaysFailInserter fails every BatchInsert call.
type alwaysFailInserter struct {
	calls int
}

func (f *alwaysFailInserter) BatchInsert(_ context.Context, logs []*store.UsageLog) error {
	f.calls++
	return errors.New("db unavailable")
}

func TestUsageBuffer_RecordDropsOldestWhenFull(t *testing.T) {
	buf := NewUsageBuffer(&alwaysFailInserter{}, time.Hour)

	const total = maxUsageBufferSize + 1
	for i := 0; i < total; i++ {
		require.NoError(t, buf.Record(context.Background(), &store.UsageLog{TokenID: uint(i + 1)}))
	}

	buf.mu.Lock()
	defer buf.mu.Unlock()
	assert.Len(t, buf.buf, maxUsageBufferSize)
	assert.Equal(t, uint(2), buf.buf[0].TokenID, "oldest record should be dropped")
	assert.Equal(t, uint(total), buf.buf[len(buf.buf)-1].TokenID, "newest record should be kept")
}

func TestUsageBuffer_FlushEmptyIsNoop(t *testing.T) {
	inserter := &alwaysFailInserter{}
	buf := NewUsageBuffer(inserter, time.Hour)

	buf.flush() // empty buffer must not touch the store

	assert.Equal(t, 0, inserter.calls)
}

// requeueRacingInserter fails its call after sneaking one extra record into
// the (already drained) buffer, so the re-queue exceeds the cap and the
// overflow branch drops the oldest record.
type requeueRacingInserter struct {
	buf   *UsageBuffer
	calls int
}

func (f *requeueRacingInserter) BatchInsert(ctx context.Context, logs []*store.UsageLog) error {
	f.calls++
	_ = f.buf.Record(ctx, &store.UsageLog{TokenID: 99999})
	return errors.New("db unavailable")
}

func TestUsageBuffer_FlushFailureRequeuesAndDropsOverflow(t *testing.T) {
	inserter := &requeueRacingInserter{}
	buf := NewUsageBuffer(inserter, time.Hour)
	inserter.buf = buf

	// Fill the buffer completely; the first failed flush requeues everything.
	for i := 0; i < maxUsageBufferSize; i++ {
		require.NoError(t, buf.Record(context.Background(), &store.UsageLog{TokenID: uint(i + 1)}))
	}
	buf.flush()
	require.Equal(t, 1, inserter.calls)

	buf.mu.Lock()
	defer buf.mu.Unlock()
	assert.Len(t, buf.buf, maxUsageBufferSize, "overflow records dropped after requeue")
	assert.Equal(t, uint(2), buf.buf[0].TokenID, "oldest re-queued record dropped")
	assert.Equal(t, uint(99999), buf.buf[len(buf.buf)-1].TokenID, "record added during flush kept")
}
