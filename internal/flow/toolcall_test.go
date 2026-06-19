package flow

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseToolCalls_NoToolCalls(t *testing.T) {
	content := "This is just regular text without any tool calls."
	text, calls := ParseToolCalls(content)

	if text != content {
		t.Errorf("expected text %q, got %q", content, text)
	}
	if len(calls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(calls))
	}
}

func TestParseToolCalls_SingleCall(t *testing.T) {
	content := `Let me check the weather.
<tool_call>
{"name": "get_weather", "arguments": "{\"location\": \"Tokyo\"}"}
</tool_call>
Done.`

	text, calls := ParseToolCalls(content)

	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls))
	}
	if calls[0].Function.Name != "get_weather" {
		t.Errorf("expected name get_weather, got %s", calls[0].Function.Name)
	}
	if calls[0].Function.Arguments != `{"location": "Tokyo"}` {
		t.Errorf("unexpected arguments: %s", calls[0].Function.Arguments)
	}
	if calls[0].ID == "" {
		t.Error("tool call should have an ID")
	}
	if calls[0].Type != "function" {
		t.Errorf("expected type function, got %s", calls[0].Type)
	}

	// Text should not contain tool_call block
	if strings.Contains(text, "<tool_call>") {
		t.Error("text should not contain tool_call block")
	}
	if !strings.Contains(text, "Let me check") {
		t.Error("text should contain surrounding content")
	}
}

func TestParseToolCalls_ParallelCalls(t *testing.T) {
	content := `<tool_call>
{"name": "get_weather", "arguments": "{\"location\": \"Tokyo\"}"}
</tool_call>
<tool_call>
{"name": "get_weather", "arguments": "{\"location\": \"London\"}"}
</tool_call>`

	text, calls := ParseToolCalls(content)

	if len(calls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(calls))
	}
	if calls[0].Function.Name != "get_weather" {
		t.Error("first call should be get_weather")
	}
	if calls[1].Function.Name != "get_weather" {
		t.Error("second call should be get_weather")
	}

	// IDs should be unique
	if calls[0].ID == calls[1].ID {
		t.Error("tool call IDs should be unique")
	}

	text = strings.TrimSpace(text)
	if text != "" {
		t.Errorf("expected empty text after extracting all tool calls, got %q", text)
	}
}

func TestParseToolCalls_MalformedJSON_TrailingComma(t *testing.T) {
	content := `<tool_call>
{"name": "search", "arguments": "{\"query\": \"test\",}"}
</tool_call>`

	_, calls := ParseToolCalls(content)

	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call despite trailing comma, got %d", len(calls))
	}
	if calls[0].Function.Name != "search" {
		t.Errorf("expected name search, got %s", calls[0].Function.Name)
	}
}

func TestParseToolCalls_MalformedJSON_UnclosedBracket(t *testing.T) {
	content := `<tool_call>
{"name": "calc", "arguments": "{\"x\": 1"
</tool_call>`

	_, calls := ParseToolCalls(content)

	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call despite unclosed bracket, got %d", len(calls))
	}
}

func TestParseToolCalls_InvalidOnly_PreservesOriginalContent(t *testing.T) {
	content := "before\n<tool_call>{\"name\":\"unknown\",\"arguments\":{}}</tool_call>\nafter"
	tools := []Tool{
		{Type: "function", Function: Function{Name: "get_weather"}},
	}

	text, calls := ParseToolCalls(content, tools)
	if len(calls) != 0 {
		t.Fatalf("expected 0 tool calls, got %d", len(calls))
	}
	if text != content {
		t.Fatalf("content should stay unchanged when no valid calls, got %q", text)
	}
}

func TestParseToolCalls_MixedValidAndInvalid_KeepsInvalidBlock(t *testing.T) {
	content := `before
<tool_call>{"name":"get_weather","arguments":{"location":"Tokyo"}}</tool_call>
<tool_call>{"name":"unknown","arguments":{"x":1}}</tool_call>
after`
	tools := []Tool{
		{Type: "function", Function: Function{Name: "get_weather"}},
	}

	text, calls := ParseToolCalls(content, tools)
	if len(calls) != 1 {
		t.Fatalf("expected 1 valid tool call, got %d", len(calls))
	}
	if strings.Contains(text, `"name":"get_weather"`) {
		t.Fatalf("valid tool call block should be removed from text: %q", text)
	}
	if !strings.Contains(text, `"name":"unknown"`) {
		t.Fatalf("invalid tool call block should be preserved in text: %q", text)
	}
}

func TestRepairJSON(t *testing.T) {
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
