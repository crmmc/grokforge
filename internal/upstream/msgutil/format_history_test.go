package msgutil

import (
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream"
)

func TestFormatToolHistory_NoToolMessages(t *testing.T) {
	messages := []upstream.Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
	}
	result := FormatToolHistory(messages)
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	for i, m := range result {
		if m.Role != messages[i].Role {
			t.Errorf("message %d: role = %q, want %q", i, m.Role, messages[i].Role)
		}
	}
}

func TestFormatToolHistory_AssistantWithToolCalls(t *testing.T) {
	messages := []upstream.Message{
		{Role: "user", Content: "What's the weather?"},
		{
			Role:    "assistant",
			Content: "Let me check.",
			ToolCalls: []upstream.ToolCall{
				{
					ID:   "call_abc123",
					Type: "function",
					Function: upstream.FunctionCall{
						Name:      "get_weather",
						Arguments: `{"location":"Tokyo"}`,
					},
				},
			},
		},
	}

	result := FormatToolHistory(messages)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}

	assistantContent, ok := result[1].Content.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", result[1].Content)
	}
	if !strings.Contains(assistantContent, "Let me check.") {
		t.Error("should preserve original content")
	}
	if !strings.Contains(assistantContent, `<tool_call>{"name":"get_weather","arguments":{"location":"Tokyo"}}</tool_call>`) {
		t.Errorf("should contain tool_call block, got: %s", assistantContent)
	}
	if len(result[1].ToolCalls) != 0 {
		t.Error("tool_calls should be cleared after formatting")
	}
}

func TestFormatToolHistory_ToolRole(t *testing.T) {
	messages := []upstream.Message{
		{
			Role:       "tool",
			Content:    `{"temp": 22}`,
			Name:       "get_weather",
			ToolCallID: "call_abc123",
		},
	}

	result := FormatToolHistory(messages)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	if result[0].Role != "user" {
		t.Errorf("role = %q, want user", result[0].Role)
	}
	content, ok := result[0].Content.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", result[0].Content)
	}
	want := `tool (get_weather, call_abc123): {"temp": 22}`
	if content != want {
		t.Errorf("content = %q, want %q", content, want)
	}
}

func TestFormatToolHistory_ToolRoleUnknownName(t *testing.T) {
	messages := []upstream.Message{
		{
			Role:       "tool",
			Content:    "result",
			ToolCallID: "call_xyz",
		},
	}

	result := FormatToolHistory(messages)
	content := result[0].Content.(string)
	if !strings.HasPrefix(content, "tool (unknown, call_xyz):") {
		t.Errorf("content = %q, want prefix 'tool (unknown, call_xyz):'", content)
	}
}

func TestFormatToolHistory_ToolRoleStructuredFallback(t *testing.T) {
	messages := []upstream.Message{
		{
			Role: "tool",
			Content: map[string]any{
				"name":         "search",
				"tool_call_id": "call_struct",
				"content":      map[string]any{"ok": true},
			},
		},
	}

	result := FormatToolHistory(messages)
	content, ok := result[0].Content.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", result[0].Content)
	}
	if !strings.Contains(content, "tool (search, call_struct):") {
		t.Fatalf("unexpected formatted content: %q", content)
	}
	if !strings.Contains(content, `{"ok":true}`) {
		t.Fatalf("structured tool content should be serialized: %q", content)
	}
}

func TestFormatToolHistory_MultiTurnConversation(t *testing.T) {
	messages := []upstream.Message{
		{Role: "user", Content: "What's the weather in Tokyo?"},
		{
			Role: "assistant",
			ToolCalls: []upstream.ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: upstream.FunctionCall{
						Name:      "get_weather",
						Arguments: `{"location":"Tokyo"}`,
					},
				},
			},
		},
		{
			Role:       "tool",
			Content:    `{"temp": 22, "condition": "sunny"}`,
			Name:       "get_weather",
			ToolCallID: "call_1",
		},
		{Role: "user", Content: "Thanks!"},
	}

	result := FormatToolHistory(messages)
	if len(result) != 4 {
		t.Fatalf("expected 4 messages, got %d", len(result))
	}
	if result[0].Role != "user" {
		t.Error("message 0 should stay user")
	}
	if result[1].Role != "assistant" {
		t.Error("message 1 should stay assistant")
	}
	assistantContent := result[1].Content.(string)
	if !strings.Contains(assistantContent, "<tool_call>") {
		t.Error("assistant message should contain tool_call block")
	}
	if result[2].Role != "user" {
		t.Errorf("message 2 role = %q, want user", result[2].Role)
	}
	if result[3].Content != "Thanks!" {
		t.Error("last message should be unchanged")
	}
}

func TestFormatToolHistory_AssistantToolCall_InvalidArgsFallback(t *testing.T) {
	messages := []upstream.Message{
		{
			Role:    "assistant",
			Content: "calling tool",
			ToolCalls: []upstream.ToolCall{
				{
					ID:   "call_bad_args",
					Type: "function",
					Function: upstream.FunctionCall{
						Name:      "search",
						Arguments: "not-json-args",
					},
				},
			},
		},
	}

	result := FormatToolHistory(messages)
	content, ok := result[0].Content.(string)
	if !ok {
		t.Fatalf("expected string content, got %T", result[0].Content)
	}
	if !strings.Contains(content, `<tool_call>{"name":"search","arguments":"not-json-args"}</tool_call>`) {
		t.Fatalf("invalid args should be kept as JSON string fallback: %q", content)
	}
}
