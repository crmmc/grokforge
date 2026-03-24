package xai

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
)

// streamResponse reads newline-delimited JSON from body and sends events to channel.
// The channel is closed when the stream ends, context is cancelled, or an error occurs.
// Empty lines are skipped.
func streamResponse(ctx context.Context, body io.ReadCloser) <-chan StreamEvent {
	ch := make(chan StreamEvent, 16)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("stream goroutine panic recovered", "panic", r)
			}
		}()
		defer close(ch)
		defer body.Close()
		doneCh := make(chan struct{})
		defer close(doneCh)

		// Close body when context is cancelled to unblock scanner
		go func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("stream cancel watcher panic recovered", "panic", r)
				}
			}()
			select {
			case <-ctx.Done():
				body.Close()
			case <-doneCh:
			}
		}()

		scanner := bufio.NewScanner(body)
		// Increase buffer for large responses
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)

		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
			}

			line := scanner.Text()
			if line == "" {
				continue
			}

			select {
			case <-ctx.Done():
				return
			case ch <- StreamEvent{Data: json.RawMessage(line)}:
			}
		}

		if err := scanner.Err(); err != nil {
			// Don't report error if context was cancelled
			select {
			case <-ctx.Done():
				return
			case ch <- StreamEvent{Error: err}:
			}
		}
	}()

	return ch
}
