package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/cache"
	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/flow"
	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterToolCalls(t *testing.T) {
	calls := []flow.ToolCall{
		{Function: flow.FunctionCall{Name: "allowed", Arguments: "{}"}},
		{Function: flow.FunctionCall{Name: "blocked", Arguments: "{}"}},
	}

	t.Run("no tools passes through", func(t *testing.T) {
		got := filterToolCalls(calls, nil)
		assert.Equal(t, calls, got)
	})

	t.Run("filters by tool name", func(t *testing.T) {
		tools := []flow.Tool{{Type: "function", Function: flow.Function{Name: "allowed"}}}
		got := filterToolCalls(calls, tools)
		require.Len(t, got, 1)
		assert.Equal(t, "allowed", got[0].Function.Name)
	})

	t.Run("empty name tools are not allowed", func(t *testing.T) {
		tools := []flow.Tool{{Type: "function", Function: flow.Function{Name: ""}}}
		got := filterToolCalls(calls, tools)
		assert.Empty(t, got)
	})
}

func TestFormatToolCallsAsText(t *testing.T) {
	assert.Empty(t, formatToolCallsAsText(nil))

	got := formatToolCallsAsText([]flow.ToolCall{
		{Function: flow.FunctionCall{Name: "lookup", Arguments: `{"q":"x"}`}},
		{Function: flow.FunctionCall{Name: "", Arguments: "{}"}},
		{Function: flow.FunctionCall{Name: "noargs"}},
	})
	want := toolCallStartTag + `{"name":"lookup","arguments":{"q":"x"}}` + toolCallEndTag +
		toolCallStartTag + `{"name":"noargs","arguments":{}}` + toolCallEndTag
	assert.Equal(t, want, got)
}

func TestWriteStreamingOrJSONError(t *testing.T) {
	h := &Handler{}
	err := upstream.ErrRateLimited

	t.Run("json when streaming disabled", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.writeStreamingOrJSONError(w, nil, err)
		assert.Equal(t, http.StatusTooManyRequests, w.Code)
		assert.Contains(t, w.Body.String(), "rate_limit_error")
	})

	t.Run("sse when streaming enabled", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.writeStreamingOrJSONError(w, boolPtr(true), err)
		assert.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, "rate_limit_error")
		assert.Contains(t, body, "data: [DONE]")
	})
}

func TestWriteMediaProxyError(t *testing.T) {
	err := errors.New("boom")

	t.Run("json when streaming disabled", func(t *testing.T) {
		w := httptest.NewRecorder()
		writeMediaProxyError(w, nil, err)
		assert.Equal(t, http.StatusBadGateway, w.Code)
		var resp struct {
			Error struct {
				Code string `json:"code"`
				Type string `json:"type"`
			} `json:"error"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, "media_proxy_failed", resp.Error.Code)
		assert.Equal(t, "server_error", resp.Error.Type)
	})

	t.Run("sse when streaming enabled", func(t *testing.T) {
		w := httptest.NewRecorder()
		writeMediaProxyError(w, boolPtr(true), err)
		assert.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, "media_proxy_failed")
		assert.Contains(t, body, "data: [DONE]")
	})
}

func TestMediaRewriteAPIError(t *testing.T) {
	apiErr := mediaRewriteAPIError(errors.New("boom"))
	assert.Equal(t, http.StatusBadGateway, apiErr.Status)
	assert.Equal(t, "media_proxy_failed", apiErr.Error.Code)
	assert.Equal(t, "server_error", apiErr.Error.Type)
}

func TestRewriteBlockingContent(t *testing.T) {
	h := &Handler{}
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	r.Host = "api.example.test"
	content := "plain answer"

	t.Run("nil config passthrough", func(t *testing.T) {
		got, err := h.rewriteBlockingContent(blockingRewriteInput{r: r, content: content})
		require.Nil(t, err)
		assert.Equal(t, content, got)
	})

	t.Run("media disabled passthrough", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.App.MediaGenerationEnabled = false
		got, err := h.rewriteBlockingContent(blockingRewriteInput{r: r, cfg: cfg, content: content})
		require.Nil(t, err)
		assert.Equal(t, content, got)
	})

	t.Run("media enabled without downloader passthrough", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.App.MediaGenerationEnabled = true
		got, err := h.rewriteBlockingContent(blockingRewriteInput{r: r, cfg: cfg, content: content})
		require.Nil(t, err)
		assert.Equal(t, content, got)
	})

	t.Run("media enabled rewrites local files", func(t *testing.T) {
		cfg := config.DefaultConfig()
		cfg.App.MediaGenerationEnabled = true
		cfg.Image.Format = config.ImageFormatLocalURL
		h := &Handler{Cfg: cfg, CacheService: cache.NewService(t.TempDir(), nil)}
		dl := func(ctx context.Context, url string) ([]byte, error) {
			return testPNGBytes(), nil
		}
		got, err := h.rewriteBlockingContent(blockingRewriteInput{
			r:        r,
			cfg:      cfg,
			download: dl,
			content:  "![img](https://assets.grok.com/users/u/generated/a.png)",
		})
		require.Nil(t, err)
		assert.Contains(t, got, "http://api.example.test/api/files/image/")
	})
}

func TestBlockingResponse_RewriteFailureReturnsMediaProxyError(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.App.MediaGenerationEnabled = true
	h := &Handler{Cfg: cfg}
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	dl := func(ctx context.Context, url string) ([]byte, error) {
		return []byte("definitely not an image"), nil
	}
	eventCh := make(chan flow.StreamEvent, 1)
	eventCh <- flow.StreamEvent{
		Content:    "![img](https://assets.grok.com/users/u/generated/a.png)",
		Downloader: dl,
	}
	close(eventCh)

	w := httptest.NewRecorder()
	chatReq := &ChatRequest{Model: "grok-3", Messages: baseValidMessages()}
	h.blockingResponse(w, r, eventCh, chatReq)

	assert.Equal(t, http.StatusBadGateway, w.Code)
	assert.Contains(t, w.Body.String(), "media_proxy_failed")
}

func TestBlockingResponse_EmptyStreamStillReturnsChoice(t *testing.T) {
	h := &Handler{}
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	eventCh := make(chan flow.StreamEvent, 1)
	close(eventCh)

	w := httptest.NewRecorder()
	h.blockingResponse(w, r, eventCh, &ChatRequest{Model: "grok-3"})

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"content":""`)
	assert.Contains(t, w.Body.String(), `"finish_reason":"stop"`)
}

// strictResponseWriter implements http.ResponseWriter without http.Flusher,
// exercising the streaming-unsupported branch.
type strictResponseWriter struct {
	header http.Header
	body   strings.Builder
	code   int
}

func newStrictResponseWriter() *strictResponseWriter {
	return &strictResponseWriter{header: make(http.Header)}
}

func (w *strictResponseWriter) Header() http.Header         { return w.header }
func (w *strictResponseWriter) Write(b []byte) (int, error) { return w.body.Write(b) }
func (w *strictResponseWriter) WriteHeader(code int)        { w.code = code }

func TestStreamResponse_RequiresFlusher(t *testing.T) {
	h := &Handler{Cfg: config.DefaultConfig()}
	w := newStrictResponseWriter()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	eventCh := make(chan flow.StreamEvent, 1)
	close(eventCh)

	h.streamResponse(w, r, eventCh, &ChatRequest{Model: "grok-3"})

	assert.Equal(t, http.StatusInternalServerError, w.code)
	assert.Contains(t, w.body.String(), "streaming_unsupported")
}

func TestStreamResponse_ErrorEventEndsStream(t *testing.T) {
	h := &Handler{Cfg: config.DefaultConfig()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	eventCh := make(chan flow.StreamEvent, 1)
	eventCh <- flow.StreamEvent{Error: upstream.ErrRateLimited}
	close(eventCh)

	h.streamResponse(w, r, eventCh, &ChatRequest{Model: "grok-3"})

	body := w.Body.String()
	assert.Contains(t, body, `"error"`)
	assert.Contains(t, body, "rate_limit_error")
	// WriteSSEError terminates the stream with [DONE] after the error payload.
	assert.Contains(t, body, "data: [DONE]")
	assert.NotContains(t, body, `"finish_reason"`)
}

func TestStreamResponse_ContextCanceled(t *testing.T) {
	h := &Handler{Cfg: config.DefaultConfig()}
	w := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).WithContext(ctx)

	// Never-closed channel: only the canceled context can end the loop.
	eventCh := make(chan flow.StreamEvent)
	cancel()

	h.streamResponse(w, r, eventCh, &ChatRequest{Model: "grok-3"})

	// Handler returns promptly; body contains the role chunk only.
	assert.Contains(t, w.Body.String(), `"role":"assistant"`)
}

// flushableFailingWriter implements Flusher but fails every write.
type flushableFailingWriter struct{}

func (w *flushableFailingWriter) Header() http.Header       { return http.Header{} }
func (w *flushableFailingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }
func (w *flushableFailingWriter) WriteHeader(int)           {}
func (w *flushableFailingWriter) Flush()                    {}

func TestStreamResponse_RoleChunkWriteError(t *testing.T) {
	h := &Handler{Cfg: config.DefaultConfig()}
	w := &flushableFailingWriter{}
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	eventCh := make(chan flow.StreamEvent, 1)
	close(eventCh)

	h.streamResponse(w, r, eventCh, &ChatRequest{Model: "grok-3"})
	// streamResponse returns after mapping the write failure to an SSE error
	// attempt on the same writer; nothing more can be asserted on the body.
}

func TestStreamResponse_HandleEventMediaErrorEndsStream(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.App.MediaGenerationEnabled = true
	h := &Handler{Cfg: cfg}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	eventCh := make(chan flow.StreamEvent, 1)
	eventCh <- flow.StreamEvent{
		Content:    "![img](https://assets.grok.com/users/u/generated/g.png)",
		Downloader: failingDownload,
	}
	close(eventCh)

	h.streamResponse(w, r, eventCh, &ChatRequest{Model: "grok-3"})

	body := w.Body.String()
	assert.Contains(t, body, `"error"`)
	assert.NotContains(t, body, "assets.grok.com")
}

func TestStreamResponse_HeldContentFlushedAtFinish(t *testing.T) {
	h := &Handler{Cfg: config.DefaultConfig()}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	eventCh := make(chan flow.StreamEvent, 1)
	eventCh <- flow.StreamEvent{Content: "trailing ![img"}
	close(eventCh)

	h.streamResponse(w, r, eventCh, &ChatRequest{Model: "grok-3"})

	body := w.Body.String()
	assert.Contains(t, body, "trailing ")
	assert.Contains(t, body, "![img")
	assert.Contains(t, body, "data: [DONE]")
}

func TestStreamResponse_FinishMediaGateErrorEndsStream(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.App.MediaGenerationEnabled = true
	h := &Handler{Cfg: cfg}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	eventCh := make(chan flow.StreamEvent, 1)
	eventCh <- flow.StreamEvent{
		Content:    "![img](https://assets.grok.com/users/u/generated/g.png",
		Downloader: failingDownload,
	}
	close(eventCh)

	h.streamResponse(w, r, eventCh, &ChatRequest{Model: "grok-3"})

	body := w.Body.String()
	assert.Contains(t, body, `"error"`)
	assert.NotContains(t, body, "assets.grok.com")
}
