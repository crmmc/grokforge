package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/flow"
	"github.com/stretchr/testify/assert"
)

func TestStreamResponse_ReasoningContent_GrokReference_ReturnsError(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.App.Thinking = true
	cfg.App.MediaGenerationEnabled = true
	h := &Handler{Cfg: cfg}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	streamReq := &ChatRequest{
		Model:           "grok-3",
		Messages:        []ChatMessage{{Role: "user", Content: "hello"}},
		Stream:          streamBoolPtr(true),
		ReasoningEffort: "low",
	}

	// Provide a downloader so rewriter is created
	dl := func(ctx context.Context, url string) ([]byte, error) {
		return nil, nil
	}

	eventCh := make(chan flow.StreamEvent, 2)
	eventCh <- flow.StreamEvent{
		ReasoningContent: "check https://assets.grok.com/users/x/generated/y.png",
		Downloader:       dl,
	}
	close(eventCh)

	h.streamResponse(w, req, eventCh, streamReq)

	body := w.Body.String()
	assert.Contains(t, body, "error")
	assert.NotContains(t, body, "assets.grok.com")
}

func TestStreamResponse_SplitGrokReference_ReturnsErrorWithoutLeakingPrefix(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.App.MediaGenerationEnabled = true
	h := &Handler{Cfg: cfg}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	streamReq := &ChatRequest{
		Model:    "grok-3",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
		Stream:   streamBoolPtr(true),
	}

	// Provide a downloader so rewriter is created and media gate can buffer split content
	dl := func(ctx context.Context, url string) ([]byte, error) {
		return nil, nil
	}

	eventCh := make(chan flow.StreamEvent, 3)
	eventCh <- flow.StreamEvent{
		Content:    "see https://assets.grok",
		Downloader: dl,
	}
	eventCh <- flow.StreamEvent{
		Content: ".com/users/x/generated/y.png",
	}
	close(eventCh)

	h.streamResponse(w, req, eventCh, streamReq)

	body := w.Body.String()
	assert.Contains(t, body, "error")
	assert.NotContains(t, body, "assets.grok")
}

// Removed TestStreamResponse_SplitBareGrokReference_ReturnsErrorWithoutLeakingPrefix
// because bare host matching (e.g., "assets.grok.com" without scheme) is no longer
// detected by leak detection after the fix to avoid false positives on normal
// conversation mentions of grok.com.

func TestStreamResponse_ChatMentionsGrokDotCom_PassesThrough(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.App.MediaGenerationEnabled = true
	h := &Handler{Cfg: cfg}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	streamReq := &ChatRequest{
		Model:    "grok-3",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
		Stream:   streamBoolPtr(true),
	}

	eventCh := make(chan flow.StreamEvent, 2)
	eventCh <- flow.StreamEvent{Content: "Grok is a product at grok.com"}
	close(eventCh)

	h.streamResponse(w, req, eventCh, streamReq)

	body := w.Body.String()
	// Should not contain error - normal chat mentioning grok.com should pass through
	assert.NotContains(t, body, "error")
	assert.Contains(t, body, "grok.com")
}

func TestStreamResponse_SplitMarkdownGrokImage_RewritesWithoutError(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.App.MediaGenerationEnabled = true
	h := &Handler{Cfg: cfg}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	streamReq := &ChatRequest{
		Model:    "grok-3",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
		Stream:   streamBoolPtr(true),
	}

	dl := func(ctx context.Context, url string) ([]byte, error) {
		return testPNGBytes(), nil
	}

	eventCh := make(chan flow.StreamEvent, 3)
	eventCh <- flow.StreamEvent{
		Content:    "![img](https://assets.grok",
		Downloader: dl,
	}
	eventCh <- flow.StreamEvent{
		Content: ".com/users/x/generated/y.png)",
	}
	close(eventCh)

	h.streamResponse(w, req, eventCh, streamReq)

	body := w.Body.String()
	assert.NotContains(t, body, "error")
	assert.NotContains(t, body, "assets.grok")
	assert.Contains(t, body, "data:image/png;base64,")
}

func TestStreamResponse_SplitMarkdownGrokImageBeforeClose_RewritesWithoutError(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.App.MediaGenerationEnabled = true
	h := &Handler{Cfg: cfg}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	streamReq := &ChatRequest{
		Model:    "grok-3",
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
		Stream:   streamBoolPtr(true),
	}

	dl := func(ctx context.Context, url string) ([]byte, error) {
		return testPNGBytes(), nil
	}

	eventCh := make(chan flow.StreamEvent, 3)
	eventCh <- flow.StreamEvent{
		Content:    "![img](https://assets.grok.com/users/x/generated/y.png",
		Downloader: dl,
	}
	eventCh <- flow.StreamEvent{
		Content: ")",
	}
	close(eventCh)

	h.streamResponse(w, req, eventCh, streamReq)

	body := w.Body.String()
	assert.NotContains(t, body, "error")
	assert.NotContains(t, body, "assets.grok")
	assert.Contains(t, body, "data:image/png;base64,")
}

func TestStreamResponse_FinishFlushesMediaGateBeforeThinkClose(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.App.Thinking = true
	cfg.App.MediaGenerationEnabled = true
	h := &Handler{Cfg: cfg}

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	streamReq := &ChatRequest{
		Model:           "grok-3",
		Messages:        []ChatMessage{{Role: "user", Content: "hello"}},
		Stream:          streamBoolPtr(true),
		ReasoningEffort: "low",
	}

	dl := func(ctx context.Context, url string) ([]byte, error) {
		return testPNGBytes(), nil
	}

	eventCh := make(chan flow.StreamEvent, 2)
	eventCh <- flow.StreamEvent{
		ReasoningContent: "tail https://example.com/final",
		Downloader:       dl,
	}
	close(eventCh)

	h.streamResponse(w, req, eventCh, streamReq)

	body := w.Body.String()
	assert.NotContains(t, body, "error")
	assert.Contains(t, body, `"content":"https://example.com/final"`)
	assert.Contains(t, body, `\u003c/think\u003e`)
}

func streamBoolPtr(b bool) *bool { return &b }

func testPNGBytes() []byte {
	return []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
}
