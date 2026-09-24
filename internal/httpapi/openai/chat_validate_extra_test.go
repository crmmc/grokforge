package openai

import (
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/flow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func baseValidMessages() []ChatMessage {
	return []ChatMessage{{Role: "user", Content: "hello"}}
}

func TestNormalizeChatRequest_ValidationErrors(t *testing.T) {
	tests := []struct {
		name     string
		req      *ChatRequest
		wantCode string
	}{
		{
			name:     "nil request",
			req:      nil,
			wantCode: "invalid_request",
		},
		{
			name: "missing model",
			req: &ChatRequest{
				Messages: baseValidMessages(),
			},
			wantCode: "missing_model",
		},
		{
			name: "empty messages",
			req: &ChatRequest{
				Model:    "grok-3",
				Messages: nil,
			},
			wantCode: "invalid_messages",
		},
		{
			name: "invalid reasoning effort",
			req: &ChatRequest{
				Model:           "grok-3",
				Messages:        baseValidMessages(),
				ReasoningEffort: "ultra",
			},
			wantCode: "invalid_reasoning_effort",
		},
		{
			name: "temperature too high",
			req: &ChatRequest{
				Model:       "grok-3",
				Messages:    baseValidMessages(),
				Temperature: float64Ptr(2.5),
			},
			wantCode: "invalid_temperature",
		},
		{
			name: "temperature negative",
			req: &ChatRequest{
				Model:       "grok-3",
				Messages:    baseValidMessages(),
				Temperature: float64Ptr(-0.1),
			},
			wantCode: "invalid_temperature",
		},
		{
			name: "top_p too high",
			req: &ChatRequest{
				Model:    "grok-3",
				Messages: baseValidMessages(),
				TopP:     float64Ptr(1.5),
			},
			wantCode: "invalid_top_p",
		},
		{
			name: "top_p negative",
			req: &ChatRequest{
				Model:    "grok-3",
				Messages: baseValidMessages(),
				TopP:     float64Ptr(-0.5),
			},
			wantCode: "invalid_top_p",
		},
		{
			name: "tool missing function name",
			req: &ChatRequest{
				Model:    "grok-3",
				Messages: baseValidMessages(),
				Tools:    []flow.Tool{{Type: "function", Function: flow.Function{Name: "  "}}},
			},
			wantCode: "missing_function_name",
		},
		{
			name: "tool choice non string non map",
			req: &ChatRequest{
				Model:      "grok-3",
				Messages:   baseValidMessages(),
				ToolChoice: 42,
			},
			wantCode: "invalid_tool_choice",
		},
		{
			name: "tool choice map without function name",
			req: &ChatRequest{
				Model:      "grok-3",
				Messages:   baseValidMessages(),
				ToolChoice: map[string]any{"type": "function", "function": map[string]any{"name": ""}},
			},
			wantCode: "invalid_tool_choice",
		},
		{
			name: "message with invalid role",
			req: &ChatRequest{
				Model:    "grok-3",
				Messages: []ChatMessage{{Role: "robot", Content: "hi"}},
			},
			wantCode: "invalid_role",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := normalizeChatRequest(tc.req, nil)
			require.NotNil(t, err)
			assert.Equal(t, tc.wantCode, err.code)
			assert.Equal(t, 400, err.status)
			assert.Equal(t, errTypeInvalidRequest, err.errType)
			assert.NotEmpty(t, err.message)
		})
	}
}

func TestNormalizeChatRequest_Success(t *testing.T) {
	tests := []struct {
		name  string
		req   *ChatRequest
		cfg   *config.Config
		check func(t *testing.T, out *ChatRequest)
	}{
		{
			name: "stream default from config",
			req:  &ChatRequest{Model: "grok-3", Messages: baseValidMessages()},
			cfg:  func() *config.Config { c := config.DefaultConfig(); c.App.Stream = true; return c }(),
			check: func(t *testing.T, out *ChatRequest) {
				require.NotNil(t, out.Stream)
				assert.True(t, *out.Stream)
			},
		},
		{
			name: "reasoning effort normalized",
			req:  &ChatRequest{Model: "grok-3", Messages: baseValidMessages(), ReasoningEffort: "  HIGH "},
			check: func(t *testing.T, out *ChatRequest) {
				assert.Equal(t, "high", out.ReasoningEffort)
			},
		},
		{
			name: "tool choice string normalized",
			req:  &ChatRequest{Model: "grok-3", Messages: baseValidMessages(), ToolChoice: "  Required "},
			check: func(t *testing.T, out *ChatRequest) {
				assert.Equal(t, "required", out.ToolChoice)
			},
		},
		{
			name: "tool choice function map valid",
			req: &ChatRequest{
				Model:      "grok-3",
				Messages:   baseValidMessages(),
				ToolChoice: map[string]any{"type": "function", "function": map[string]any{"name": "lookup"}},
			},
			check: func(t *testing.T, out *ChatRequest) {
				assert.NotNil(t, out.ToolChoice)
			},
		},
		{
			name: "temperature boundary values kept",
			req:  &ChatRequest{Model: "grok-3", Messages: baseValidMessages(), Temperature: float64Ptr(0)},
			check: func(t *testing.T, out *ChatRequest) {
				require.NotNil(t, out.Temperature)
				assert.Equal(t, 0.0, *out.Temperature)
			},
		},
		{
			name: "top_p default applied",
			req:  &ChatRequest{Model: "grok-3", Messages: baseValidMessages()},
			check: func(t *testing.T, out *ChatRequest) {
				require.NotNil(t, out.TopP)
				assert.Equal(t, defaultChatTopP, *out.TopP)
			},
		},
		{
			name: "parallel tool calls default",
			req:  &ChatRequest{Model: "grok-3", Messages: baseValidMessages()},
			check: func(t *testing.T, out *ChatRequest) {
				require.NotNil(t, out.ParallelToolCalls)
				assert.True(t, *out.ParallelToolCalls)
			},
		},
		{
			name: "valid tools pass through",
			req: &ChatRequest{
				Model:    "grok-3",
				Messages: baseValidMessages(),
				Tools:    []flow.Tool{{Type: " function ", Function: flow.Function{Name: " lookup "}}},
			},
			check: func(t *testing.T, out *ChatRequest) {
				require.Len(t, out.Tools, 1)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := normalizeChatRequest(tc.req, tc.cfg)
			require.Nil(t, err)
			require.NotNil(t, out)
			if tc.check != nil {
				tc.check(t, out)
			}
		})
	}
}

func float64Ptr(v float64) *float64 { return &v }

func TestValidateMessage_ContentVariants(t *testing.T) {
	tests := []struct {
		name     string
		msg      ChatMessage
		wantCode string
	}{
		{
			name:     "empty role",
			msg:      ChatMessage{Role: "  ", Content: "hi"},
			wantCode: "invalid_role",
		},
		{
			name:     "tool message without tool_call_id",
			msg:      ChatMessage{Role: "tool", Content: "result"},
			wantCode: "missing_tool_call_id",
		},
		{
			name:     "tool message with tool_call_id ok",
			msg:      ChatMessage{Role: "tool", Content: "result", ToolCallID: "t1"},
			wantCode: "",
		},
		{
			name:     "assistant with tool calls and nil content ok",
			msg:      ChatMessage{Role: "assistant", Content: nil, ToolCalls: []flow.ToolCall{{}}},
			wantCode: "",
		},
		{
			name:     "nil content rejected",
			msg:      ChatMessage{Role: "user", Content: nil},
			wantCode: "empty_content",
		},
		{
			name:     "empty string content rejected",
			msg:      ChatMessage{Role: "user", Content: "   "},
			wantCode: "empty_content",
		},
		{
			name:     "empty array content rejected",
			msg:      ChatMessage{Role: "user", Content: []any{}},
			wantCode: "empty_content",
		},
		{
			name:     "non-object block rejected",
			msg:      ChatMessage{Role: "user", Content: []any{"not-an-object"}},
			wantCode: "invalid_block",
		},
		{
			name:     "unsupported content type rejected",
			msg:      ChatMessage{Role: "user", Content: 12345},
			wantCode: "invalid_content",
		},
		{
			name: "string content ok",
			msg:  ChatMessage{Role: "user", Content: "hi"},
		},
		{
			name: "array of text blocks ok",
			msg: ChatMessage{Role: "assistant", Content: []any{
				map[string]any{"type": "text", "text": "part 1"},
				map[string]any{"type": "text", "text": "part 2"},
			}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateMessage(tc.msg, 0)
			if tc.wantCode == "" {
				assert.Nil(t, err)
				return
			}
			require.NotNil(t, err)
			assert.Equal(t, tc.wantCode, err.code)
		})
	}
}

func TestValidateSingleContentObject(t *testing.T) {
	tests := []struct {
		name     string
		content  map[string]any
		wantCode string
	}{
		{
			name:     "missing type",
			content:  map[string]any{"text": "hi"},
			wantCode: "invalid_content_type",
		},
		{
			name:     "non text type",
			content:  map[string]any{"type": "image_url"},
			wantCode: "invalid_content_type",
		},
		{
			name:     "empty text",
			content:  map[string]any{"type": "text", "text": "  "},
			wantCode: "empty_content",
		},
		{
			name:     "text missing",
			content:  map[string]any{"type": "text"},
			wantCode: "empty_content",
		},
		{
			name:     "valid",
			content:  map[string]any{"type": "text", "text": "hi"},
			wantCode: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSingleContentObject(tc.content, "user")
			if tc.wantCode == "" {
				assert.Nil(t, err)
				return
			}
			require.NotNil(t, err)
			assert.Equal(t, tc.wantCode, err.code)
		})
	}
}

func TestValidateContentBlock(t *testing.T) {
	tests := []struct {
		name     string
		block    map[string]any
		role     string
		wantCode string
	}{
		{
			name:     "empty block",
			block:    map[string]any{},
			role:     "user",
			wantCode: "empty_block",
		},
		{
			name:     "missing type field",
			block:    map[string]any{"text": "hi"},
			role:     "user",
			wantCode: "missing_type",
		},
		{
			name:     "empty type value",
			block:    map[string]any{"type": "   "},
			role:     "user",
			wantCode: "empty_type",
		},
		{
			name:     "invalid user block type",
			block:    map[string]any{"type": "video_url"},
			role:     "user",
			wantCode: "invalid_type",
		},
		{
			name:     "non text block for assistant",
			block:    map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/a.png"}},
			role:     "assistant",
			wantCode: "invalid_type",
		},
		{
			name:     "text block with empty text",
			block:    map[string]any{"type": "text", "text": "  "},
			role:     "user",
			wantCode: "empty_text",
		},
		{
			name:     "image_url without media object",
			block:    map[string]any{"type": "image_url"},
			role:     "user",
			wantCode: "missing_url",
		},
		{
			name:     "image_url with empty url",
			block:    map[string]any{"type": "image_url", "image_url": map[string]any{"url": ""}},
			role:     "user",
			wantCode: "invalid_media",
		},
		{
			name:     "input_audio without media object",
			block:    map[string]any{"type": "input_audio"},
			role:     "user",
			wantCode: "missing_audio",
		},
		{
			name:     "input_audio with bad data",
			block:    map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": "not-a-url"}},
			role:     "user",
			wantCode: "invalid_media",
		},
		{
			name:     "file without media object",
			block:    map[string]any{"type": "file"},
			role:     "user",
			wantCode: "missing_file",
		},
		{
			name:     "file with empty file_data",
			block:    map[string]any{"type": "file", "file": map[string]any{"file_data": ""}},
			role:     "user",
			wantCode: "invalid_media",
		},
		{
			name:     "valid user image block",
			block:    map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/a.png"}},
			role:     "user",
			wantCode: "",
		},
		{
			name:     "valid user audio block",
			block:    map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": "data:audio/wav;base64,QUJD"}},
			role:     "user",
			wantCode: "",
		},
		{
			name:     "valid user file block",
			block:    map[string]any{"type": "file", "file": map[string]any{"file_data": "https://example.com/f.pdf"}},
			role:     "user",
			wantCode: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateContentBlock(tc.block, tc.role, 0)
			if tc.wantCode == "" {
				assert.Nil(t, err)
				return
			}
			require.NotNil(t, err)
			assert.Equal(t, tc.wantCode, err.code)
		})
	}
}

func TestValidateMediaInput(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "empty", value: "", wantErr: true},
		{name: "data uri", value: "data:image/png;base64,AAAA", wantErr: false},
		{name: "http url", value: "http://example.com/a.png", wantErr: false},
		{name: "https url", value: "https://example.com/a.png", wantErr: false},
		{name: "raw base64 rejected", value: strings.Repeat("QUJD", 32), wantErr: true},
		{name: "plain string rejected", value: "just some text", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateMediaInput(tc.value)
			if tc.wantErr {
				require.NotNil(t, err)
				assert.Equal(t, "invalid_media", err.code)
				return
			}
			assert.Nil(t, err)
		})
	}
}

func TestLooksLikeBase64(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "valid base64 long enough", value: strings.Repeat("QUJDREVG", 8), want: true},
		{name: "too short", value: "QUJD", want: false},
		{name: "length not multiple of 4", value: strings.Repeat("QUJDRE", 5) + "QUJ", want: false},
		{name: "invalid base64 chars", value: strings.Repeat("!!!!****", 8), want: false},
		{name: "whitespace stripped then valid", value: strings.Repeat("QUJD REVG", 8), want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, looksLikeBase64(tc.value))
		})
	}
}

func TestIsStreamEnabled(t *testing.T) {
	assert.False(t, isStreamEnabled(nil))
	assert.True(t, isStreamEnabled(boolPtr(true)))
	assert.False(t, isStreamEnabled(boolPtr(false)))
}

func boolPtr(b bool) *bool { return &b }

func TestToolCallsEnabled(t *testing.T) {
	tests := []struct {
		name string
		req  *ChatRequest
		want bool
	}{
		{name: "nil request", req: nil, want: false},
		{name: "no tools", req: &ChatRequest{Model: "m"}, want: false},
		{name: "tools with choice none", req: &ChatRequest{Model: "m", Tools: []flow.Tool{{Type: "function", Function: flow.Function{Name: "f"}}}, ToolChoice: "none"}, want: false},
		{name: "tools with choice auto", req: &ChatRequest{Model: "m", Tools: []flow.Tool{{Type: "function", Function: flow.Function{Name: "f"}}}, ToolChoice: "auto"}, want: true},
		{name: "tools without choice", req: &ChatRequest{Model: "m", Tools: []flow.Tool{{Type: "function", Function: flow.Function{Name: "f"}}}}, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, toolCallsEnabled(tc.req))
		})
	}
}

func TestNormalizeTemperature(t *testing.T) {
	tests := []struct {
		name     string
		temp     *float64
		wantCode string
	}{
		{name: "nil gets default", temp: nil, wantCode: ""},
		{name: "in range", temp: float64Ptr(1.5), wantCode: ""},
		{name: "too low", temp: float64Ptr(-1), wantCode: "invalid_temperature"},
		{name: "too high", temp: float64Ptr(3), wantCode: "invalid_temperature"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &ChatRequest{Temperature: tc.temp}
			err := normalizeTemperature(req)
			if tc.wantCode == "" {
				assert.Nil(t, err)
				require.NotNil(t, req.Temperature)
				return
			}
			require.NotNil(t, err)
			assert.Equal(t, tc.wantCode, err.code)
		})
	}
}

func TestNormalizeTopP(t *testing.T) {
	tests := []struct {
		name     string
		topP     *float64
		wantCode string
	}{
		{name: "nil gets default", topP: nil, wantCode: ""},
		{name: "in range", topP: float64Ptr(0.5), wantCode: ""},
		{name: "too low", topP: float64Ptr(-0.5), wantCode: "invalid_top_p"},
		{name: "too high", topP: float64Ptr(2), wantCode: "invalid_top_p"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &ChatRequest{TopP: tc.topP}
			err := normalizeTopP(req)
			if tc.wantCode == "" {
				assert.Nil(t, err)
				require.NotNil(t, req.TopP)
				return
			}
			require.NotNil(t, err)
			assert.Equal(t, tc.wantCode, err.code)
		})
	}
}

func TestShouldShowThinking(t *testing.T) {
	tests := []struct {
		name string
		req  *ChatRequest
		cfg  *config.Config
		want bool
	}{
		{
			name: "effort low overrides config",
			req:  &ChatRequest{ReasoningEffort: "low"},
			cfg:  &config.Config{App: config.AppConfig{Thinking: false}},
			want: true,
		},
		{
			name: "effort none overrides config",
			req:  &ChatRequest{ReasoningEffort: "none"},
			cfg:  &config.Config{App: config.AppConfig{Thinking: true}},
			want: false,
		},
		{
			name: "config thinking true",
			cfg:  &config.Config{App: config.AppConfig{Thinking: true}},
			want: true,
		},
		{
			name: "no request no config",
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, shouldShowThinking(tc.req, tc.cfg))
		})
	}
}

func TestValidateMessage_MapAndBlockErrorPropagation(t *testing.T) {
	t.Run("map content valid", func(t *testing.T) {
		err := validateMessage(ChatMessage{
			Role:    "user",
			Content: map[string]any{"type": "text", "text": "hi"},
		}, 0)
		assert.Nil(t, err)
	})

	t.Run("nil map content rejected", func(t *testing.T) {
		err := validateMessage(ChatMessage{Role: "user", Content: map[string]any(nil)}, 0)
		require.NotNil(t, err)
		assert.Equal(t, "invalid_content_item", err.code)
	})

	t.Run("array block error propagates", func(t *testing.T) {
		err := validateMessage(ChatMessage{
			Role: "user",
			Content: []any{
				map[string]any{"type": "text", "text": "ok"},
				map[string]any{"type": "text", "text": "   "},
			},
		}, 0)
		require.NotNil(t, err)
		assert.Equal(t, "empty_text", err.code)
	})
}
