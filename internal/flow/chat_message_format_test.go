package flow

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatToolHistory_NoToolMessages(t *testing.T) {
	messages := []Message{
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
	messages := []Message{
		{Role: "user", Content: "What's the weather?"},
		{
			Role:    "assistant",
			Content: "Let me check.",
			ToolCalls: []ToolCall{
				{
					ID:   "call_abc123",
					Type: "function",
					Function: FunctionCall{
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

	// Assistant message should be converted to text with tool_call blocks
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
	// Tool calls should be cleared
	if len(result[1].ToolCalls) != 0 {
		t.Error("tool_calls should be cleared after formatting")
	}
}

func TestFormatToolHistory_ToolRole(t *testing.T) {
	messages := []Message{
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

	// Tool message should become user role
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
	messages := []Message{
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
	messages := []Message{
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
	messages := []Message{
		{Role: "user", Content: "What's the weather in Tokyo?"},
		{
			Role: "assistant",
			ToolCalls: []ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: FunctionCall{
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
	// First message unchanged
	if result[0].Role != "user" {
		t.Error("message 0 should stay user")
	}
	// Assistant with tool_call block
	if result[1].Role != "assistant" {
		t.Error("message 1 should stay assistant")
	}
	assistantContent := result[1].Content.(string)
	if !strings.Contains(assistantContent, "<tool_call>") {
		t.Error("assistant message should contain tool_call block")
	}
	// Tool becomes user
	if result[2].Role != "user" {
		t.Errorf("message 2 role = %q, want user", result[2].Role)
	}
	// Last user message unchanged
	if result[3].Content != "Thanks!" {
		t.Error("last message should be unchanged")
	}
}

func TestContentToString(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{"string", "hello", "hello"},
		{"nil", nil, ""},
		{"map", map[string]any{"key": "val"}, `{"key":"val"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := contentToString(tt.input)
			if got != tt.want {
				t.Errorf("contentToString(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatStructuredMessage_ToolPreservesCallID(t *testing.T) {
	got, ok := formatStructuredMessage("tool", map[string]any{
		"content":      "result",
		"name":         "search",
		"tool_call_id": "call_123",
	})
	if !ok {
		t.Fatal("expected structured message to be formatted")
	}
	want := "tool (search, call_123): result"
	if got != want {
		t.Fatalf("formatStructuredMessage() = %q, want %q", got, want)
	}
}

func TestStripCodeFences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no fences", `{"a":1}`, `{"a":1}`},
		{"json fence", "```json\n{\"a\":1}\n```", `{"a":1}`},
		{"plain fence", "```\n{\"a\":1}\n```", `{"a":1}`},
		{"no end fence", "```json\n{\"a\":1}", `{"a":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripCodeFences(tt.input)
			if got != tt.want {
				t.Errorf("stripCodeFences() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestExtractJSONObject(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"pure json", `{"a":1}`, `{"a":1}`},
		{"with prefix", `result: {"a":1}`, `{"a":1}`},
		{"with suffix", `{"a":1} done`, `{"a":1}`},
		{"no braces", "no json here", "no json here"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractJSONObject(tt.input)
			if got != tt.want {
				t.Errorf("extractJSONObject() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRepairJSON_Enhanced(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantJSON bool
	}{
		{"trailing comma object", `{"a": 1,}`, true},
		{"trailing comma array", `[1, 2,]`, true},
		{"unclosed object", `{"a": 1`, true},
		{"unclosed array", `[1, 2`, true},
		{"valid json", `{"a": 1}`, true},
		{"code fence wrapped", "```json\n{\"a\": 1}\n```", true},
		{"with prefix text", `Here is the result: {"a": 1}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repaired := repairJSON(tt.input)
			var v any
			err := json.Unmarshal([]byte(repaired), &v)
			if tt.wantJSON && err != nil {
				t.Errorf("repairJSON(%q) = %q, still invalid: %v", tt.input, repaired, err)
			}
		})
	}
}

func TestGenToolCallID(t *testing.T) {
	id := genToolCallID()
	if !strings.HasPrefix(id, "call_") {
		t.Errorf("ID should start with 'call_', got %q", id)
	}
	// "call_" = 5 chars + 24 hex chars = 29 total
	if len(id) != 29 {
		t.Errorf("ID length = %d, want 29 (call_ + 24 hex), got %q", len(id), id)
	}

	// Ensure uniqueness
	id2 := genToolCallID()
	if id == id2 {
		t.Error("two calls should generate different IDs")
	}
}

func TestParseToolCalls_WithToolValidation(t *testing.T) {
	content := `<tool_call>
{"name": "get_weather", "arguments": "{\"location\": \"Tokyo\"}"}
</tool_call>
<tool_call>
{"name": "unknown_func", "arguments": "{}"}
</tool_call>`

	tools := []Tool{
		{Type: "function", Function: Function{Name: "get_weather"}},
	}

	_, calls := ParseToolCalls(content, tools)

	if len(calls) != 1 {
		t.Fatalf("expected 1 valid tool call, got %d", len(calls))
	}
	if calls[0].Function.Name != "get_weather" {
		t.Errorf("expected name 'get_weather', got %q", calls[0].Function.Name)
	}
}

func TestParseToolCalls_WithDictArguments(t *testing.T) {
	// Arguments as JSON object (not string-encoded)
	content := `<tool_call>
{"name": "search", "arguments": {"query": "test"}}
</tool_call>`

	_, calls := ParseToolCalls(content)
	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls))
	}
	// Arguments should be serialized as JSON string
	var args map[string]any
	if err := json.Unmarshal([]byte(calls[0].Function.Arguments), &args); err != nil {
		t.Errorf("arguments should be valid JSON: %v", err)
	}
	if args["query"] != "test" {
		t.Errorf("expected query=test, got %v", args["query"])
	}
}

func TestParseToolCalls_CodeFenceWrappedJSON(t *testing.T) {
	content := "<tool_call>\n```json\n{\"name\": \"calc\", \"arguments\": \"{\\\"x\\\": 1}\"}\n```\n</tool_call>"

	_, calls := ParseToolCalls(content)
	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call despite code fences, got %d", len(calls))
	}
	if calls[0].Function.Name != "calc" {
		t.Errorf("expected name 'calc', got %q", calls[0].Function.Name)
	}
}

func TestFormatToolHistory_AssistantToolCall_InvalidArgsFallback(t *testing.T) {
	messages := []Message{
		{
			Role:    "assistant",
			Content: "calling tool",
			ToolCalls: []ToolCall{
				{
					ID:   "call_bad_args",
					Type: "function",
					Function: FunctionCall{
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
