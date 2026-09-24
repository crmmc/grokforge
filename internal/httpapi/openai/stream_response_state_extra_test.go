package openai

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/flow"
	"github.com/crmmc/grokforge/internal/httpapi"
	tkn "github.com/crmmc/grokforge/internal/token"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingResponseWriter errors on every Write.
type failingResponseWriter struct{}

func (w *failingResponseWriter) Header() http.Header       { return http.Header{} }
func (w *failingResponseWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }
func (w *failingResponseWriter) WriteHeader(int)           {}
func (w *failingResponseWriter) Flush()                    {}

func newTestStreamState(t *testing.T, h *Handler, cfg *config.Config, w http.ResponseWriter, r *http.Request, req *ChatRequest) *streamResponseState {
	t.Helper()
	return newStreamResponseState(streamResponseOptions{
		h:       h,
		r:       r,
		writer:  httpapi.NewSSEWriter(w),
		flusher: w.(http.Flusher),
		req:     req,
		cfg:     cfg,
	})
}

func TestIsContentOnlyChunk(t *testing.T) {
	tests := []struct {
		name  string
		chunk chatStreamChunk
		want  bool
	}{
		{
			name:  "no choices",
			chunk: chatStreamChunk{},
			want:  false,
		},
		{
			name:  "multiple choices",
			chunk: chatStreamChunk{Choices: []chatStreamChoice{{}, {}}},
			want:  false,
		},
		{
			name:  "with finish reason",
			chunk: chatStreamChunk{Choices: []chatStreamChoice{{FinishReason: strPtr("stop")}}},
			want:  false,
		},
		{
			name:  "role delta",
			chunk: chatStreamChunk{Choices: []chatStreamChoice{{Delta: chatStreamDelta{Role: "assistant", Content: "x"}}}},
			want:  false,
		},
		{
			name:  "with tool calls",
			chunk: chatStreamChunk{Choices: []chatStreamChoice{{Delta: chatStreamDelta{Content: "x", ToolCalls: []flow.ToolCall{{}}}}}},
			want:  false,
		},
		{
			name:  "content only",
			chunk: chatStreamChunk{Choices: []chatStreamChoice{{Delta: chatStreamDelta{Content: "text"}}}},
			want:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, isContentOnlyChunk(tc.chunk))
		})
	}
}

func strPtr(s string) *string { return &s }

func TestGuardChunk_PassthroughWithoutContent(t *testing.T) {
	h := &Handler{Cfg: config.DefaultConfig()}
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	s := newTestStreamState(t, h, h.Cfg, httptest.NewRecorder(), r, &ChatRequest{Model: "grok-3"})

	noChoice := chatStreamChunk{}
	got, emit, err := s.guardChunk(noChoice)
	require.Nil(t, err)
	assert.True(t, emit)
	assert.Equal(t, noChoice, got)

	emptyDelta := chatStreamChunk{Choices: []chatStreamChoice{{Delta: chatStreamDelta{Role: "assistant"}}}}
	got, emit, err = s.guardChunk(emptyDelta)
	require.Nil(t, err)
	assert.True(t, emit)
	assert.Equal(t, emptyDelta, got)
}

func TestGuardChunk_HoldsContentUntilSafe(t *testing.T) {
	cfg := config.DefaultConfig()
	h := &Handler{Cfg: cfg}
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	s := newTestStreamState(t, h, cfg, httptest.NewRecorder(), r, &ChatRequest{Model: "grok-3"})

	chunk := chatStreamChunk{Choices: []chatStreamChoice{{Delta: chatStreamDelta{Content: "trailing ![img"}}}}
	got, emit, err := s.guardChunk(chunk)
	require.Nil(t, err)
	// Only the safe prefix flushes; the incomplete markdown stays buffered.
	assert.True(t, emit)
	assert.Equal(t, "trailing ", got.Choices[0].Delta.Content)
	assert.Equal(t, "![img", s.mediaGate.pending)

	// A chunk that is entirely unsafe and content-only is suppressed entirely.
	held := chatStreamChunk{Choices: []chatStreamChoice{{Delta: chatStreamDelta{Content: "](x"}}}}
	got, emit, err = s.guardChunk(held)
	require.Nil(t, err)
	assert.False(t, emit, "fully held content-only chunk must be suppressed")
	assert.Equal(t, "![img](x", s.mediaGate.pending)
}

func TestGuardChunk_RewriteError(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.App.MediaGenerationEnabled = true
	h := &Handler{Cfg: cfg}
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	s := newTestStreamState(t, h, cfg, httptest.NewRecorder(), r, &ChatRequest{Model: "grok-3"})
	s.rewriter = newMediaRewriter(failingDownload, nil, "base64", nil)

	chunk := chatStreamChunk{Choices: []chatStreamChoice{{Delta: chatStreamDelta{Content: "![img](https://assets.grok.com/users/u/g.png)"}}}}
	_, _, err := s.guardChunk(chunk)
	require.NotNil(t, err)
}

func TestHandleEvent_WriterError(t *testing.T) {
	cfg := config.DefaultConfig()
	h := &Handler{Cfg: cfg}
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	s := newTestStreamState(t, h, cfg, &failingResponseWriter{}, r, &ChatRequest{Model: "grok-3"})

	err := s.handleEvent(flow.StreamEvent{Content: "hello"})
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "write failed")
}

func TestFinish_FlushError(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.App.MediaGenerationEnabled = true
	h := &Handler{Cfg: cfg}
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	s := newTestStreamState(t, h, cfg, httptest.NewRecorder(), r, &ChatRequest{Model: "grok-3"})
	s.rewriter = newMediaRewriter(failingDownload, nil, "base64", nil)
	s.mediaGate.pending = "![img](https://assets.grok.com/users/u/g.png)"

	err := s.finish()
	require.NotNil(t, err)
}

func TestFinish_WritesTailAfterGate(t *testing.T) {
	cfg := config.DefaultConfig()
	h := &Handler{Cfg: cfg}
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	s := newTestStreamState(t, h, cfg, w, r, &ChatRequest{Model: "grok-3"})
	s.mediaGate.pending = "tail text"

	require.Nil(t, s.finish())
	assert.Contains(t, w.Body.String(), "tail text")
}

func TestFinish_WriterError(t *testing.T) {
	cfg := config.DefaultConfig()
	h := &Handler{Cfg: cfg}
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	s := newTestStreamState(t, h, cfg, &failingResponseWriter{}, r, &ChatRequest{Model: "grok-3"})
	s.mediaGate.pending = "tail text"

	err := s.finish()
	require.NotNil(t, err)
}

func TestEnsureMediaRewriter_GatedByConfig(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	t.Run("disabled config skips rewriter", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.App.MediaGenerationEnabled = false
		h := &Handler{Cfg: cfg}
		s := newTestStreamState(t, h, cfg, httptest.NewRecorder(), r, &ChatRequest{Model: "grok-3"})
		s.ensureMediaRewriter(flow.StreamEvent{Downloader: failingDownload})
		assert.Nil(t, s.rewriter)
	})

	t.Run("enabled config creates rewriter", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.App.MediaGenerationEnabled = true
		h := &Handler{Cfg: cfg, CacheService: nil}
		s := newTestStreamState(t, h, cfg, httptest.NewRecorder(), r, &ChatRequest{Model: "grok-3"})
		s.ensureMediaRewriter(flow.StreamEvent{Downloader: pngDownload})
		require.NotNil(t, s.rewriter)
	})

	t.Run("no downloader no rewriter", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.App.MediaGenerationEnabled = true
		h := &Handler{Cfg: cfg}
		s := newTestStreamState(t, h, cfg, httptest.NewRecorder(), r, &ChatRequest{Model: "grok-3"})
		s.ensureMediaRewriter(flow.StreamEvent{})
		assert.Nil(t, s.rewriter)
	})
}

func TestWriteError_MapsUpstreamError(t *testing.T) {
	cfg := config.DefaultConfig()
	h := &Handler{Cfg: cfg}
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	s := newTestStreamState(t, h, cfg, w, r, &ChatRequest{Model: "grok-3"})

	s.writeError(tkn.ErrNoTokenAvailable)
	body := w.Body.String()
	assert.Contains(t, body, "no_token_available")
	assert.Contains(t, body, "data: [DONE]")
}
