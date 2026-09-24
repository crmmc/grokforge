package xai

import (
	"context"
	"errors"
	"io"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// panickyBody panics on the first Read - used to exercise the recover path.
type panickyBody struct{}

func (b *panickyBody) Read([]byte) (int, error) { panic("read boom") }
func (b *panickyBody) Close() error             { return nil }

func TestStreamResponseRecoversReadPanic(t *testing.T) {
	ch := streamResponse(context.Background(), &panickyBody{})

	var events []StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}
	assert.Empty(t, events, "panic should be recovered and channel closed")
}

// gatedPanicBody hands out `bulk` on the first Read, then signals `entered`
// and blocks on the gate; when the gate opens it returns a read error.
// Every Close call (the ctx watcher's, then the final defer's) signals on
// `closes`, opens the gate once, and then panics - so the cancel watcher's
// recover and the outer recover are both exercised deterministically.
type gatedPanicBody struct {
	bulk    []byte
	entered chan struct{}
	gate    chan struct{}
	closes  chan struct{}
	gateOff sync.Once
}

func (b *gatedPanicBody) Read(p []byte) (int, error) {
	if len(b.bulk) > 0 {
		n := copy(p, b.bulk)
		b.bulk = nil
		return n, nil
	}
	select {
	case b.entered <- struct{}{}:
	default:
	}
	<-b.gate
	return 0, errors.New("boom after gate")
}

func (b *gatedPanicBody) Close() error {
	select {
	case b.closes <- struct{}{}:
	default:
	}
	b.gateOff.Do(func() { close(b.gate) })
	panic("close boom")
}

func TestStreamResponseErrorSendSkippedWhenCancelled(t *testing.T) {
	// 33 lines: after the consumer takes 17, the goroutine has sent all 33
	// (16 buffered = full channel) and is blocked reading. Cancelling makes
	// the watcher Close the body (panic recovered there) which unblocks the
	// reader with an error; the scanner-error send-select then sees ctx.Done
	// with a full channel, so the error event is dropped deterministically.
	// The drain starts only after the stream goroutine has exited (two Close
	// calls observed), so no receiver can make the error send ready.
	ctx, cancel := context.WithCancel(context.Background())
	body := &gatedPanicBody{
		bulk:    []byte(strings.Repeat("{\"n\":1}\n", 33)),
		entered: make(chan struct{}, 1),
		gate:    make(chan struct{}),
		closes:  make(chan struct{}, 4),
	}
	ch := streamResponse(ctx, body)

	for i := 0; i < 17; i++ {
		ev := <-ch
		require.NoError(t, ev.Error)
	}
	<-body.entered // goroutine is inside the second Read, past every send
	cancel()

	<-body.closes // watcher's Close on ctx.Done (its panic is recovered)
	<-body.closes // main goroutine's deferred Close at exit

	total := 0
	for ev := range ch {
		total++
		assert.NoError(t, ev.Error)
	}
	assert.Equal(t, 16, total, "only the buffered events; error event skipped")
}

// gatedBody returns `first` immediately, then blocks until the gate is
// closed, then returns `second` and EOF afterwards.
type gatedBody struct {
	first  []byte
	second []byte
	gate   chan struct{}
	once   sync.Once
}

func (b *gatedBody) Read(p []byte) (int, error) {
	if b.first != nil {
		n := copy(p, b.first)
		b.first = nil
		return n, nil
	}
	<-b.gate
	if b.second != nil {
		n := copy(p, b.second)
		b.second = nil
		return n, nil
	}
	return 0, io.EOF
}

func (b *gatedBody) Close() error {
	b.once.Do(func() { close(b.gate) })
	return nil
}

func TestStreamResponseContextCancelAtLoopTop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	body := &gatedBody{
		first:  []byte("{\"a\":1}\n"),
		second: []byte("{\"b\":2}\n"),
		gate:   make(chan struct{}),
	}
	ch := streamResponse(ctx, body)

	ev := <-ch
	require.Equal(t, `{"a":1}`, string(ev.Data))

	cancel()                                  // cancel before the second line is released
	body.once.Do(func() { close(body.gate) }) // release the second line

	var events []StreamEvent
	for e := range ch {
		events = append(events, e)
	}
	assert.Empty(t, events, "loop-top ctx check should return before sending the second line")
}

func TestStreamResponseCancelWhileSendBlocked(t *testing.T) {
	// All lines are available in one Read: the goroutine fills the 16-slot
	// buffer and blocks on send #17. Yield to the scheduler so that state is
	// reached, then cancel - the blocked send-select must take ctx.Done.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := strings.Repeat("{\"n\":1}\n", 1000)
	ch := streamResponse(ctx, io.NopCloser(strings.NewReader(body)))

	for i := 0; i < 100000; i++ {
		runtime.Gosched()
	}
	cancel()

	total := 0
	for ev := range ch {
		total++
		assert.NoError(t, ev.Error)
	}
	assert.LessOrEqual(t, total, 16, "at most the buffered events are delivered")
}
