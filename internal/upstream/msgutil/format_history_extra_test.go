package msgutil

import (
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatToolHistory_ToolCallBlockVariants(t *testing.T) {
	tests := []struct {
		name      string
		toolCall  upstream.ToolCall
		wantParts []string
	}{
		{
			name: "empty tool name becomes unknown_tool",
			toolCall: upstream.ToolCall{
				ID:       "call_1",
				Type:     "function",
				Function: upstream.FunctionCall{Arguments: `{"a":1}`},
			},
			wantParts: []string{`<tool_call>{"name":"unknown_tool","arguments":{"a":1}}</tool_call>`},
		},
		{
			name: "empty arguments become empty object",
			toolCall: upstream.ToolCall{
				ID:       "call_2",
				Type:     "function",
				Function: upstream.FunctionCall{Name: "search"},
			},
			wantParts: []string{`<tool_call>{"name":"search","arguments":{}}</tool_call>`},
		},
		{
			name: "whitespace arguments keep raw json",
			toolCall: upstream.ToolCall{
				ID:   "call_3",
				Type: "function",
				Function: upstream.FunctionCall{
					Name:      "search",
					Arguments: `  {"a": 1}  `,
				},
			},
			wantParts: []string{`<tool_call>{"name":"search","arguments":{"a": 1}}</tool_call>`},
		},
		{
			name: "numeric arguments stay raw",
			toolCall: upstream.ToolCall{
				ID:   "call_4",
				Type: "function",
				Function: upstream.FunctionCall{
					Name:      "lookup",
					Arguments: "42",
				},
			},
			wantParts: []string{`<tool_call>{"name":"lookup","arguments":42}</tool_call>`},
		},
		{
			name: "json string arguments stay raw",
			toolCall: upstream.ToolCall{
				ID:   "call_5",
				Type: "function",
				Function: upstream.FunctionCall{
					Name:      "echo",
					Arguments: `"plain"`,
				},
			},
			wantParts: []string{`<tool_call>{"name":"echo","arguments":"plain"}</tool_call>`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatToolHistory([]upstream.Message{{
				Role:      "assistant",
				ToolCalls: []upstream.ToolCall{tt.toolCall},
			}})
			require.Len(t, result, 1)
			assert.Equal(t, "assistant", result[0].Role)
			assert.Empty(t, result[0].ToolCalls)
			content, ok := result[0].Content.(string)
			require.True(t, ok, "content should be string, got %T", result[0].Content)
			for _, part := range tt.wantParts {
				assert.Contains(t, content, part)
			}
		})
	}
}

func TestFormatToolHistory_AssistantEmptyContentDropsTextPart(t *testing.T) {
	result := FormatToolHistory([]upstream.Message{{
		Role: "assistant",
		ToolCalls: []upstream.ToolCall{{
			ID:       "call_1",
			Type:     "function",
			Function: upstream.FunctionCall{Name: "search", Arguments: `{}`},
		}},
	}})
	require.Len(t, result, 1)
	content, ok := result[0].Content.(string)
	require.True(t, ok)
	assert.True(t, strings.HasPrefix(content, "<tool_call>"),
		"content should start with tool call block, got %q", content)
}

func TestFormatToolHistory_ToolRoleVariants(t *testing.T) {
	tests := []struct {
		name      string
		msg       upstream.Message
		wantRole  string
		wantParts []string
	}{
		{
			name: "map content without content key serializes whole map",
			msg: upstream.Message{
				Role: "tool",
				Content: map[string]any{
					"name":         "search",
					"tool_call_id": "call_1",
				},
			},
			wantRole:  "user",
			wantParts: []string{"tool (search, call_1): ", `"name":"search"`},
		},
		{
			name: "explicit name field wins over map",
			msg: upstream.Message{
				Role: "tool",
				Name: "outer_name",
				Content: map[string]any{
					"name":         "inner_name",
					"tool_call_id": "call_2",
					"content":      "result",
				},
			},
			wantRole:  "user",
			wantParts: []string{"tool (outer_name, call_2): result"},
		},
		{
			name: "nil content formats as empty",
			msg: upstream.Message{
				Role:       "tool",
				Name:       "search",
				ToolCallID: "call_3",
			},
			wantRole:  "user",
			wantParts: []string{"tool (search, call_3): "},
		},
		{
			name: "missing tool call id defaults",
			msg: upstream.Message{
				Role:    "tool",
				Content: "result",
			},
			wantRole:  "user",
			wantParts: []string{"tool (unknown, unknown_call): result"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatToolHistory([]upstream.Message{tt.msg})
			require.Len(t, result, 1)
			assert.Equal(t, tt.wantRole, result[0].Role)
			content, ok := result[0].Content.(string)
			require.True(t, ok, "content should be string, got %T", result[0].Content)
			for _, part := range tt.wantParts {
				assert.Contains(t, content, part)
			}
		})
	}
}

func TestFormatToolHistory_EmptyInput(t *testing.T) {
	result := FormatToolHistory(nil)
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestFormatToolMessageContent_Defaults(t *testing.T) {
	tests := []struct {
		name       string
		content    string
		toolName   string
		toolCallID string
		want       string
	}{
		{"all set", "body", "search", "call_1", "tool (search, call_1): body"},
		{"missing name", "body", "", "call_1", "tool (unknown, call_1): body"},
		{"missing id", "body", "search", "", "tool (search, unknown_call): body"},
		{"missing both", "body", "", "", "tool (unknown, unknown_call): body"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, formatToolMessageContent(tt.content, tt.toolName, tt.toolCallID))
		})
	}
}
