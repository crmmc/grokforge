package flow

import (
	"encoding/json"
	"strings"
	"testing"
)

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
	if len(id) != 29 {
		t.Errorf("ID length = %d, want 29 (call_ + 24 hex), got %q", len(id), id)
	}

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
	content := `<tool_call>
{"name": "search", "arguments": {"query": "test"}}
</tool_call>`

	_, calls := ParseToolCalls(content)
	if len(calls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls))
	}
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
