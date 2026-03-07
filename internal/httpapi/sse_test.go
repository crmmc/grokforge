package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSSEWriter(t *testing.T) {
	w := httptest.NewRecorder()
	sw := NewSSEWriter(w)

	require.NotNil(t, sw)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
	assert.Equal(t, "no", w.Header().Get("X-Accel-Buffering"))
}

func TestSSEWriter_WriteSSE(t *testing.T) {
	w := httptest.NewRecorder()
	sw := NewSSEWriter(w)

	data := map[string]string{"content": "hello"}
	err := sw.WriteSSE(data)

	require.NoError(t, err)
	body := w.Body.String()
	assert.True(t, strings.HasPrefix(body, "data: "))
	assert.True(t, strings.HasSuffix(body, "\n\n"))
	assert.Contains(t, body, `"content":"hello"`)
}

func TestSSEWriter_WriteSSEDone(t *testing.T) {
	w := httptest.NewRecorder()
	sw := NewSSEWriter(w)

	sw.WriteSSEDone()

	assert.Equal(t, "data: [DONE]\n\n", w.Body.String())
}

func TestSSEWriter_WriteSSEError(t *testing.T) {
	w := httptest.NewRecorder()
	sw := NewSSEWriter(w)

	apiErr := NewAPIError(500, "server_error", "internal_error", "something went wrong")
	sw.WriteSSEError(apiErr)

	body := w.Body.String()
	// Should have event: error line
	assert.Contains(t, body, "event: error\n")
	// Should have data with error JSON
	assert.Contains(t, body, `"message":"something went wrong"`)
	// Should end with [DONE]
	assert.True(t, strings.HasSuffix(body, "data: [DONE]\n\n"))
}
