package grok

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func decodeGrokPayload(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	return payload
}

func TestBuildBody_NilRequest(t *testing.T) {
	g := New("", nil, Options{})
	_, err := g.buildBody(nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat request is required")
}

func TestBuildBody_SamplingOverridesAndFlags(t *testing.T) {
	temp := 0.7
	topP := 0.9
	g := New("", nil, Options{})
	body, err := g.buildBody(&upstream.ChatRequest{
		Messages:          []upstream.Message{{Role: "user", Content: "hi"}},
		UpstreamMode:      "model",
		Temperature:       &temp,
		TopP:              &topP,
		ReasoningEffort:   "high",
		CustomInstruction: "be terse",
		DeepSearch:        "deep",
		Temporary:         true,
		DisableMemory:     true,
	}, []string{"FID-1"})
	require.NoError(t, err)

	payload := decodeGrokPayload(t, body)
	assert.Equal(t, "model", payload["modeId"])
	assert.Equal(t, true, payload["temporary"])
	assert.Equal(t, true, payload["disableMemory"])
	assert.Equal(t, "be terse", payload["customPersonality"])
	assert.Equal(t, "deep", payload["deepsearchPreset"])
	atts, ok := payload["fileAttachments"].([]any)
	require.True(t, ok, "fileAttachments=%v", payload["fileAttachments"])
	assert.Equal(t, []any{"FID-1"}, atts)

	meta, ok := payload["responseMetadata"].(map[string]any)
	require.True(t, ok, "responseMetadata=%v", payload["responseMetadata"])
	override, ok := meta["modelConfigOverride"].(map[string]any)
	require.True(t, ok, "modelConfigOverride=%v", meta)
	assert.Equal(t, 0.7, override["temperature"])
	assert.Equal(t, 0.9, override["topP"])
	assert.Equal(t, "high", override["reasoningEffort"])
}

func TestBuildBody_NoOverridesOmitsMeta(t *testing.T) {
	g := New("", nil, Options{})
	body, err := g.buildBody(&upstream.ChatRequest{
		Messages:     []upstream.Message{{Role: "user", Content: "hi"}},
		UpstreamMode: "auto",
	}, nil)
	require.NoError(t, err)

	payload := decodeGrokPayload(t, body)
	meta, ok := payload["responseMetadata"].(map[string]any)
	require.True(t, ok)
	assert.Empty(t, meta)
	_, hasCustom := payload["customPersonality"]
	assert.False(t, hasCustom)
}

func TestBuildMessages_Errors(t *testing.T) {
	g := New("", nil, Options{})

	t.Run("all messages empty", func(t *testing.T) {
		_, err := g.buildBody(&upstream.ChatRequest{
			Messages:     []upstream.Message{{Role: "user", Content: "   "}},
			UpstreamMode: "auto",
		}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "all messages have empty content")
	})

	t.Run("unmarshalable map content", func(t *testing.T) {
		_, err := buildMessages(&upstream.ChatRequest{
			Messages: []upstream.Message{{
				Role:    "user",
				Content: map[string]any{"x": func() {}},
			}},
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported type")
	})
}

func TestBuildMessages_ToolPromptWithoutSystem(t *testing.T) {
	req := &upstream.ChatRequest{
		Messages: []upstream.Message{{Role: "user", Content: "hi"}},
		Tools: []upstream.Tool{{
			Type: "function",
			Function: upstream.Function{
				Name:        "get_weather",
				Description: "Get weather",
				Parameters:  map[string]any{"type": "object"},
			},
		}},
	}
	messages, err := buildMessages(req)
	require.NoError(t, err)
	require.NotEmpty(t, messages)
	assert.Equal(t, "system", messages[0].Role)
	assert.Contains(t, messages[0].Content, "get_weather")
}

func TestMessageContentText(t *testing.T) {
	tests := []struct {
		name    string
		msg     upstream.Message
		want    string
		wantErr bool
	}{
		{
			name: "plain string",
			msg:  upstream.Message{Role: "user", Content: "hello"},
			want: "hello",
		},
		{
			name: "structured map with content key",
			msg:  upstream.Message{Role: "user", Content: map[string]any{"content": "inner text"}},
			want: "inner text",
		},
		{
			name: "structured map without content key falls back to json",
			msg:  upstream.Message{Role: "user", Content: map[string]any{"type": "custom"}},
			want: `{"type":"custom"}`,
		},
		{
			name:    "marshal failure surfaces error",
			msg:     upstream.Message{Role: "user", Content: map[string]any{"x": func() {}}},
			wantErr: true,
		},
		{
			name: "nil content",
			msg:  upstream.Message{Role: "user", Content: nil},
			want: "",
		},
		{
			name: "scalar content",
			msg:  upstream.Message{Role: "user", Content: 42},
			want: "42",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := messageContentText(tt.msg)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFlattenMessages(t *testing.T) {
	tests := []struct {
		name     string
		messages []upstream.Message
		want     string
	}{
		{
			name:     "empty",
			messages: nil,
			want:     "",
		},
		{
			name:     "single message",
			messages: []upstream.Message{{Role: "user", Content: "only"}},
			want:     "only",
		},
		{
			name: "last user message unprefixed",
			messages: []upstream.Message{
				{Role: "system", Content: "sys"},
				{Role: "assistant", Content: "prev answer"},
				{Role: "user", Content: "question"},
			},
			want: "system: sys\n\nassistant: prev answer\n\nquestion",
		},
		{
			name: "no user message keeps prefixes",
			messages: []upstream.Message{
				{Role: "system", Content: "sys"},
				{Role: "assistant", Content: "ans"},
			},
			want: "system: sys\n\nassistant: ans",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, flattenMessages(tt.messages))
		})
	}
}

func TestAllMessagesEmpty(t *testing.T) {
	tests := []struct {
		name     string
		messages []upstream.Message
		want     bool
	}{
		{name: "nil", messages: nil, want: true},
		{name: "whitespace only", messages: []upstream.Message{{Role: "user", Content: " \t\n"}}, want: true},
		{name: "has content", messages: []upstream.Message{{Role: "user", Content: " x "}}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, allMessagesEmpty(tt.messages))
		})
	}
}

func TestBuildBody_ToolResultHistory(t *testing.T) {
	g := New("", nil, Options{})
	body, err := g.buildBody(&upstream.ChatRequest{
		Messages: []upstream.Message{
			{Role: "assistant", Content: "calling", ToolCalls: []upstream.ToolCall{{
				ID:       "c1",
				Type:     "function",
				Function: upstream.FunctionCall{Name: "get_weather", Arguments: "{\"city\":\"sf\"}"},
			}}},
			{Role: "tool", ToolCallID: "c1", Name: "get_weather", Content: "sunny"},
			{Role: "user", Content: "thanks"},
		},
		UpstreamMode: "auto",
	}, nil)
	require.NoError(t, err)

	payload := decodeGrokPayload(t, body)
	message, ok := payload["message"].(string)
	require.True(t, ok, "message=%T", payload["message"])
	for _, want := range []string{"get_weather", "tool (get_weather, c1): sunny", "thanks"} {
		assert.True(t, strings.Contains(message, want), "message %q missing %q", message, want)
	}
}
