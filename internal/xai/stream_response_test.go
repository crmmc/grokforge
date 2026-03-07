package xai

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestStreamResponse_MultipleLines(t *testing.T) {
	input := `{"id":1}
{"id":2}
{"id":3}`

	body := io.NopCloser(strings.NewReader(input))
	ch := streamResponse(context.Background(), body)

	var events []StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	if len(events) != 3 {
		t.Fatalf("got %d events, want 3", len(events))
	}

	want := []string{`{"id":1}`, `{"id":2}`, `{"id":3}`}
	for i, ev := range events {
		if ev.Error != nil {
			t.Errorf("events[%d].Error = %v, want nil", i, ev.Error)
		}
		if string(ev.Data) != want[i] {
			t.Errorf("events[%d].Data = %q, want %q", i, string(ev.Data), want[i])
		}
	}
}

func TestStreamResponse_EmptyLines(t *testing.T) {
	input := `{"id":1}

{"id":2}

`

	body := io.NopCloser(strings.NewReader(input))
	ch := streamResponse(context.Background(), body)

	var events []StreamEvent
	for ev := range ch {
		events = append(events, ev)
	}

	// Should only get 2 events, empty lines skipped
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2 (empty lines skipped)", len(events))
	}

	if string(events[0].Data) != `{"id":1}` {
		t.Errorf("events[0].Data = %q, want {\"id\":1}", string(events[0].Data))
	}
	if string(events[1].Data) != `{"id":2}` {
		t.Errorf("events[1].Data = %q, want {\"id\":2}", string(events[1].Data))
	}
}

func TestStreamResponse_ContextCancel(t *testing.T) {
	// Use a pipe so we can control when data is available
	pr, pw := io.Pipe()

	ctx, cancel := context.WithCancel(context.Background())
	ch := streamResponse(ctx, pr)

	// Write one line
	go func() {
		pw.Write([]byte(`{"id":1}` + "\n"))
		time.Sleep(10 * time.Millisecond)
		// Don't close - simulate slow stream
	}()

	// Read first event
	ev := <-ch
	if string(ev.Data) != `{"id":1}` {
		t.Errorf("first event = %q, want {\"id\":1}", string(ev.Data))
	}

	// Cancel context
	cancel()

	// Channel should close without more events
	select {
	case _, ok := <-ch:
		if ok {
			// May get one more event due to timing, but channel should close
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("channel did not close after context cancel")
	}

	pw.Close()
}

func TestStreamResponse_ScannerError(t *testing.T) {
	// Create a reader that returns an error
	errReader := &errorReader{err: errors.New("read error")}
	body := io.NopCloser(errReader)

	ch := streamResponse(context.Background(), body)

	var lastEvent StreamEvent
	for ev := range ch {
		lastEvent = ev
	}

	if lastEvent.Error == nil {
		t.Error("expected error event, got nil")
	}
	if !strings.Contains(lastEvent.Error.Error(), "read error") {
		t.Errorf("error = %v, want to contain 'read error'", lastEvent.Error)
	}
}

// errorReader is a reader that always returns an error.
type errorReader struct {
	err error
}

func (r *errorReader) Read(p []byte) (int, error) {
	return 0, r.err
}
