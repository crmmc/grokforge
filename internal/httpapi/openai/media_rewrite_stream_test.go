package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/flow"
)

func TestStreamResponse_RewritesMediaInCurrentChunk(t *testing.T) {
	h := newMediaRewriteTestHandler()
	target := "https://assets.grok.com/users/u/generated/id/image.png"
	eventCh := blockingEvents(flow.StreamEvent{
		Content: "![x](" + target + ")",
		Downloader: func(_ context.Context, rawURL string) ([]byte, error) {
			if rawURL != target {
				t.Fatalf("download URL = %q, want %q", rawURL, target)
			}
			return testPNGBytes(), nil
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	stream := true

	h.streamResponse(w, req, eventCh, &ChatRequest{Model: "grok-3", Stream: &stream})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), target) {
		t.Fatalf("stream leaked upstream URL: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "data:image/png;base64,") {
		t.Fatalf("stream missing rewritten image: %s", w.Body.String())
	}
}

func TestStreamResponse_DoesNotRewriteReasoningMediaTargets(t *testing.T) {
	calls := 0
	h := newMediaRewriteTestHandler()
	target := "https://assets.grok.com/users/u/generated/id/r.png"
	eventCh := blockingEvents(flow.StreamEvent{
		ReasoningContent: "![r](" + target + ")",
		Downloader:       countingFailingDownload(&calls),
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	stream := true

	h.streamResponse(w, req, eventCh, &ChatRequest{
		Model:           "grok-3",
		Stream:          &stream,
		ReasoningEffort: "high",
	})

	if calls != 0 {
		t.Fatalf("reasoning downloader calls = %d, want 0", calls)
	}
	if !strings.Contains(w.Body.String(), target) {
		t.Fatalf("reasoning target was changed: %s", w.Body.String())
	}
}
