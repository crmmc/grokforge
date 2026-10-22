package xai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatSuccess(t *testing.T) {
	stub := newStubUpstream(t, sseHandler(
		"data: {\"n\":1}\n\n",
		"data: {\"n\":2}\n\n",
		"data: [DONE]\n\n",
	))
	c := newTestClient(stub.doer(), nil)

	events, err := c.Chat(context.Background(), &ChatRequest{
		Messages:     []Message{{Role: "user", Content: "hello"}},
		UpstreamMode: "auto",
	})
	require.NoError(t, err)

	got := collectEvents(t, events)
	require.Len(t, got, 2)
	for _, ev := range got {
		assert.NoError(t, ev.Error)
		assert.Contains(t, string(ev.Data), "\"n\":")
	}

	calls := stub.calls()
	require.Len(t, calls, 1)
	assert.Equal(t, http.MethodPost, calls[0].Method)
	assert.Equal(t, "/rest/app-chat/conversations/new", calls[0].Path)
	assert.Contains(t, calls[0].Header.Get("Cookie"), "sso=test-token")
	assert.Equal(t, "https://grok.com", calls[0].Header.Get("Origin"))

	var payload map[string]any
	require.NoError(t, json.Unmarshal(calls[0].Body, &payload))
	assert.Equal(t, "auto", payload["modeId"])
	assert.Equal(t, "hello", payload["message"])
}

func TestChatStatusMapping(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		contentType string
		body        string
		wantErr     error
		wantText    string
	}{
		{"429 rate limited", http.StatusTooManyRequests, "application/json", "{}", ErrRateLimited, ""},
		{"401 invalid token", http.StatusUnauthorized, "application/json", "{}", ErrInvalidToken, ""},
		{"403 cloudflare challenge", http.StatusForbidden, "text/html", "<html>Just a moment</html>", ErrCFChallenge, ""},
		{"403 forbidden", http.StatusForbidden, "application/json", `{"error":"blocked"}`, ErrForbidden, ""},
		{"500 unexpected status", http.StatusInternalServerError, "text/plain", "boom", nil, "unexpected status 500"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			})
			c := newTestClient(stub.doer(), nil)

			events, err := c.Chat(context.Background(), &ChatRequest{
				Messages:     []Message{{Role: "user", Content: "hi"}},
				UpstreamMode: "auto",
			})
			require.NoError(t, err)

			got := collectEvents(t, events)
			require.Len(t, got, 1)
			require.Error(t, got[0].Error)
			if tt.wantErr != nil {
				assert.ErrorIs(t, got[0].Error, tt.wantErr)
			} else {
				assert.Contains(t, got[0].Error.Error(), tt.wantText)
			}
			assert.Len(t, stub.calls(), 1) // MaxRetry=0
		})
	}
}

func TestChatRetryThenSuccess(t *testing.T) {
	stub := newStubUpstream(t, sseHandler("data: {\"ok\":true}\n\n", "data: [DONE]\n\n"))
	doer := &flakyDoer{failures: 1, err: errors.New("transient"), next: stub.doer()}
	c := newTestClient(doer, func(o *Options) { o.MaxRetry = 2 })

	events, err := c.Chat(context.Background(), &ChatRequest{
		Messages:     []Message{{Role: "user", Content: "hi"}},
		UpstreamMode: "auto",
	})
	require.NoError(t, err)

	got := collectEvents(t, events)
	require.Len(t, got, 1)
	assert.NoError(t, got[0].Error)
	assert.EqualValues(t, 2, doer.calls.Load())
}

func TestChatMaxRetriesExceeded(t *testing.T) {
	doer := &failDoer{err: errors.New("always fails")}
	c := newTestClient(doer, func(o *Options) { o.MaxRetry = 1 })

	events, err := c.Chat(context.Background(), &ChatRequest{
		Messages:     []Message{{Role: "user", Content: "hi"}},
		UpstreamMode: "auto",
	})
	require.NoError(t, err)

	got := collectEvents(t, events)
	require.Len(t, got, 1)
	require.Error(t, got[0].Error)
	assert.Contains(t, got[0].Error.Error(), "max retries exceeded")
	assert.EqualValues(t, 2, doer.calls.Load())
}

func TestChatNetworkErrorWrapped(t *testing.T) {
	doer := &failDoer{err: &url.Error{Op: "Post", URL: grokAPIURL, Err: errors.New("dial fail")}}
	c := newTestClient(doer, nil)

	events, err := c.Chat(context.Background(), &ChatRequest{
		Messages:     []Message{{Role: "user", Content: "hi"}},
		UpstreamMode: "auto",
	})
	require.NoError(t, err)

	got := collectEvents(t, events)
	require.Len(t, got, 1)
	require.Error(t, got[0].Error)
	assert.ErrorIs(t, got[0].Error, ErrNetwork)
	assert.EqualValues(t, 1, doer.calls.Load())
}

func TestChatStopsOnContextCanceledFromDoer(t *testing.T) {
	doer := &failDoer{err: context.Canceled}
	c := newTestClient(doer, nil)

	events, err := c.Chat(context.Background(), &ChatRequest{
		Messages:     []Message{{Role: "user", Content: "hi"}},
		UpstreamMode: "auto",
	})
	require.NoError(t, err)

	got := collectEvents(t, events)
	require.Len(t, got, 1)
	require.Error(t, got[0].Error)
	assert.ErrorIs(t, got[0].Error, context.Canceled)
	assert.EqualValues(t, 1, doer.calls.Load())
}

func TestChatCancelDuringRetryWait(t *testing.T) {
	sig := make(chan struct{})
	doer := &failDoer{err: errors.New("fail first"), onCall: sig}
	c := newTestClient(doer, func(o *Options) {
		o.MaxRetry = 2
		o.RetryInterval = 10 * time.Second
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, err := c.Chat(ctx, &ChatRequest{
		Messages:     []Message{{Role: "user", Content: "hi"}},
		UpstreamMode: "auto",
	})
	require.NoError(t, err)

	<-sig // first attempt failed
	cancel()

	got := collectEvents(t, events)
	require.Len(t, got, 1)
	require.Error(t, got[0].Error)
	assert.ErrorIs(t, got[0].Error, context.Canceled)
	assert.EqualValues(t, 1, doer.calls.Load())
}

func TestChatStopsOnForbiddenWithoutRetry(t *testing.T) {
	stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":"blocked"}`)
	})
	c := newTestClient(stub.doer(), func(o *Options) { o.MaxRetry = 5 })

	events, err := c.Chat(context.Background(), &ChatRequest{
		Messages:     []Message{{Role: "user", Content: "hi"}},
		UpstreamMode: "auto",
	})
	require.NoError(t, err)

	got := collectEvents(t, events)
	require.Len(t, got, 1)
	require.Error(t, got[0].Error)
	assert.ErrorIs(t, got[0].Error, ErrForbidden)
	assert.Len(t, stub.calls(), 1)
}

func TestChatClosedClient(t *testing.T) {
	c := newTestClient(&staticDoer{}, nil)
	require.NoError(t, c.Close())

	events, err := c.Chat(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	assert.Nil(t, events)
	assert.ErrorIs(t, err, ErrStreamClosed)
}

func TestChatBuildBodyError(t *testing.T) {
	c := newTestClient(&staticDoer{}, nil)
	events, err := c.Chat(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	assert.Nil(t, events)
	assert.EqualError(t, err, "upstream mode is required")
}

func TestParseSSEStreamScannerError(t *testing.T) {
	c := &client{}
	events := make(chan StreamEvent, 4)
	err := c.parseSSEStream(&errorReader{err: errors.New("read fail")}, events)
	close(events)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNetwork)
}

func TestFlattenMessages(t *testing.T) {
	tests := []struct {
		name     string
		messages []Message
		want     string
	}{
		{"empty", nil, ""},
		{"single message", []Message{{Role: "user", Content: "hi"}}, "hi"},
		{"user last", []Message{{Role: "system", Content: "sys"}, {Role: "user", Content: "q"}}, "system: sys\n\nq"},
		{"no user message", []Message{{Role: "system", Content: "s"}, {Role: "assistant", Content: "a"}}, "system: s\n\nassistant: a"},
		{"user in middle", []Message{
			{Role: "user", Content: "u1"},
			{Role: "assistant", Content: "a"},
			{Role: "user", Content: "u2"},
		}, "user: u1\n\nassistant: a\n\nu2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, flattenMessages(tt.messages))
		})
	}
}

func TestCloneMap(t *testing.T) {
	t.Run("nil returns empty map", func(t *testing.T) {
		got := cloneMap(nil)
		require.NotNil(t, got)
		assert.Empty(t, got)
	})

	t.Run("deep clones nested maps", func(t *testing.T) {
		src := map[string]any{
			"scalar": 1,
			"outer":  map[string]any{"inner": map[string]any{"k": "v"}},
		}
		cloned := cloneMap(src)
		require.NotSame(t, &src, &cloned)

		nested := cloned["outer"].(map[string]any)
		inner := nested["inner"].(map[string]any)
		inner["k"] = "changed"

		assert.Equal(t, 1, cloned["scalar"])
		assert.Equal(t, "v", src["outer"].(map[string]any)["inner"].(map[string]any)["k"])
		assert.Equal(t, "changed", inner["k"])
	})
}

func TestTruncateBody(t *testing.T) {
	tests := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"shorter than limit", "short", 10, "short"},
		{"longer than limit", "1234567890", 5, "12345"},
		{"empty", "", 5, ""},
		{"exact length", "abc", 3, "abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, truncateBody(tt.in, tt.n))
		})
	}
}

func TestBuildChatBodyOptionalFields(t *testing.T) {
	temp := 0.5
	topP := 0.8
	tests := []struct {
		name   string
		mutate func(*ChatRequest)
		check  func(t *testing.T, payload map[string]any)
	}{
		{
			name:   "temperature",
			mutate: func(r *ChatRequest) { r.Temperature = &temp },
			check: func(t *testing.T, payload map[string]any) {
				mco := payload["responseMetadata"].(map[string]any)["modelConfigOverride"].(map[string]any)
				assert.Equal(t, 0.5, mco["temperature"])
			},
		},
		{
			name:   "top p and reasoning effort",
			mutate: func(r *ChatRequest) { r.TopP = &topP; r.ReasoningEffort = "HIGH" },
			check: func(t *testing.T, payload map[string]any) {
				mco := payload["responseMetadata"].(map[string]any)["modelConfigOverride"].(map[string]any)
				assert.Equal(t, 0.8, mco["topP"])
				assert.Equal(t, "HIGH", mco["reasoningEffort"])
			},
		},
		{
			name:   "deep search preset",
			mutate: func(r *ChatRequest) { r.DeepSearch = "deeper" },
			check: func(t *testing.T, payload map[string]any) {
				assert.Equal(t, "deeper", payload["deepsearchPreset"])
			},
		},
		{
			name:   "image edit flags",
			mutate: func(r *ChatRequest) { r.IsImageEdit = true },
			check: func(t *testing.T, payload map[string]any) {
				assert.Equal(t, false, payload["isReasoning"])
				assert.Equal(t, true, payload["disableTextFollowUps"])
			},
		},
		{
			name: "media request sends model name",
			mutate: func(r *ChatRequest) {
				r.ToolOverrides = map[string]any{"imageGen": true}
				r.UpstreamModel = "imagine-image-edit"
			},
			check: func(t *testing.T, payload map[string]any) {
				assert.Equal(t, "imagine-image-edit", payload["modelName"])
				_, hasMode := payload["modeId"]
				assert.False(t, hasMode)
			},
		},
		{
			name:   "nil file attachments becomes empty array",
			mutate: func(r *ChatRequest) {},
			check: func(t *testing.T, payload map[string]any) {
				assert.Equal(t, []any{}, payload["fileAttachments"])
			},
		},
		{
			name: "nested model config is cloned",
			mutate: func(r *ChatRequest) {
				r.ModelConfig = map[string]any{"nested": map[string]any{"k": "v"}}
			},
			check: func(t *testing.T, payload map[string]any) {
				mco := payload["responseMetadata"].(map[string]any)["modelConfigOverride"].(map[string]any)
				assert.Equal(t, map[string]any{"k": "v"}, mco["nested"])
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &ChatRequest{
				Messages:      []Message{{Role: "user", Content: "hi"}},
				UpstreamModel: "grok-3",
				UpstreamMode:  "auto",
			}
			tt.mutate(req)
			body, err := buildChatBody(req)
			require.NoError(t, err)
			var payload map[string]any
			require.NoError(t, json.Unmarshal(body, &payload))
			tt.check(t, payload)
		})
	}
}
