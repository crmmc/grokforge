package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingResponseWriter fails every Write and does not implement http.Flusher.
type failingResponseWriter struct {
	header http.Header
}

func (w *failingResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = http.Header{}
	}
	return w.header
}

func (w *failingResponseWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }
func (w *failingResponseWriter) WriteHeader(int)           {}

func TestSSEWriter_WriteSSE_MarshalError(t *testing.T) {
	w := httptest.NewRecorder()
	sw := NewSSEWriter(w)

	err := sw.WriteSSE(make(chan int)) // channels cannot be marshaled to JSON

	require.Error(t, err)
	assert.Empty(t, w.Body.String())
}

func TestSSEWriter_WriteSSE_WriteError(t *testing.T) {
	sw := NewSSEWriter(&failingResponseWriter{})

	err := sw.WriteSSE(map[string]string{"a": "b"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "write failed")
}

func TestSSEWriter_WriteSSEDone_WriteError(t *testing.T) {
	sw := NewSSEWriter(&failingResponseWriter{})

	sw.WriteSSEDone() // must not panic on write error
}

func TestSSEWriter_WriteSSEError_WriteError(t *testing.T) {
	sw := NewSSEWriter(&failingResponseWriter{})

	sw.WriteSSEError(NewAPIError(500, "server_error", "x", "y")) // must not panic
}

func TestSSEWriter_NonFlusherWriterStillWrites(t *testing.T) {
	w := &failingResponseWriter{}
	sw := NewSSEWriter(w)

	require.NotNil(t, sw)
	assert.Nil(t, sw.flusher, "writer without Flusher must leave flusher nil")
}
