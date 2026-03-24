package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// SSEWriter wraps http.ResponseWriter for Server-Sent Events output.
type SSEWriter struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// NewSSEWriter creates a new SSE writer with proper headers.
// Sets Content-Type: text/event-stream, Cache-Control: no-cache,
// and X-Accel-Buffering: no for nginx proxy compatibility.
func NewSSEWriter(w http.ResponseWriter) *SSEWriter {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, _ := w.(http.Flusher)
	return &SSEWriter{w: w, flusher: flusher}
}

// WriteSSE writes a data event in SSE format: data: {json}\n\n
func (s *SSEWriter) WriteSSE(data any) error {
	bytes, err := json.Marshal(data)
	if err != nil {
		return err
	}
	fmt.Fprintf(s.w, "data: %s\n\n", bytes)
	if s.flusher != nil {
		s.flusher.Flush()
	}
	return nil
}

// WriteSSEDone writes the final [DONE] event.
func (s *SSEWriter) WriteSSEDone() {
	fmt.Fprint(s.w, "data: [DONE]\n\n")
	if s.flusher != nil {
		s.flusher.Flush()
	}
}

// WriteSSEError writes an OpenAI-compatible error payload followed by [DONE].
func (s *SSEWriter) WriteSSEError(apiErr *APIError) {
	bytes, _ := json.Marshal(apiErr)
	fmt.Fprintf(s.w, "data: %s\n\n", bytes)
	if s.flusher != nil {
		s.flusher.Flush()
	}
	s.WriteSSEDone()
}
