package msgutil

import (
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tool(name string) upstream.Tool {
	return upstream.Tool{Type: "function", Function: upstream.Function{Name: name}}
}

func TestBuildToolPrompt_SkipsNonFunctionTools(t *testing.T) {
	tools := []upstream.Tool{
		{Type: "retrieval", Function: upstream.Function{Name: "docs"}},
		tool("search"),
	}
	prompt := BuildToolPrompt(tools, "auto", true)
	require.Contains(t, prompt, "search")
	assert.NotContains(t, prompt, "docs")
}

func TestBuildToolPrompt_SkipsUnmarshalableTool(t *testing.T) {
	tools := []upstream.Tool{
		{
			Type: "function",
			Function: upstream.Function{
				Name:       "bad",
				Parameters: map[string]any{"ch": make(chan int)},
			},
		},
		tool("good"),
	}
	prompt := BuildToolPrompt(tools, "auto", true)
	assert.NotContains(t, prompt, "bad")
	assert.Contains(t, prompt, "good")
}

func TestBuildToolPrompt_ChoiceMapVariants(t *testing.T) {
	tools := []upstream.Tool{tool("search")}
	tests := []struct {
		name       string
		toolChoice any
		wantSub    string
		notSub     string
	}{
		{
			name: "function map with name",
			toolChoice: map[string]any{
				"type":     "function",
				"function": map[string]any{"name": "search"},
			},
			wantSub: `You MUST call the "search" function in your response.`,
		},
		{
			name: "function entry not a map",
			toolChoice: map[string]any{
				"function": "search",
			},
			notSub: "You MUST call",
		},
		{
			name: "function name not a string",
			toolChoice: map[string]any{
				"function": map[string]any{"name": 42},
			},
			notSub: "You MUST call",
		},
		{
			name:       "missing function key",
			toolChoice: map[string]any{"type": "function"},
			notSub:     "You MUST call",
		},
		{
			name:       "non-string non-map choice",
			toolChoice: 42,
			notSub:     "You MUST call",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := BuildToolPrompt(tools, tt.toolChoice, true)
			require.NotEmpty(t, prompt)
			if tt.wantSub != "" {
				assert.Contains(t, prompt, tt.wantSub)
			}
			if tt.notSub != "" {
				assert.NotContains(t, prompt, tt.notSub)
			}
		})
	}
}

func TestBuildToolPrompt_RequiredRestrictsParallelCalls(t *testing.T) {
	prompt := BuildToolPrompt([]upstream.Tool{tool("search")}, "required", false)
	assert.True(t, strings.Contains(prompt, "You may only make one tool call at a time."))
	assert.True(t, strings.Contains(prompt, "You MUST call at least one tool in your response."))
}

func TestBuildToolPrompt_HeaderAndFormat(t *testing.T) {
	prompt := BuildToolPrompt([]upstream.Tool{tool("search")}, "auto", true)
	assert.True(t, strings.HasPrefix(prompt, "You are a function calling AI assistant."))
	assert.Contains(t, prompt, `<tool_call>
{"name": "<function_name>", "arguments": {"<arg_name>": <value>}}
</tool_call>`)
	assert.Contains(t, prompt, "<tools>\n")
	assert.True(t, strings.HasSuffix(strings.TrimRight(prompt, "\n"), "</tools>"))
}
