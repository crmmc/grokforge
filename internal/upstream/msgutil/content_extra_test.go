package msgutil

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContentToString_ScalarAndFallback(t *testing.T) {
	ch := make(chan int, 1)
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{"bool", true, "true"},
		{"int", 42, "42"},
		{"float", 1.5, "1.5"},
		{"slice", []any{"a", 1}, `["a",1]`},
		{"nested map", map[string]any{"a": map[string]any{"b": 1}}, `{"a":{"b":1}}`},
		{"pointer struct", &struct{ A string }{A: "x"}, `{"A":"x"}`},
		{"unmarshalable channel", ch, fmt.Sprintf("%v", ch)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ContentToString(tt.input))
		})
	}
}

func TestContentToString_JSONErrorFallsBackToStringOf(t *testing.T) {
	// json.Marshal cannot encode channels, so the fallback must kick in.
	ch := make(chan int, 1)
	_, marshalErr := json.Marshal(ch)
	require.Error(t, marshalErr)

	got := ContentToString(ch)
	assert.Equal(t, fmt.Sprintf("%v", ch), got)
	assert.False(t, strings.HasPrefix(got, "{"))
}

func TestFormatStructuredMessage(t *testing.T) {
	tests := []struct {
		name     string
		role     string
		content  map[string]any
		wantText string
		wantOK   bool
	}{
		{
			name:     "missing content key",
			role:     "user",
			content:  map[string]any{"name": "x"},
			wantText: "",
			wantOK:   false,
		},
		{
			name:     "nil map",
			role:     "user",
			content:  nil,
			wantText: "",
			wantOK:   false,
		},
		{
			name:     "non-tool role passthrough",
			role:     "assistant",
			content:  map[string]any{"content": "hello"},
			wantText: "hello",
			wantOK:   true,
		},
		{
			name:     "tool role with defaults",
			role:     "tool",
			content:  map[string]any{"content": "payload"},
			wantText: "tool (unknown, unknown_call): payload",
			wantOK:   true,
		},
		{
			name: "tool role with explicit metadata",
			role: "tool",
			content: map[string]any{
				"content":      "payload",
				"name":         "search",
				"tool_call_id": "call_1",
			},
			wantText: "tool (search, call_1): payload",
			wantOK:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := FormatStructuredMessage(tt.role, tt.content)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantText, got)
		})
	}
}

func TestStringFromAny(t *testing.T) {
	tests := []struct {
		name  string
		input any
		want  string
	}{
		{"string trimmed", "  padded  ", "padded"},
		{"empty string", "", ""},
		{"nil", nil, ""},
		{"int", 42, ""},
		{"bool", true, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, StringFromAny(tt.input))
		})
	}
}
