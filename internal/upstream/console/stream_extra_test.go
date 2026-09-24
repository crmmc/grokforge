package console

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConsoleEvent_Events(t *testing.T) {
	tests := []struct {
		name       string
		event      string
		data       string
		want       upstream.StreamEvent
		wantIgnore bool
	}{
		{
			name:  "output text delta",
			event: "response.output_text.delta",
			data:  `{"delta":"hello"}`,
			want:  upstream.StreamEvent{Content: "hello"},
		},
		{
			name:  "reasoning summary text delta",
			event: "response.reasoning_summary_text.delta",
			data:  `{"delta":"thinking"}`,
			want:  upstream.StreamEvent{ReasoningContent: "thinking", IsThinking: true},
		},
		{
			name:  "reasoning summary delta uses text field",
			event: "response.reasoning_summary.delta",
			data:  `{"text":"pondering"}`,
			want:  upstream.StreamEvent{ReasoningContent: "pondering", IsThinking: true},
		},
		{
			name:  "annotation added",
			event: "response.output_text.annotation.added",
			data:  `{"annotation":{"url":"https://example.com/a","title":"A"}}`,
			want:  upstream.StreamEvent{SearchSources: []upstream.SearchSource{{URL: "https://example.com/a", Title: "A", Type: "web"}}},
		},
		{
			name:  "completed with usage",
			event: "response.completed",
			data:  `{"response":{"usage":{"input_tokens":3,"output_tokens":1}}}`,
			want:  upstream.StreamEvent{Usage: &upstream.Usage{PromptTokens: 3, CompletionTokens: 1, TotalTokens: 4}},
		},
		{
			name:       "completed without usage",
			event:      "response.completed",
			data:       `{"response":{}}`,
			wantIgnore: true,
		},
		{
			name:  "failed with message",
			event: "response.failed",
			data:  `{"error":{"message":"rate limited"}}`,
			want:  upstream.StreamEvent{Error: mustConsoleError(t, `{"error":{"message":"rate limited"}}`, "response.failed")},
		},
		{
			name:  "top level error event with string data",
			event: "error",
			data:  `"HTTP 429 too many"`,
			want:  upstream.StreamEvent{Error: mustConsoleError(t, `"HTTP 429 too many"`, "error")},
		},
		{
			name:       "unknown event ignored",
			event:      "response.heartbeat",
			data:       `{"delta":"x"}`,
			wantIgnore: true,
		},
		{
			name:  "event name inferred from data type",
			event: "",
			data:  `{"type":"response.output_text.delta","delta":"inferred"}`,
			want:  upstream.StreamEvent{Content: "inferred"},
		},
		{
			name:  "event name inferred whitespace trimmed",
			event: "   ",
			data:  `{"type":"response.reasoning_summary_text.delta","delta":"ws"}`,
			want:  upstream.StreamEvent{ReasoningContent: "ws", IsThinking: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseConsoleEvent(tt.event, json.RawMessage(tt.data))
			if tt.wantIgnore {
				assert.True(t, isEmptyConsoleEvent(got), "expected empty event, got %#v", got)
				return
			}
			if tt.want.Error != nil {
				require.Error(t, got.Error)
				assert.Equal(t, tt.want.Error.Error(), got.Error.Error())
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func mustConsoleError(t *testing.T, data, eventName string) error {
	t.Helper()
	return consoleError(json.RawMessage(data), eventName)
}

func TestConsoleEventType(t *testing.T) {
	assert.Equal(t, "response.completed", consoleEventType(json.RawMessage(`{"type":"response.completed"}`)))
	assert.Equal(t, "response.completed", consoleEventType(json.RawMessage(`{"type":"  response.completed  "}`)))
	assert.Empty(t, consoleEventType(json.RawMessage(`{"delta":"x"}`)))
	assert.Empty(t, consoleEventType(json.RawMessage(`{bad`)))
}

func TestConsoleDeltaText(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "delta wins", data: `{"delta":"d","text":"t"}`, want: "d"},
		{name: "text fallback", data: `{"text":"t"}`, want: "t"},
		{name: "neither", data: `{"other":1}`, want: ""},
		{name: "invalid json", data: `{bad`, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, consoleDeltaText(json.RawMessage(tt.data)))
		})
	}
}

func TestConsoleUsage(t *testing.T) {
	tests := []struct {
		name string
		data string
		want *upstream.Usage
	}{
		{name: "invalid json", data: `{bad`, want: nil},
		{name: "no usage", data: `{"response":{}}`, want: nil},
		{name: "top level usage", data: `{"usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}`, want: &upstream.Usage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5}},
		{
			name: "response usage computes total",
			data: `{"response":{"usage":{"input_tokens":2,"output_tokens":3}}}`,
			want: &upstream.Usage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5},
		},
		{
			name: "alt key names",
			data: `{"usage":{"prompt_tokens":7,"completionTokens":8}}`,
			want: &upstream.Usage{PromptTokens: 7, CompletionTokens: 8, TotalTokens: 15},
		},
		{
			name: "non map payload",
			data: `[1,2]`,
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, consoleUsage(json.RawMessage(tt.data)))
		})
	}
}

func TestFindConsoleUsageMap(t *testing.T) {
	t.Run("top level usage", func(t *testing.T) {
		m, ok := findConsoleUsageMap(map[string]any{"usage": map[string]any{"input_tokens": 1}})
		assert.True(t, ok)
		assert.NotNil(t, m)
	})
	t.Run("nested response usage", func(t *testing.T) {
		m, ok := findConsoleUsageMap(map[string]any{"response": map[string]any{"usage": map[string]any{"input_tokens": 1}}})
		assert.True(t, ok)
		assert.NotNil(t, m)
	})
	t.Run("missing usage", func(t *testing.T) {
		_, ok := findConsoleUsageMap(map[string]any{"other": 1})
		assert.False(t, ok)
	})
	t.Run("non map", func(t *testing.T) {
		_, ok := findConsoleUsageMap("string")
		assert.False(t, ok)
	})
}

func TestConsoleNumber(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  int
	}{
		{name: "float64", value: 3.9, want: 3},
		{name: "int", value: 5, want: 5},
		{name: "json number", value: json.Number("42"), want: 42},
		{name: "string", value: "x", want: 0},
		{name: "nil", value: nil, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, consoleNumber(tt.value))
		})
	}
}

func TestIntFromConsoleMap_KeysInOrder(t *testing.T) {
	m := map[string]any{"b": 2, "a": 1}
	assert.Equal(t, 1, intFromConsoleMap(m, "a", "b"))
	assert.Equal(t, 2, intFromConsoleMap(m, "b"))
	assert.Equal(t, 0, intFromConsoleMap(m, "missing"))
}

func TestConsoleSearchSources(t *testing.T) {
	t.Run("invalid json", func(t *testing.T) {
		assert.Nil(t, consoleSearchSources(json.RawMessage(`{bad`)))
	})
	t.Run("nested collection", func(t *testing.T) {
		sources := consoleSearchSources(json.RawMessage(`{
			"item": {
				"content": [
					{"url": "https://example.com/a", "title": "A"},
					{"url": "https://x.com/u/status/9", "text": "Post"},
					{"url": "http://twitter.com/u/status/8", "name": "Tw"},
					{"url": "", "title": "skipped"}
				]
			}
		}`))
		require.Len(t, sources, 3)
		assert.Equal(t, "web", sources[0].Type)
		assert.Equal(t, "x_post", sources[1].Type)
		assert.Equal(t, "x_post", sources[2].Type)
		assert.Equal(t, "Tw", sources[2].Title)
	})
}

func TestConsoleSourceTitle(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]any
		want string
	}{
		{name: "title", m: map[string]any{"title": "T"}, want: "T"},
		{name: "text", m: map[string]any{"text": "X"}, want: "X"},
		{name: "name", m: map[string]any{"name": "N"}, want: "N"},
		{name: "query", m: map[string]any{"query": "Q"}, want: "Q"},
		{name: "none", m: map[string]any{"other": "O"}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, consoleSourceTitle(tt.m))
		})
	}
}

func TestConsoleSourceType(t *testing.T) {
	assert.Equal(t, "x_post", consoleSourceType("https://x.com/u/status/1"))
	assert.Equal(t, "x_post", consoleSourceType("https://www.twitter.com/u/status/1"))
	assert.Equal(t, "web", consoleSourceType("https://example.com/a"))
}

func TestConsoleError(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		eventName string
		wantIs    error
		wantSub   string
	}{
		{
			name:    "string payload classified",
			data:    `"credit exhausted"`,
			wantIs:  upstream.ErrCreditExhausted,
			wantSub: "credit exhausted",
		},
		{
			name:      "invalid json falls back to event name",
			data:      `{{{`,
			eventName: "response.failed",
			wantIs:    upstream.ErrServerError,
			wantSub:   "console response.failed",
		},
		{
			name:   "message map",
			data:   `{"message":"payment required now"}`,
			wantIs: upstream.ErrCreditExhausted,
		},
		{
			name:   "nested error map",
			data:   `{"error":{"message":"too many requests"}}`,
			wantIs: upstream.ErrRateLimited,
		},
		{
			name:   "nested response map",
			data:   `{"response":{"message":"server exploded"}}`,
			wantIs: upstream.ErrServerError,
		},
		{
			name:      "no message anywhere",
			data:      `{"other":1}`,
			eventName: "error",
			wantIs:    upstream.ErrServerError,
			wantSub:   "console error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := consoleError(json.RawMessage(tt.data), tt.eventName)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantIs)
			if tt.wantSub != "" {
				assert.Contains(t, err.Error(), tt.wantSub)
			}
		})
	}
}

func TestClassifyConsoleError(t *testing.T) {
	tests := []struct {
		name   string
		msg    string
		wantIs error
	}{
		{name: "rate", msg: "rate limited", wantIs: upstream.ErrRateLimited},
		{name: "too many", msg: "Too Many Requests", wantIs: upstream.ErrRateLimited},
		{name: "429", msg: "HTTP 429", wantIs: upstream.ErrRateLimited},
		{name: "credit", msg: "Credit exhausted", wantIs: upstream.ErrCreditExhausted},
		{name: "quota", msg: "quota exceeded", wantIs: upstream.ErrCreditExhausted},
		{name: "payment required", msg: "Payment Required", wantIs: upstream.ErrCreditExhausted},
		{name: "402", msg: "error 402", wantIs: upstream.ErrCreditExhausted},
		{name: "other", msg: "weird failure", wantIs: upstream.ErrServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := classifyConsoleError(tt.msg)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantIs)
			assert.Contains(t, err.Error(), tt.msg)
		})
	}
}

func TestFindConsoleErrorMessage(t *testing.T) {
	tests := []struct {
		name    string
		payload any
		want    string
	}{
		{name: "non map", payload: "str", want: ""},
		{name: "direct message", payload: map[string]any{"message": "direct"}, want: "direct"},
		{name: "nested error map", payload: map[string]any{"error": map[string]any{"message": "inner"}}, want: "inner"},
		{
			name:    "nested response map",
			payload: map[string]any{"response": map[string]any{"error": map[string]any{"message": "deep"}}},
			want:    "deep",
		},
		{name: "no message", payload: map[string]any{"other": 1}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, findConsoleErrorMessage(tt.payload))
		})
	}
}

func TestEmitConsoleSSEBlock(t *testing.T) {
	t.Run("empty block ignored", func(t *testing.T) {
		done, err := emitConsoleSSEBlock(context.Background(), consoleSSEBlock{event: "response.completed"}, make(chan upstream.StreamEvent, 1))
		assert.False(t, done)
		assert.NoError(t, err)
	})

	t.Run("done marker stops stream", func(t *testing.T) {
		done, err := emitConsoleSSEBlock(context.Background(), consoleSSEBlock{data: []string{"[DONE]"}}, make(chan upstream.StreamEvent, 1))
		assert.True(t, done)
		assert.NoError(t, err)
	})

	t.Run("event delivered", func(t *testing.T) {
		ch := make(chan upstream.StreamEvent, 1)
		done, err := emitConsoleSSEBlock(context.Background(), consoleSSEBlock{event: "response.output_text.delta", data: []string{`{"delta":"hi"}`}}, ch)
		assert.False(t, done)
		assert.NoError(t, err)
		select {
		case ev := <-ch:
			assert.Equal(t, "hi", ev.Content)
		default:
			t.Fatal("event not delivered")
		}
	})

	t.Run("cancelled context with blocking channel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		ch := make(chan upstream.StreamEvent) // unbuffered: send blocks, ctx wins deterministically
		block := consoleSSEBlock{event: "response.output_text.delta", data: []string{`{"delta":"hi"}`}}
		done, err := emitConsoleSSEBlock(ctx, block, ch)
		assert.False(t, done)
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestParseStream_CRLFCommentsAndFinalFlush(t *testing.T) {
	body := ": keepalive\r\n" +
		"event: response.output_text.delta\r\n" +
		"data: {\"delta\":\"Hi\"}\r\n" +
		"\r\n" +
		"event: response.output_text.delta\r\n" +
		"data: {\"delta\":\"!\"}" // no trailing blank line: flushed at EOF

	c := New("", nil, Options{})
	ch := make(chan upstream.StreamEvent, 16)
	require.NoError(t, c.parseStream(context.Background(), strings.NewReader(body), ch))
	close(ch)

	var content string
	for ev := range ch {
		require.NoError(t, ev.Error)
		content += ev.Content
	}
	assert.Equal(t, "Hi!", content)
}

func TestParseStream_ContextCancelled(t *testing.T) {
	c := New("", nil, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ch := make(chan upstream.StreamEvent, 16)
	err := c.parseStream(ctx, strings.NewReader("event: e\ndata: {}\n\n"), ch)
	require.ErrorIs(t, err, context.Canceled)
}

func TestParseStream_DoneStopsEarly(t *testing.T) {
	body := "event: response.output_text.delta\n" +
		"data: {\"delta\":\"a\"}\n\n" +
		"data: [DONE]\n\n" +
		"event: response.output_text.delta\n" +
		"data: {\"delta\":\"never\"}\n\n"

	c := New("", nil, Options{})
	ch := make(chan upstream.StreamEvent, 16)
	require.NoError(t, c.parseStream(context.Background(), strings.NewReader(body), ch))
	close(ch)

	var content string
	for ev := range ch {
		require.NoError(t, ev.Error)
		content += ev.Content
	}
	assert.Equal(t, "a", content)
}

func TestParseStream_ScannerError(t *testing.T) {
	c := New("", nil, Options{})
	ch := make(chan upstream.StreamEvent, 16)
	err := c.parseStream(context.Background(),
		io.NopCloser(io.MultiReader(strBody("data: {\"delta\":\"a\"}\n\n"), iotest.ErrReader(io.ErrUnexpectedEOF))), ch)
	require.Error(t, err)
	assert.ErrorIs(t, err, upstream.ErrNetwork)
	assert.Contains(t, err.Error(), "unexpected EOF")
}
