package token

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"
)

// testCaptureHandler captures slog records and signals once a message matches.
// It lets tests synchronize on log output instead of sleeping.
type testCaptureHandler struct {
	mu      sync.Mutex
	records []string
	match   string
	signal  chan struct{}
	once    sync.Once
}

func newCaptureHandler(match string) *testCaptureHandler {
	return &testCaptureHandler{match: match, signal: make(chan struct{})}
}

func (h *testCaptureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *testCaptureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	h.records = append(h.records, r.Message)
	h.mu.Unlock()
	if r.Message == h.match {
		h.once.Do(func() { close(h.signal) })
	}
	return nil
}

func (h *testCaptureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *testCaptureHandler) WithGroup(string) slog.Handler      { return h }

func TestSafeGo_RecoversPanic(t *testing.T) {
	handler := newCaptureHandler("goroutine panic recovered")
	orig := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(orig) })

	safeGo("extra_test_panic", func() {
		panic("boom")
	})

	select {
	case <-handler.signal:
	case <-time.After(2 * time.Second):
		t.Fatal("panic inside safeGo was not recovered")
	}
}
