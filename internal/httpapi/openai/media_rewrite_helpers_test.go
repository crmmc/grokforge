package openai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/flow"
)

func countingFailingDownload(calls *int) flow.DownloadFunc {
	return func(_ context.Context, _ string) ([]byte, error) {
		*calls = *calls + 1
		return nil, errMediaRewriteDownload
	}
}

func testPNGBytes() []byte {
	return []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
}

func newMediaRewriteTestHandler() *Handler {
	cfg := config.DefaultConfig()
	cfg.App.MediaGenerationEnabled = true
	cfg.App.Thinking = true
	return &Handler{Cfg: cfg}
}

func blockingEvents(events ...flow.StreamEvent) <-chan flow.StreamEvent {
	ch := make(chan flow.StreamEvent, len(events))
	for _, event := range events {
		ch <- event
	}
	close(ch)
	return ch
}

func runBlockingMediaRewriteTest(
	t *testing.T,
	h *Handler,
	eventCh <-chan flow.StreamEvent,
	req *ChatRequest,
) chatCompletionResponse {
	t.Helper()
	httpReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	h.blockingResponse(w, httpReq, eventCh, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var resp chatCompletionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp
}
