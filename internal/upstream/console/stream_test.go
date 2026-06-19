package console

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream"
)

func TestParseStream_Normal(t *testing.T) {
	data, err := os.ReadFile("test/data/normal_stream.txt")
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}

	c := New("", nil, Options{})
	ch := make(chan upstream.StreamEvent, 16)
	if err := c.parseStream(context.Background(), strings.NewReader(string(data)), ch); err != nil {
		t.Fatalf("parseStream: %v", err)
	}
	close(ch)

	var content string
	var usage *upstream.Usage
	for ev := range ch {
		if ev.Error != nil {
			t.Fatalf("stream error: %v", ev.Error)
		}
		content += ev.Content
		if ev.Usage != nil {
			usage = ev.Usage
		}
	}
	if content != "Hi" {
		t.Errorf("content = %q, want Hi", content)
	}
	if usage == nil {
		t.Fatal("usage is nil")
	}
	if usage.TotalTokens != 4 {
		t.Errorf("usage.TotalTokens = %d, want 4", usage.TotalTokens)
	}
}

func TestParseConsoleEvent_ContentReasoningUsageAndError(t *testing.T) {
	content := parseConsoleEvent("response.output_text.delta", consoleJSON(t, map[string]any{"delta": "hello"}))
	if content.Content != "hello" {
		t.Fatalf("Content = %q, want hello", content.Content)
	}

	reasoning := parseConsoleEvent("response.reasoning_summary_text.delta", consoleJSON(t, map[string]any{"delta": "thinking"}))
	if reasoning.ReasoningContent != "thinking" || !reasoning.IsThinking {
		t.Fatalf("reasoning = %#v, want thinking content", reasoning)
	}

	usage := parseConsoleEvent("response.completed", consoleJSON(t, map[string]any{
		"response": map[string]any{"usage": map[string]any{"input_tokens": 10, "output_tokens": 4}},
	}))
	if usage.Usage == nil || usage.Usage.PromptTokens != 10 || usage.Usage.CompletionTokens != 4 || usage.Usage.TotalTokens != 14 {
		t.Fatalf("usage = %#v, want 10/4/14", usage.Usage)
	}

	failed := parseConsoleEvent("response.failed", consoleJSON(t, map[string]any{"error": map[string]any{"message": "boom"}}))
	if failed.Error == nil || !strings.Contains(failed.Error.Error(), "boom") {
		t.Fatalf("Error = %v, want boom", failed.Error)
	}
}

func TestParseConsoleEvent_SearchSources(t *testing.T) {
	parsed := parseConsoleEvent("response.output_item.done", consoleJSON(t, map[string]any{
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

func consoleJSON(t *testing.T, data any) json.RawMessage {
	t.Helper()
	payload, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal console event: %v", err)
	}
	return payload
}
