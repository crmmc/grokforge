package console

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDoer is a hand-written Doer fake: it records requests and returns a
// canned response or error.
type fakeDoer struct {
	reqs []*http.Request
	resp *http.Response
	err  error
}

func (f *fakeDoer) Do(req *http.Request) (*http.Response, error) {
	f.reqs = append(f.reqs, req)
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func strBody(s string) io.ReadCloser { return io.NopCloser(strings.NewReader(s)) }

func TestConsoleUpstreamName(t *testing.T) {
	assert.Equal(t, "console", New("", nil, Options{}).Name())
}

func TestConsoleOptionFallbacks(t *testing.T) {
	c := New("", nil, Options{})
	assert.Empty(t, c.cookieString("tok"))
	assert.Nil(t, c.headerOrder())
	assert.Empty(t, c.browserProfile())
	assert.Empty(t, c.userAgent())
	assert.Empty(t, c.statsigID())
}

func TestConsoleChat_SyncErrorPaths(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		req     *upstream.ChatRequest
		wantSub string
	}{
		{
			name:    "nil request",
			req:     nil,
			wantSub: "chat request is nil",
		},
		{
			name: "empty model",
			req: &upstream.ChatRequest{
				Messages: []upstream.Message{{Role: "user", Content: "hi"}},
			},
			wantSub: "console model is required",
		},
		{
			name:    "invalid base url",
			baseURL: "http://[",
			req: &upstream.ChatRequest{
				Messages: []upstream.Message{{Role: "user", Content: "hi"}},
				Model:    "grok-4.20",
			},
			wantSub: "missing ']' in host",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := New(tt.baseURL, &upstream.StdlibDoer{}, Options{})
			_, err := c.Chat(context.Background(), "tok", tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantSub)
			assert.NotErrorIs(t, err, upstream.ErrNetwork)
		})
	}
}

func TestConsoleChat_DoerErrorMapsToNetwork(t *testing.T) {
	doer := &fakeDoer{err: &url.Error{Op: "Post", URL: "https://console.x.ai", Err: io.ErrUnexpectedEOF}}
	c := New("", doer, Options{})
	_, err := c.Chat(context.Background(), "tok", &upstream.ChatRequest{
		Messages: []upstream.Message{{Role: "user", Content: "hi"}},
		Model:    "grok-4.20",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, upstream.ErrNetwork)
	assert.Contains(t, err.Error(), "unexpected EOF")
}

func TestConsoleChat_Unauthorized(t *testing.T) {
	doer := &fakeDoer{resp: &http.Response{
		StatusCode: http.StatusUnauthorized,
		Header:     http.Header{},
		Body:       strBody(""),
	}}
	c := New("", doer, Options{})
	_, err := c.Chat(context.Background(), "tok", &upstream.ChatRequest{
		Messages: []upstream.Message{{Role: "user", Content: "hi"}},
		Model:    "grok-4.20",
	})
	assert.ErrorIs(t, err, upstream.ErrInvalidToken)
}

func TestConsoleChat_StreamErrorEvent(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		ctxDone bool
		wantSub string
	}{
		{
			name:    "malformed error event data",
			body:    "event: error\ndata: {{{\n\n",
			wantSub: "console error",
		},
		{
			name:    "cancelled context mid stream",
			body:    "event: response.output_text.delta\ndata: {\"delta\":\"a\"}\n\n",
			ctxDone: true,
			wantSub: "context canceled",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doer := &fakeDoer{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       strBody(tt.body),
			}}
			c := New("", doer, Options{})
			ctx := context.Background()
			if tt.ctxDone {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			ch, err := c.Chat(ctx, "tok", &upstream.ChatRequest{
				Messages: []upstream.Message{{Role: "user", Content: "hi"}},
				Model:    "grok-4.20",
			})
			require.NoError(t, err)

			var streamErr error
			for ev := range ch {
				if ev.Error != nil {
					streamErr = ev.Error
				}
			}
			require.Error(t, streamErr, "expected stream error event")
			assert.Contains(t, streamErr.Error(), tt.wantSub)
		})
	}
}
