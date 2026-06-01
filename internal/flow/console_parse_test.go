package flow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/xai"
)

func TestParseConsoleEvent_ContentReasoningUsageAndError(t *testing.T) {
	f := &ChatFlow{}

	content := f.parseConsoleEvent(consoleWrappedTestEvent(t, "response.output_text.delta", map[string]any{"delta": "hello"}))
	if content.Content != "hello" {
		t.Fatalf("Content = %q, want hello", content.Content)
	}

	reasoning := f.parseConsoleEvent(consoleWrappedTestEvent(t, "response.reasoning_summary_text.delta", map[string]any{"delta": "thinking"}))
	if reasoning.ReasoningContent != "thinking" || !reasoning.IsThinking {
		t.Fatalf("reasoning = %#v, want thinking content", reasoning)
	}

	usage := f.parseConsoleEvent(consoleWrappedTestEvent(t, "response.completed", map[string]any{
		"response": map[string]any{"usage": map[string]any{"input_tokens": 10, "output_tokens": 4}},
	}))
	if usage.Usage == nil || usage.Usage.PromptTokens != 10 || usage.Usage.CompletionTokens != 4 || usage.Usage.TotalTokens != 14 {
		t.Fatalf("usage = %#v, want 10/4/14", usage.Usage)
	}

	failed := f.parseConsoleEvent(consoleWrappedTestEvent(t, "response.failed", map[string]any{"error": map[string]any{"message": "boom"}}))
	if failed.Error == nil || !strings.Contains(failed.Error.Error(), "boom") {
		t.Fatalf("Error = %v, want boom", failed.Error)
	}
}

func TestParseConsoleEvent_SearchSources(t *testing.T) {
	f := &ChatFlow{}
	parsed := f.parseConsoleEvent(consoleWrappedTestEvent(t, "response.output_item.done", map[string]any{
		"item": map[string]any{
			"content": []any{
				map[string]any{"url": "https://example.com/a", "title": "A"},
				map[string]any{"url": "https://x.com/user/status/1", "text": "Post"},
			},
		},
	}))
	if len(parsed.SearchSources) != 2 {
		t.Fatalf("SearchSources = %#v, want 2", parsed.SearchSources)
	}
	if parsed.SearchSources[0].Type != "web" || parsed.SearchSources[0].Title != "A" {
		t.Fatalf("first source = %#v, want web A", parsed.SearchSources[0])
	}
	if parsed.SearchSources[1].Type != "x_post" || parsed.SearchSources[1].Title != "Post" {
		t.Fatalf("second source = %#v, want x_post Post", parsed.SearchSources[1])
	}
}

func consoleWrappedTestEvent(t *testing.T, event string, data any) xai.StreamEvent {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"event": event, "data": data})
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return xai.StreamEvent{Data: payload}
}
