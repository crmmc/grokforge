//go:build cgo

package transport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestCurlState() (*curlState, *io.PipeReader) {
	reader, writer := io.Pipe()
	return &curlState{
		ctx:         context.Background(),
		bodyWriter:  writer,
		header:      make(http.Header),
		headerReady: make(chan struct{}),
	}, reader
}

func TestBoolToCInt(t *testing.T) {
	assert.EqualValues(t, 1, boolToCInt(true))
	assert.EqualValues(t, 0, boolToCInt(false))
}

func TestCStringArray(t *testing.T) {
	t.Run("empty returns nil with noop cleanup", func(t *testing.T) {
		for _, values := range [][]string{nil, {}} {
			array, cleanup := cStringArray(values)
			assert.Nil(t, array)
			require.NotPanics(t, cleanup)
		}
	})

	t.Run("values allocate cleanup frees", func(t *testing.T) {
		array, cleanup := cStringArray([]string{"X-Test: v1", "X-Other: v2"})
		require.NotNil(t, array)
		require.NotPanics(t, cleanup)
	})
}

func TestLoadCurlState(t *testing.T) {
	t.Run("unknown handle", func(t *testing.T) {
		state, ok := loadCurlState(12345)
		assert.False(t, ok)
		assert.Nil(t, state)
	})

	t.Run("stored handle", func(t *testing.T) {
		const handle = uintptr(987654321)
		want, reader := newTestCurlState()
		defer reader.Close()
		curlHandles.Store(handle, want)
		defer curlHandles.Delete(handle)

		got, ok := loadCurlState(handle)
		assert.True(t, ok)
		assert.Same(t, want, got)
	})
}

func TestCurlState_SetStatus(t *testing.T) {
	state, reader := newTestCurlState()
	defer reader.Close()

	state.setStatus(503)
	state.mu.Lock()
	defer state.mu.Unlock()
	assert.Equal(t, 503, state.statusCode)
}

func TestCurlState_SignalHeaders(t *testing.T) {
	state, reader := newTestCurlState()
	defer reader.Close()

	firstErr := errors.New("first")
	state.signalHeaders(firstErr)

	select {
	case <-state.headerReady:
	default:
		t.Fatal("headerReady should be closed after first signal")
	}

	// Second signal must be a no-op (sync.Once).
	secondErr := errors.New("second")
	state.signalHeaders(secondErr)

	state.mu.Lock()
	gotErr := state.headerErr
	state.mu.Unlock()
	assert.Same(t, firstErr, gotErr)
}

func TestCurlState_WriteBody(t *testing.T) {
	t.Run("writes to pipe", func(t *testing.T) {
		state, reader := newTestCurlState()
		defer reader.Close()

		done := make(chan string, 1)
		go func() {
			b, err := io.ReadAll(reader)
			if err != nil {
				done <- "read error: " + err.Error()
				return
			}
			done <- string(b)
		}()

		assert.Equal(t, 2, state.writeBody([]byte("hi")))
		assert.NoError(t, state.bodyWriter.Close())
		assert.Equal(t, "hi", <-done)
	})

	t.Run("canceled context returns zero", func(t *testing.T) {
		state, reader := newTestCurlState()
		defer reader.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		state.ctx = ctx

		assert.Equal(t, 0, state.writeBody([]byte("hi")))
	})

	t.Run("write error returns zero", func(t *testing.T) {
		state, reader := newTestCurlState()
		require.NoError(t, reader.CloseWithError(errors.New("reader gone")))

		assert.Equal(t, 0, state.writeBody([]byte("hi")))
	})
}

func TestCurlState_WriteHeaderLine(t *testing.T) {
	tests := []struct {
		name          string
		line          string
		preStatus     int
		preHeader     http.Header
		wantStatus    int
		wantHeaderKey string
		wantHeaderVal string
		wantSignaled  bool
	}{
		{
			name:          "status line sets status and resets header",
			line:          "HTTP/1.1 404 Not Found\r\n",
			preStatus:     200,
			preHeader:     http.Header{"X-Old": {"1"}},
			wantStatus:    404,
			wantHeaderKey: "",
		},
		{
			name:          "lowercase status line parses",
			line:          "http/1.1 201 created\r\n",
			wantStatus:    201,
			wantHeaderKey: "",
		},
		{
			name:          "invalid status number ignored",
			line:          "HTTP/1.1 abc def\r\n",
			preStatus:     200,
			wantStatus:    200,
			wantHeaderKey: "",
		},
		{
			name:          "status line without code ignored",
			line:          "HTTP/\r\n",
			preStatus:     200,
			wantStatus:    200,
			wantHeaderKey: "",
		},
		{
			name:          "header line added canonically",
			line:          "x-reason: because\r\n",
			wantStatus:    0,
			wantHeaderKey: "X-Reason",
			wantHeaderVal: "because",
		},
		{
			name:          "line without colon ignored",
			line:          "garbage-header\r\n",
			wantStatus:    0,
			wantHeaderKey: "",
		},
		{
			name:         "empty line signals headers",
			line:         "\r\n",
			wantStatus:   0,
			wantSignaled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, reader := newTestCurlState()
			defer reader.Close()
			if tt.preStatus != 0 {
				state.statusCode = tt.preStatus
			}
			if tt.preHeader != nil {
				state.header = tt.preHeader
			}

			n := state.writeHeaderLine(tt.line)
			assert.Equal(t, len(tt.line), n)

			state.mu.Lock()
			status := state.statusCode
			var gotVal string
			if tt.wantHeaderKey != "" {
				gotVal = state.header.Get(tt.wantHeaderKey)
			} else if tt.preHeader != nil {
				gotVal = state.header.Get("X-Old")
			}
			state.mu.Unlock()

			assert.Equal(t, tt.wantStatus, status)
			if tt.wantHeaderKey != "" {
				assert.Equal(t, tt.wantHeaderVal, gotVal)
			}
			if tt.preHeader != nil && tt.wantHeaderKey == "" && !tt.wantSignaled {
				assert.Empty(t, gotVal, "header map should be reset by status line")
			}
			if tt.wantSignaled {
				select {
				case <-state.headerReady:
				default:
					t.Fatal("empty line should signal header readiness")
				}
			}
		})
	}

	t.Run("canceled context returns zero", func(t *testing.T) {
		state, reader := newTestCurlState()
		defer reader.Close()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		state.ctx = ctx

		assert.Equal(t, 0, state.writeHeaderLine("HTTP/1.1 200 OK\r\n"))
	})
}
