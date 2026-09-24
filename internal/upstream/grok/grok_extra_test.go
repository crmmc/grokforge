package grok

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

func TestGrokUpstreamName(t *testing.T) {
	assert.Equal(t, "grok", New("", nil, Options{}).Name())
}

func TestGrokUploadURL(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want string
	}{
		{name: "nil option falls back to default", opts: Options{}, want: DefaultUploadURL},
		{name: "empty override falls back to default", opts: Options{UploadURL: func() string { return "" }}, want: DefaultUploadURL},
		{name: "override wins", opts: Options{UploadURL: func() string { return "https://example.com/upload" }}, want: "https://example.com/upload"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, New("", nil, tt.opts).uploadURL())
		})
	}
}

func TestGrokCookieStringNilOption(t *testing.T) {
	g := New("", nil, Options{})
	assert.Empty(t, g.cookieString("tok"))
}

func TestGrokChat_SyncErrorPaths(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		req     *upstream.ChatRequest
		wantSub string
	}{
		{
			name:    "multimodal parse failure",
			baseURL: "",
			req: &upstream.ChatRequest{
				Messages: []upstream.Message{{
					Role:    "user",
					Content: []map[string]any{{"type": "definitely_unknown"}},
				}},
				UpstreamMode: "auto",
			},
			wantSub: "unknown content type",
		},
		{
			name:    "missing upstream mode",
			baseURL: "",
			req: &upstream.ChatRequest{
				Messages: []upstream.Message{{Role: "user", Content: "hi"}},
			},
			wantSub: "upstream mode is required",
		},
		{
			name:    "invalid base url",
			baseURL: "http://[",
			req: &upstream.ChatRequest{
				Messages:     []upstream.Message{{Role: "user", Content: "hi"}},
				UpstreamMode: "auto",
			},
			wantSub: "missing ']' in host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := New(tt.baseURL, &upstream.StdlibDoer{}, Options{})
			_, err := g.Chat(context.Background(), "tok", tt.req)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantSub)
			assert.NotErrorIs(t, err, upstream.ErrNetwork)
		})
	}
}

func TestGrokChat_DoerErrorMapsToNetwork(t *testing.T) {
	doer := &fakeDoer{err: &url.Error{Op: "Post", URL: "https://grok.com", Err: io.EOF}}
	g := New("", doer, Options{})
	_, err := g.Chat(context.Background(), "tok", &upstream.ChatRequest{
		Messages:     []upstream.Message{{Role: "user", Content: "hi"}},
		UpstreamMode: "auto",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, upstream.ErrNetwork)
}

func TestGrokChat_Unauthorized(t *testing.T) {
	doer := &fakeDoer{resp: &http.Response{
		StatusCode: http.StatusUnauthorized,
		Header:     http.Header{},
		Body:       strBody(""),
	}}
	g := New("", doer, Options{})
	_, err := g.Chat(context.Background(), "tok", &upstream.ChatRequest{
		Messages:     []upstream.Message{{Role: "user", Content: "hi"}},
		UpstreamMode: "auto",
	})
	assert.ErrorIs(t, err, upstream.ErrInvalidToken)
}

func TestGrokChat_StreamErrorEvent(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantSub string
	}{
		{name: "malformed json event", body: "{\"result\":oops\n", wantSub: ""},
		{name: "cancelled context", body: "{\"result\":{\"response\":{\"token\":\"A\"}}}\n", wantSub: "context canceled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doer := &fakeDoer{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       strBody(tt.body),
			}}
			g := New("", doer, Options{})
			ctx := context.Background()
			if tt.name == "cancelled context" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			ch, err := g.Chat(ctx, "tok", &upstream.ChatRequest{
				Messages:     []upstream.Message{{Role: "user", Content: "hi"}},
				UpstreamMode: "auto",
			})
			require.NoError(t, err)

			var streamErr error
			for ev := range ch {
				if ev.Error != nil {
					streamErr = ev.Error
				}
			}
			require.Error(t, streamErr, "expected stream error event")
			if tt.wantSub != "" {
				assert.Contains(t, streamErr.Error(), tt.wantSub)
			}
		})
	}
}
