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

func TestConsoleResponsesSuccess(t *testing.T) {
	stub := newStubUpstream(t, sseHandler(
		"event: response.output_text.delta\r\n",
		"data: {\"delta\":\"Hi\"}\r\n",
		"\r\n",
		"data: [DONE]\r\n",
		"\r\n",
	))
	c := newTestClient(stub.doer(), nil)

	events, err := c.ConsoleResponses(context.Background(), &ConsoleRequest{
		Model: "grok-4",
		Input: []ConsoleInputItem{{
			Role:    "user",
			Content: []ConsoleContent{{Type: "input_text", Text: "hello"}},
		}},
	})
	require.NoError(t, err)

	got := collectEvents(t, events)
	require.Len(t, got, 1)
	assert.NoError(t, got[0].Error)

	var wrapped struct {
		Event string          `json:"event"`
		Data  json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(got[0].Data, &wrapped))
	assert.Equal(t, "response.output_text.delta", wrapped.Event)
	assert.Contains(t, string(wrapped.Data), "\"delta\":\"Hi\"")

	calls := stub.calls()
	require.Len(t, calls, 1)
	assert.Equal(t, "/v1/responses", calls[0].Path)
	assert.Equal(t, "https://console.x.ai", calls[0].Header.Get("Origin"))
	assert.Equal(t, "https://console.x.ai/", calls[0].Header.Get("Referer"))
	assert.Contains(t, calls[0].Header.Get("Cookie"), "sso=test-token")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(calls[0].Body, &payload))
	assert.Equal(t, "grok-4", payload["model"])
	assert.Equal(t, true, payload["stream"])
}

func TestConsoleResponsesStatusMapping(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		contentType string
		body        string
		wantErr     error
		wantText    string
	}{
		{"401 invalid token", http.StatusUnauthorized, "application/json", "{}", ErrInvalidToken, ""},
		{"402 credit exhausted", http.StatusPaymentRequired, "application/json", "{}", ErrConsoleCreditExhausted, ""},
		{"403 cloudflare challenge", http.StatusForbidden, "text/html", "<html>Just a moment</html>", ErrCFChallenge, ""},
		{"403 forbidden", http.StatusForbidden, "application/json", `{"error":"x"}`, ErrForbidden, ""},
		{"429 rate limited", http.StatusTooManyRequests, "application/json", "{}", ErrRateLimited, ""},
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

			events, err := c.ConsoleResponses(context.Background(), &ConsoleRequest{
				Model: "grok-4",
				Input: []ConsoleInputItem{{
					Role:    "user",
					Content: []ConsoleContent{{Type: "input_text", Text: "hi"}},
				}},
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

func TestConsoleResponsesNetworkErrorWrapped(t *testing.T) {
	doer := &failDoer{err: &url.Error{Op: "Post", URL: consoleResponsesAPIURL, Err: errors.New("dial fail")}}
	c := newTestClient(doer, nil)

	events, err := c.ConsoleResponses(context.Background(), &ConsoleRequest{
		Model: "grok-4",
		Input: []ConsoleInputItem{{
			Role:    "user",
			Content: []ConsoleContent{{Type: "input_text", Text: "hi"}},
		}},
	})
	require.NoError(t, err)

	got := collectEvents(t, events)
	require.Len(t, got, 1)
	require.Error(t, got[0].Error)
	assert.ErrorIs(t, got[0].Error, ErrNetwork)
	assert.EqualValues(t, 1, doer.calls.Load())
}

func TestConsoleResponsesMaxRetriesExceeded(t *testing.T) {
	doer := &failDoer{err: errors.New("always fails")}
	c := newTestClient(doer, func(o *Options) { o.MaxRetry = 1 })

	events, err := c.ConsoleResponses(context.Background(), &ConsoleRequest{
		Model: "grok-4",
		Input: []ConsoleInputItem{{
			Role:    "user",
			Content: []ConsoleContent{{Type: "input_text", Text: "hi"}},
		}},
	})
	require.NoError(t, err)

	got := collectEvents(t, events)
	require.Len(t, got, 1)
	require.Error(t, got[0].Error)
	assert.Contains(t, got[0].Error.Error(), "max retries exceeded")
	assert.EqualValues(t, 2, doer.calls.Load())
}

func TestConsoleResponsesStopsOnContextCanceled(t *testing.T) {
	doer := &failDoer{err: context.Canceled}
	c := newTestClient(doer, nil)

	events, err := c.ConsoleResponses(context.Background(), &ConsoleRequest{
		Model: "grok-4",
		Input: []ConsoleInputItem{{
			Role:    "user",
			Content: []ConsoleContent{{Type: "input_text", Text: "hi"}},
		}},
	})
	require.NoError(t, err)

	got := collectEvents(t, events)
	require.Len(t, got, 1)
	require.Error(t, got[0].Error)
	assert.ErrorIs(t, got[0].Error, context.Canceled)
	assert.EqualValues(t, 1, doer.calls.Load())
}

func TestConsoleResponsesCancelDuringRetryWait(t *testing.T) {
	sig := make(chan struct{})
	doer := &failDoer{err: errors.New("fail first"), onCall: sig}
	c := newTestClient(doer, func(o *Options) {
		o.MaxRetry = 2
		o.RetryInterval = 10 * time.Second
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	events, err := c.ConsoleResponses(ctx, &ConsoleRequest{
		Model: "grok-4",
		Input: []ConsoleInputItem{{
			Role:    "user",
			Content: []ConsoleContent{{Type: "input_text", Text: "hi"}},
		}},
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

func TestConsoleResponsesStopsOnForbiddenWithoutRetry(t *testing.T) {
	stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"error":"x"}`)
	})
	c := newTestClient(stub.doer(), func(o *Options) { o.MaxRetry = 5 })

	events, err := c.ConsoleResponses(context.Background(), &ConsoleRequest{
		Model: "grok-4",
		Input: []ConsoleInputItem{{
			Role:    "user",
			Content: []ConsoleContent{{Type: "input_text", Text: "hi"}},
		}},
	})
	require.NoError(t, err)

	got := collectEvents(t, events)
	require.Len(t, got, 1)
	require.Error(t, got[0].Error)
	assert.ErrorIs(t, got[0].Error, ErrForbidden)
	assert.Len(t, stub.calls(), 1)
}

func TestConsoleResponsesClosedClient(t *testing.T) {
	c := newTestClient(&staticDoer{}, nil)
	require.NoError(t, c.Close())

	events, err := c.ConsoleResponses(context.Background(), &ConsoleRequest{Model: "grok-4"})
	assert.Nil(t, events)
	assert.ErrorIs(t, err, ErrStreamClosed)
}

func TestConsoleResponsesInputErrors(t *testing.T) {
	c := newTestClient(&staticDoer{}, nil)

	t.Run("nil request", func(t *testing.T) {
		events, err := c.ConsoleResponses(context.Background(), nil)
		assert.Nil(t, events)
		assert.EqualError(t, err, "console request is nil")
	})

	t.Run("missing model", func(t *testing.T) {
		events, err := c.ConsoleResponses(context.Background(), &ConsoleRequest{Model: "   "})
		assert.Nil(t, events)
		assert.EqualError(t, err, "console model is required")
	})
}

func TestConsoleReasoningEffort(t *testing.T) {
	tests := []struct {
		name      string
		effort    string
		supported bool
		want      string
	}{
		{"empty supported", "", true, ""},
		{"none", "none", true, ""},
		{"none padded", "  NONE  ", true, ""},
		{"xhigh maps to high", "xhigh", true, "high"},
		{"default lowercases", "HIGH", true, "high"},
		{"unsupported always empty", "high", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, consoleReasoningEffort(tt.effort, tt.supported))
		})
	}
}
