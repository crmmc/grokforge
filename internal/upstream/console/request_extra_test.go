package console

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeConsolePayload(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	return payload
}

func TestBuildBody_NilRequest(t *testing.T) {
	c := New("", nil, Options{})
	_, err := c.buildBody(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat request is nil")
}

func TestBuildBody_EmptyModel(t *testing.T) {
	c := New("", nil, Options{})
	_, err := c.buildBody(&upstream.ChatRequest{
		Messages: []upstream.Message{{Role: "user", Content: "hi"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "console model is required")
}

func TestBuildConsoleBody_Nil(t *testing.T) {
	_, err := buildConsoleBody(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "console request is nil")
}

func TestBuildConsoleBody_PayloadVariants(t *testing.T) {
	temp := 0.4
	topP := 0.95
	maxTokens := 256

	tests := []struct {
		name      string
		req       *ConsoleRequest
		assertion func(t *testing.T, payload map[string]any)
	}{
		{
			name: "minimal",
			req:  &ConsoleRequest{Model: "grok-4.20", Input: []ConsoleInputItem{{Role: "user", Content: []ConsoleContent{{Type: "input_text", Text: "hi"}}}}},
			assertion: func(t *testing.T, payload map[string]any) {
				assert.Equal(t, "grok-4.20", payload["model"])
				assert.Equal(t, true, payload["stream"])
				_, hasInstructions := payload["instructions"]
				assert.False(t, hasInstructions)
				_, hasReasoning := payload["reasoning"]
				assert.False(t, hasReasoning)
			},
		},
		{
			name: "instructions",
			req:  &ConsoleRequest{Model: "grok-4.20", Instructions: "be nice"},
			assertion: func(t *testing.T, payload map[string]any) {
				assert.Equal(t, "be nice", payload["instructions"])
			},
		},
		{
			name: "sampling params",
			req:  &ConsoleRequest{Model: "grok-4.20", Temperature: &temp, TopP: &topP, MaxTokens: &maxTokens},
			assertion: func(t *testing.T, payload map[string]any) {
				assert.Equal(t, 0.4, payload["temperature"])
				assert.Equal(t, 0.95, payload["top_p"])
				assert.InDelta(t, float64(256), payload["max_output_tokens"], 0.0001)
			},
		},
		{
			name: "reasoning effort supported",
			req:  &ConsoleRequest{Model: "grok-4.20", ReasoningEffort: "xhigh", SupportsReasoningEffort: true},
			assertion: func(t *testing.T, payload map[string]any) {
				reasoning, ok := payload["reasoning"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "high", reasoning["effort"])
			},
		},
		{
			name: "web search",
			req:  &ConsoleRequest{Model: "grok-4.20", WebSearch: true},
			assertion: func(t *testing.T, payload map[string]any) {
				tools, ok := payload["tools"].([]any)
				require.True(t, ok)
				require.Len(t, tools, 1)
				tool, ok := tools[0].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "web_search", tool["type"])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, err := buildConsoleBody(tt.req)
			require.NoError(t, err)
			tt.assertion(t, decodeConsolePayload(t, body))
		})
	}
}

func TestBuildConsoleRequest_Errors(t *testing.T) {
	t.Run("nil request", func(t *testing.T) {
		_, err := buildConsoleRequest(nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "chat request is nil")
	})

	t.Run("all messages empty", func(t *testing.T) {
		_, err := buildConsoleRequest(&upstream.ChatRequest{
			Model:    "grok-4.20",
			Messages: []upstream.Message{{Role: "user", Content: ""}},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "all messages have empty content")
	})
}

func TestBuildConsoleRequest_InstructionsOnlyInput(t *testing.T) {
	built, err := buildConsoleRequest(&upstream.ChatRequest{
		Model:    "grok-4.20",
		Messages: []upstream.Message{{Role: "system", Content: "you are helpful"}},
	})
	require.NoError(t, err)
	assert.Empty(t, built.Input)
	assert.Equal(t, "you are helpful", built.Instructions)
}

func TestBuildConsoleRequest_ToolPromptLeadsInstructions(t *testing.T) {
	built, err := buildConsoleRequest(&upstream.ChatRequest{
		Model:    "grok-4.20",
		Messages: []upstream.Message{{Role: "user", Content: "use the tool"}},
		Tools: []upstream.Tool{{
			Type: "function",
			Function: upstream.Function{
				Name:        "get_weather",
				Description: "Get weather",
				Parameters:  map[string]any{"type": "object"},
			},
		}},
	})
	require.NoError(t, err)
	require.NotEmpty(t, built.Instructions)
	assert.True(t, strings.HasPrefix(built.Instructions, "You are a function calling AI assistant"), "instructions=%q", built.Instructions)
	assert.Contains(t, built.Instructions, "get_weather")
	require.Len(t, built.Input, 1)
	assert.Equal(t, "user", built.Input[0].Role)
}

func TestBuildConsoleRequest_TopLevelAndFlagsPropagated(t *testing.T) {
	built, err := buildConsoleRequest(&upstream.ChatRequest{
		Model:    "grok-4.20",
		Messages: []upstream.Message{{Role: "user", Content: "hi"}},
		TopP:     floatPtr(0.9),
	})
	require.NoError(t, err)
	require.NotNil(t, built.TopP)
	assert.InDelta(t, 0.9, *built.TopP, 0.0001)
}

func floatPtr(f float64) *float64 { return &f }

func TestConsoleReasoningEffort(t *testing.T) {
	tests := []struct {
		name      string
		effort    string
		supported bool
		want      string
	}{
		{name: "unsupported", effort: "high", supported: false, want: ""},
		{name: "empty supported", effort: "", supported: true, want: ""},
		{name: "none", effort: "none", supported: true, want: ""},
		{name: "none with spaces", effort: "  NONE  ", supported: true, want: ""},
		{name: "xhigh maps to high", effort: "xhigh", supported: true, want: "high"},
		{name: "uppercase normalized", effort: "  HIGH ", supported: true, want: "high"},
		{name: "low", effort: "low", supported: true, want: "low"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, consoleReasoningEffort(tt.effort, tt.supported))
		})
	}
}

func TestConsoleInstructionText(t *testing.T) {
	tests := []struct {
		name    string
		content any
		want    string
	}{
		{name: "plain string", content: "hello", want: "hello"},
		{name: "map with text type", content: map[string]any{"type": "text", "text": "from text"}, want: "from text"},
		{name: "map with content key", content: map[string]any{"content": "from content"}, want: "from content"},
		{name: "map fallback json", content: map[string]any{"type": "other"}, want: `{"type":"other"}`},
		{
			name: "array of parts",
			content: []any{
				map[string]any{"type": "text", "text": "a"},
				"plain",
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://x/y.png"}},
			},
			want: `a"plain"{"image_url":{"url":"https://x/y.png"},"type":"image_url"}`,
		},
		{name: "empty array", content: []any{}, want: ""},
		{name: "scalar fallback", content: 42, want: "42"},
		{name: "nil fallback", content: nil, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, consoleInstructionText(tt.content))
		})
	}
}

func TestConsoleContentForRole(t *testing.T) {
	tests := []struct {
		name    string
		role    string
		content any
		want    []ConsoleContent
	}{
		{
			name:    "user string",
			role:    "user",
			content: "hello",
			want:    []ConsoleContent{{Type: "input_text", Text: "hello"}},
		},
		{
			name:    "assistant string",
			role:    "assistant",
			content: "answer",
			want:    []ConsoleContent{{Type: "output_text", Text: "answer"}},
		},
		{
			name:    "empty string yields nil",
			role:    "user",
			content: "",
			want:    nil,
		},
		{
			name:    "map text part",
			role:    "user",
			content: map[string]any{"type": "text", "text": "look"},
			want:    []ConsoleContent{{Type: "input_text", Text: "look"}},
		},
		{
			name:    "map image part",
			role:    "user",
			content: map[string]any{"type": "image_url", "image_url": map[string]any{"url": " https://x/y.png "}},
			want:    []ConsoleContent{{Type: "input_image", ImageURL: "https://x/y.png"}},
		},
		{
			name:    "map with content key uses structured format",
			role:    "user",
			content: map[string]any{"content": "inner"},
			want:    []ConsoleContent{{Type: "input_text", Text: "inner"}},
		},
		{
			name:    "map fallback to json",
			role:    "user",
			content: map[string]any{"type": "weird"},
			want:    []ConsoleContent{{Type: "input_text", Text: `{"type":"weird"}`}},
		},
		{
			name:    "assistant image block becomes text fallback",
			role:    "assistant",
			content: map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://x/y.png"}},
			want:    []ConsoleContent{{Type: "output_text", Text: `{"image_url":{"url":"https://x/y.png"},"type":"image_url"}`}},
		},
		{
			name:    "slice of maps",
			role:    "user",
			content: []map[string]any{{"type": "text", "text": "a"}, {"type": "file"}},
			want: []ConsoleContent{
				{Type: "input_text", Text: "a"},
				{Type: "input_text", Text: `{"type":"file"}`},
			},
		},
		{
			name: "mixed any slice",
			role: "user",
			content: []any{
				map[string]any{"type": "text", "text": "a"},
				"plain item",
			},
			want: []ConsoleContent{
				{Type: "input_text", Text: "a"},
				{Type: "input_text", Text: `"plain item"`},
			},
		},
		{
			name:    "scalar content",
			role:    "user",
			content: 7,
			want:    []ConsoleContent{{Type: "input_text", Text: "7"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := consoleContentForRole(tt.role, tt.content)
			if tt.want == nil {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestConsoleContentForRole_ContentBlocks(t *testing.T) {
	tests := []struct {
		name   string
		role   string
		blocks []upstream.ContentBlock
		want   []ConsoleContent
	}{
		{
			name: "text and image for user",
			role: "user",
			blocks: []upstream.ContentBlock{
				{Type: "text", Text: "look"},
				{Type: "image_url", ImageURL: &upstream.ImageURLBlock{URL: "https://x/y.png"}},
			},
			want: []ConsoleContent{
				{Type: "input_text", Text: "look"},
				{Type: "input_image", ImageURL: "https://x/y.png"},
			},
		},
		{
			name:   "assistant text keeps output type",
			role:   "assistant",
			blocks: []upstream.ContentBlock{{Type: "text", Text: "answer"}},
			want:   []ConsoleContent{{Type: "output_text", Text: "answer"}},
		},
		{
			name:   "assistant image block skipped entirely",
			role:   "assistant",
			blocks: []upstream.ContentBlock{{Type: "image_url", ImageURL: &upstream.ImageURLBlock{URL: "https://x/y.png"}}},
			want:   nil,
		},
		{
			name:   "nil image url skipped",
			role:   "user",
			blocks: []upstream.ContentBlock{{Type: "image_url"}},
			want:   nil,
		},
		{
			name:   "unknown block falls back to json",
			role:   "user",
			blocks: []upstream.ContentBlock{{Type: "input_audio"}},
			want:   []ConsoleContent{{Type: "input_text", Text: `{"type":"input_audio"}`}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := consoleContentForRole(tt.role, tt.blocks)
			if tt.want == nil {
				assert.Empty(t, got)
			} else {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestConsoleContentPart(t *testing.T) {
	tests := []struct {
		name      string
		role      string
		block     map[string]any
		want      ConsoleContent
		wantMatch bool
	}{
		{
			name:      "text",
			role:      "user",
			block:     map[string]any{"type": "text", "text": "hi"},
			want:      ConsoleContent{Type: "input_text", Text: "hi"},
			wantMatch: true,
		},
		{
			name:      "image url string for user",
			role:      "user",
			block:     map[string]any{"type": "image_url", "image_url": "https://x/a.png"},
			want:      ConsoleContent{Type: "input_image", ImageURL: "https://x/a.png"},
			wantMatch: true,
		},
		{
			name:      "image url map for user",
			role:      "user",
			block:     map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://x/b.png"}},
			want:      ConsoleContent{Type: "input_image", ImageURL: "https://x/b.png"},
			wantMatch: true,
		},
		{
			name:      "image url missing for user",
			role:      "user",
			block:     map[string]any{"type": "image_url"},
			want:      ConsoleContent{Type: "input_text", Text: `{"type":"image_url"}`},
			wantMatch: true,
		},
		{
			name:      "image url for assistant becomes fallback text",
			role:      "assistant",
			block:     map[string]any{"type": "image_url", "image_url": "https://x/c.png"},
			want:      ConsoleContent{Type: "output_text", Text: `{"image_url":"https://x/c.png","type":"image_url"}`},
			wantMatch: true,
		},
		{
			name:      "file block",
			role:      "user",
			block:     map[string]any{"type": "file", "file_id": "f1"},
			want:      ConsoleContent{Type: "input_text", Text: `{"file_id":"f1","type":"file"}`},
			wantMatch: true,
		},
		{
			name:      "input_file block",
			role:      "user",
			block:     map[string]any{"type": "input_file"},
			want:      ConsoleContent{Type: "input_text", Text: `{"type":"input_file"}`},
			wantMatch: true,
		},
		{
			name:      "input_audio block",
			role:      "user",
			block:     map[string]any{"type": "input_audio"},
			want:      ConsoleContent{Type: "input_text", Text: `{"type":"input_audio"}`},
			wantMatch: true,
		},
		{
			name:      "audio block",
			role:      "user",
			block:     map[string]any{"type": "audio"},
			want:      ConsoleContent{Type: "input_text", Text: `{"type":"audio"}`},
			wantMatch: true,
		},
		{
			name:      "unknown type no match",
			role:      "user",
			block:     map[string]any{"type": "bogus"},
			wantMatch: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := consoleContentPart(tt.role, tt.block)
			assert.Equal(t, tt.wantMatch, ok)
			if ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestConsoleImageURL(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  string
	}{
		{name: "string", value: " https://x/a.png ", want: "https://x/a.png"},
		{name: "map", value: map[string]any{"url": "https://x/b.png"}, want: "https://x/b.png"},
		{name: "map without url", value: map[string]any{}, want: ""},
		{name: "nil", value: nil, want: ""},
		{name: "number", value: 3, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, consoleImageURL(tt.value))
		})
	}
}

func TestConsoleTextContent(t *testing.T) {
	assert.Nil(t, consoleTextContent("input_text", ""))
	assert.Equal(t, []ConsoleContent{{Type: "input_text", Text: "x"}}, consoleTextContent("input_text", "x"))
}

func TestConsoleTextTypeAndInputRole(t *testing.T) {
	assert.Equal(t, "output_text", consoleTextType("assistant"))
	assert.Equal(t, "input_text", consoleTextType("user"))
	assert.Equal(t, "output_text", consoleTextType(" assistant "))
	assert.Equal(t, "assistant", consoleInputRole("assistant"))
	assert.Equal(t, "user", consoleInputRole("developer"))
	assert.Equal(t, "user", consoleInputRole(" user "))
}

func TestConsoleJSONFallback_MarshalError(t *testing.T) {
	got := consoleJSONFallback(make(chan int))
	assert.True(t, strings.HasPrefix(got, "0x"), "got %q", got)
}
