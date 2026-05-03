package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/flow"
	"github.com/crmmc/grokforge/internal/httpapi"
)

var errMediaRewriteDownload = errors.New("download failed")

func TestMediaRewriter_PassesThroughNilAndPlainGrokText(t *testing.T) {
	content := "see https://grok.com/img/abc/1.png, https://assets.grok.com/users/u/generated/id/image.png, and imagine-public"
	got, err := rewriteContent(nil, context.Background(), content)
	if err != nil {
		t.Fatalf("rewriteContent(nil) error = %v", err)
	}
	if got != content {
		t.Fatalf("rewriteContent(nil) = %q, want %q", got, content)
	}

	calls := 0
	rewriter := newMediaRewriter(countingFailingDownload(&calls))
	got, err = rewriteContent(rewriter, context.Background(), content)
	if err != nil {
		t.Fatalf("rewriteContent() error = %v", err)
	}
	if got != content || calls != 0 {
		t.Fatalf("plain text rewrite = %q, calls = %d", got, calls)
	}

	content = "![x](https://grok.com/about)"
	got, err = rewriteContent(rewriter, context.Background(), content)
	if err != nil {
		t.Fatalf("rewriteContent(non-media markdown) error = %v", err)
	}
	if got != content || calls != 0 {
		t.Fatalf("non-media markdown rewrite = %q, calls = %d", got, calls)
	}

	content = "![x](https://assets.grok.com/about)"
	got, err = rewriteContent(rewriter, context.Background(), content)
	if err != nil {
		t.Fatalf("rewriteContent(assets non-media) error = %v", err)
	}
	if got != content || calls != 0 {
		t.Fatalf("assets non-media rewrite = %q, calls = %d", got, calls)
	}
}

func TestMediaRewriter_RewritesGeneratedMarkdownImages(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantURL string
	}{
		{
			name:    "relative generated asset",
			content: "![x](users/u/generated/id/image.png)",
			wantURL: "https://assets.grok.com/users/u/generated/id/image.png",
		},
		{
			name:    "absolute assets generated asset",
			content: "![x](https://assets.grok.com/users/u/generated/id/image.png)",
			wantURL: "https://assets.grok.com/users/u/generated/id/image.png",
		},
		{
			name:    "absolute assets image chunk asset",
			content: "![x](https://assets.grok.com/cards/id/image.png)",
			wantURL: "https://assets.grok.com/cards/id/image.png",
		},
		{
			name:    "absolute grok image asset",
			content: "![x](https://grok.com/img/abc/1.png)",
			wantURL: "https://grok.com/img/abc/1.png",
		},
		{
			name:    "absolute grok legacy images asset",
			content: "![x](https://grok.com/images/123e4567-e89b-12d3-a456-426614174000.png)",
			wantURL: "https://grok.com/images/123e4567-e89b-12d3-a456-426614174000.png",
		},
		{
			name:    "http asset canonicalized to https",
			content: "![x](http://assets.grok.com/users/u/generated/id/image.png?token=1)",
			wantURL: "https://assets.grok.com/users/u/generated/id/image.png?token=1",
		},
		{
			name:    "http grok image canonicalized to https",
			content: "![x](http://grok.com/images/123e4567-e89b-12d3-a456-426614174000.png)",
			wantURL: "https://grok.com/images/123e4567-e89b-12d3-a456-426614174000.png",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotURL := ""
			rewriter := newMediaRewriter(func(_ context.Context, rawURL string) ([]byte, error) {
				gotURL = rawURL
				return testPNGBytes(), nil
			})
			got, err := rewriteContent(rewriter, context.Background(), tt.content)
			if err != nil {
				t.Fatalf("rewriteContent() error = %v", err)
			}
			if gotURL != tt.wantURL {
				t.Fatalf("download URL = %q, want %q", gotURL, tt.wantURL)
			}
			if !strings.Contains(got, "![x](data:image/png;base64,") {
				t.Fatalf("rewritten content missing data URI: %q", got)
			}
		})
	}
}

func TestMediaRewriter_MarkdownDownloadFailureReturnsError(t *testing.T) {
	rewriter := newMediaRewriter(countingFailingDownload(new(int)))
	content := "![x](https://assets.grok.com/users/u/generated/id/image.png)"

	got, err := rewriteContent(rewriter, context.Background(), content)
	if err == nil {
		t.Fatal("rewriteContent() expected error")
	}
	if got != "" {
		t.Fatalf("rewriteContent() content = %q, want empty on error", got)
	}
}

func TestMediaRewriter_NonImageDownloadReturnsError(t *testing.T) {
	rewriter := newMediaRewriter(func(context.Context, string) ([]byte, error) {
		return []byte("not an image"), nil
	})
	content := "![x](https://assets.grok.com/users/u/generated/id/image.png)"

	got, err := rewriteContent(rewriter, context.Background(), content)
	if err == nil {
		t.Fatal("rewriteContent() expected error")
	}
	if got != "" {
		t.Fatalf("rewriteContent() content = %q, want empty on error", got)
	}
}

func TestBlockingResponse_DoesNotRewriteReasoningMediaTargets(t *testing.T) {
	calls := 0
	h := newMediaRewriteTestHandler()
	eventCh := blockingEvents(
		flow.StreamEvent{
			ReasoningContent: "![r](https://assets.grok.com/users/u/generated/id/r.png)",
			Downloader:       countingFailingDownload(&calls),
		},
		flow.StreamEvent{Content: "done"},
	)

	resp := runBlockingMediaRewriteTest(t, h, eventCh, &ChatRequest{
		Model:           "grok-3",
		ReasoningEffort: "high",
	})
	if calls != 0 {
		t.Fatalf("reasoning downloader calls = %d, want 0", calls)
	}
	content := resp.Choices[0].Message.Content
	if !strings.Contains(content, "https://assets.grok.com/users/u/generated/id/r.png") {
		t.Fatalf("reasoning content was changed: %q", content)
	}
}

func TestBlockingResponse_DoesNotRewriteToolCallArguments(t *testing.T) {
	calls := 0
	h := newMediaRewriteTestHandler()
	eventCh := blockingEvents(flow.StreamEvent{
		ToolCalls: []flow.ToolCall{{
			ID:   "call_1",
			Type: "function",
			Function: flow.FunctionCall{
				Name:      "lookup",
				Arguments: `{"url":"![x](https://assets.grok.com/users/u/generated/id/x.png)"}`,
			},
		}},
		Downloader: countingFailingDownload(&calls),
	})

	resp := runBlockingMediaRewriteTest(t, h, eventCh, &ChatRequest{Model: "grok-3"})
	if calls != 0 {
		t.Fatalf("tool-call downloader calls = %d, want 0", calls)
	}
	content := resp.Choices[0].Message.Content
	if !strings.Contains(content, "https://assets.grok.com/users/u/generated/id/x.png") {
		t.Fatalf("tool-call content was changed: %q", content)
	}
}

func TestBlockingResponse_MediaRewriteFailureReturnsAPIError(t *testing.T) {
	h := newMediaRewriteTestHandler()
	target := "https://assets.grok.com/users/u/generated/id/image.png"
	eventCh := blockingEvents(flow.StreamEvent{
		Content:    "![x](" + target + ")",
		Downloader: countingFailingDownload(new(int)),
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()

	h.blockingResponse(w, req, eventCh, &ChatRequest{Model: "grok-3"})
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadGateway)
	}
	if strings.Contains(w.Body.String(), target) {
		t.Fatalf("error response leaked upstream URL: %s", w.Body.String())
	}
	var apiErr httpapi.APIError
	if err := json.Unmarshal(w.Body.Bytes(), &apiErr); err != nil {
		t.Fatalf("decode API error: %v", err)
	}
	if apiErr.Error.Code != "media_proxy_failed" {
		t.Fatalf("error code = %q, want media_proxy_failed", apiErr.Error.Code)
	}
}

func TestBlockingResponse_RewritesMediaWhenGenerationDisabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.App.MediaGenerationEnabled = false
	h := &Handler{Cfg: cfg}
	target := "https://assets.grok.com/users/u/generated/id/image.png"
	calls := 0
	eventCh := blockingEvents(flow.StreamEvent{
		Content: "![x](" + target + ")",
		Downloader: func(_ context.Context, rawURL string) ([]byte, error) {
			calls++
			if rawURL != target {
				t.Fatalf("download URL = %q, want %q", rawURL, target)
			}
			return testPNGBytes(), nil
		},
	})

	resp := runBlockingMediaRewriteTest(t, h, eventCh, &ChatRequest{Model: "grok-3"})
	content := resp.Choices[0].Message.Content
	if calls != 1 {
		t.Fatalf("download calls = %d, want 1", calls)
	}
	if strings.Contains(content, target) {
		t.Fatalf("response leaked upstream URL: %q", content)
	}
	if !strings.Contains(content, "![x](data:image/png;base64,") {
		t.Fatalf("response missing rewritten data URI: %q", content)
	}
}

func TestRewriteEventContent_RefreshesDownloader(t *testing.T) {
	target := "https://assets.grok.com/users/u/generated/id/image.png"
	var rewriter *mediaRewriter
	first := flow.StreamEvent{
		Downloader: countingFailingDownload(new(int)),
	}
	if err := rewriteEventContent(context.Background(), &first, &rewriter); err != nil {
		t.Fatalf("first rewriteEventContent() error = %v", err)
	}

	second := flow.StreamEvent{
		Content: "![x](" + target + ")",
		Downloader: func(_ context.Context, rawURL string) ([]byte, error) {
			if rawURL != target {
				t.Fatalf("download URL = %q, want %q", rawURL, target)
			}
			return testPNGBytes(), nil
		},
	}
	if err := rewriteEventContent(context.Background(), &second, &rewriter); err != nil {
		t.Fatalf("second rewriteEventContent() error = %v", err)
	}
	if strings.Contains(second.Content, target) {
		t.Fatalf("second content leaked upstream URL: %q", second.Content)
	}
}

func TestRenderImagesForChatRejectsUpstreamURL(t *testing.T) {
	h := &Handler{}
	target := "https://assets.grok.com/users/u/generated/id/image.png"

	content, err := h.renderImagesForChat(&flow.ImageResponse{
		Data: []flow.ImageData{{URL: target}},
	})
	if err == nil {
		t.Fatal("renderImagesForChat() expected error")
	}
	if content != "" {
		t.Fatalf("renderImagesForChat() content = %q, want empty on error", content)
	}
}
