package msgutil

import "testing"

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
			got := ContentToString(tt.input)
			if got != tt.want {
				t.Errorf("ContentToString(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatStructuredMessage_ToolPreservesCallID(t *testing.T) {
	got, ok := FormatStructuredMessage("tool", map[string]any{
		"content":      "result",
		"name":         "search",
		"tool_call_id": "call_123",
	})
	if !ok {
		t.Fatal("expected structured message to be formatted")
	}
	want := "tool (search, call_123): result"
	if got != want {
		t.Fatalf("FormatStructuredMessage() = %q, want %q", got, want)
	}
}
