package xai

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
)

// streamResponse reads newline-delimited JSON from body and sends events to channel.
// The channel is closed when the stream ends, context is cancelled, or an error occurs.
// Empty lines are skipped.
func streamResponse(ctx context.Context, body io.ReadCloser) <-chan StreamEvent {
	ch := make(chan StreamEvent, 16)

	go func() {
		defer close(ch)
		defer body.Close()

		// Close body when context is cancelled to unblock scanner
		go func() {
			<-ctx.Done()
			body.Close()
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
