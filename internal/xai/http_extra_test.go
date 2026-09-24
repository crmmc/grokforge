package xai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/upstream/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------- shared stub infrastructure ----------

// recordedRequest captures one request received by a stubUpstream.
type recordedRequest struct {
	Method string
	Path   string
	Header http.Header
	Body   []byte
}

// stubUpstream is an httptest server plus a transport.Doer that routes xai
// requests (which target hardcoded grok.com / console.x.ai URLs) onto it.
type stubUpstream struct {
	server *httptest.Server
	base   *url.URL

	mu       sync.Mutex
	requests []recordedRequest
}

func newStubUpstream(t *testing.T, handler http.HandlerFunc) *stubUpstream {
	t.Helper()
	s := &stubUpstream{}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s.mu.Lock()
		s.requests = append(s.requests, recordedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Header: r.Header.Clone(),
			Body:   body,
		})
		s.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(s.server.Close)
	u, err := url.Parse(s.server.URL)
	require.NoError(t, err)
	s.base = u
	return s
}

func (s *stubUpstream) calls() []recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]recordedRequest(nil), s.requests...)
}

// doer returns a Doer that forwards requests to the stub server.
func (s *stubUpstream) doer() transport.Doer { return &routeDoer{base: s.base} }

// routeDoer rewrites the hardcoded xai URLs onto the stub server host.
type routeDoer struct {
	base   *url.URL
	client *http.Client
}

func (d *routeDoer) Do(req *http.Request) (*http.Response, error) {
	client := d.client
	if client == nil {
		client = &http.Client{}
	}
	orig := *req.URL
	req.URL.Scheme = d.base.Scheme
	req.URL.Host = d.base.Host
	resp, err := client.Do(req)
	req.URL.Scheme = orig.Scheme
	req.URL.Host = orig.Host
	return resp, err
}

// staticDoer returns a canned response or error without touching the network.
type staticDoer struct {
	resp  *http.Response
	err   error
	calls atomic.Int32
}

func (d *staticDoer) Do(*http.Request) (*http.Response, error) {
	d.calls.Add(1)
	return d.resp, d.err
}

// failDoer always fails with err; onCall optionally signals after each call.
type failDoer struct {
	err    error
	onCall chan struct{}
	calls  atomic.Int32
}

func (d *failDoer) Do(*http.Request) (*http.Response, error) {
	d.calls.Add(1)
	if d.onCall != nil {
		d.onCall <- struct{}{}
	}
	return nil, d.err
}

// flakyDoer fails the first `failures` calls with err, then delegates to next.
type flakyDoer struct {
	failures int32
	err      error
	next     transport.Doer
	calls    atomic.Int32
}

func (d *flakyDoer) Do(req *http.Request) (*http.Response, error) {
	if d.calls.Add(1) <= d.failures {
		return nil, d.err
	}
	return d.next.Do(req)
}

func bodyResponse(status int, contentType, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {contentType}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

// sseHandler responds with a 200 text/event-stream body built from lines.
func sseHandler(lines ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		for _, line := range lines {
			_, _ = io.WriteString(w, line)
		}
	}
}

func newTestClient(doer transport.Doer, mutate func(*Options)) *client {
	opts := DefaultOptions()
	opts.MaxRetry = 0
	opts.RetryInterval = time.Millisecond
	if mutate != nil {
		mutate(opts)
	}
	return &client{token: "test-token", opts: opts, http: doer, assetHTTP: doer}
}

func collectEvents(t *testing.T, ch <-chan StreamEvent) []StreamEvent {
	t.Helper()
	var out []StreamEvent
	timeout := time.After(10 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-timeout:
			t.Fatal("timed out collecting stream events")
		}
	}
}

// ---------- doRequest family ----------

func TestDoRequestSendsAntiBotHeaders(t *testing.T) {
	stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "{}")
	})
	c := newTestClient(stub.doer(), nil)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"https://grok.com/rest/app-chat/upload-file", strings.NewReader(`{}`))
	require.NoError(t, err)

	resp, err := c.doRequest(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	calls := stub.calls()
	require.Len(t, calls, 1)
	assert.Equal(t, http.MethodPost, calls[0].Method)
	assert.Equal(t, "/rest/app-chat/upload-file", calls[0].Path)
	assert.Contains(t, calls[0].Header.Get("Cookie"), "sso=test-token")
	assert.Equal(t, "https://grok.com", calls[0].Header.Get("Origin"))
	assert.Equal(t, "https://grok.com/", calls[0].Header.Get("Referer"))
	assert.NotEmpty(t, calls[0].Header.Get("User-Agent"))
	assert.NotEmpty(t, calls[0].Header.Get("x-statsig-id"))
	assert.NotEmpty(t, calls[0].Header.Get("x-xai-request-id"))
}

func TestDoConsoleRequestUsesConsoleOrigin(t *testing.T) {
	stub := newStubUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "{}")
	})
	c := newTestClient(stub.doer(), nil)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"https://console.x.ai/v1/responses", strings.NewReader(`{}`))
	require.NoError(t, err)

	resp, err := c.doConsoleRequest(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	calls := stub.calls()
	require.Len(t, calls, 1)
	assert.Equal(t, "/v1/responses", calls[0].Path)
	assert.Equal(t, "https://console.x.ai", calls[0].Header.Get("Origin"))
	assert.Equal(t, "https://console.x.ai/", calls[0].Header.Get("Referer"))
}

func TestDoRequestClosedClient(t *testing.T) {
	c := newTestClient(&staticDoer{}, nil)
	require.NoError(t, c.Close())

	req := &http.Request{Method: http.MethodPost, URL: &url.URL{Scheme: "https", Host: "grok.com", Path: "/x"}}
	_, err := c.doRequest(req)
	assert.ErrorIs(t, err, ErrStreamClosed)
	_, err = c.doAssetRequest(req)
	assert.ErrorIs(t, err, ErrStreamClosed)
	_, err = c.doConsoleRequest(req)
	assert.ErrorIs(t, err, ErrStreamClosed)
}

func TestDoRequestWithNilHeader(t *testing.T) {
	c := newTestClient(&staticDoer{resp: bodyResponse(200, "application/json", "{}")}, nil)
	req := &http.Request{Method: http.MethodPost, URL: &url.URL{Scheme: "https", Host: "grok.com", Path: "/x"}}
	resp, err := c.doRequest(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.NotNil(t, req.Header)
}

func TestDoRequestMasksLongCookie(t *testing.T) {
	c := newTestClient(&staticDoer{resp: bodyResponse(200, "application/json", "{}")}, nil)
	c.token = strings.Repeat("t", 40)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"https://grok.com/x", nil)
	require.NoError(t, err)
	resp, err := c.doRequest(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestSetProxyAndResetSessionLifecycle(t *testing.T) {
	c, err := NewClient("lifecycle-token")
	require.NoError(t, err)
	impl := c.(*client)

	require.NoError(t, impl.setProxy(""))
	assert.NotNil(t, impl.http)
	assert.NotNil(t, impl.assetHTTP)
	require.NoError(t, c.ResetSession())

	require.NoError(t, c.Close())
	assert.ErrorIs(t, impl.setProxy(""), ErrStreamClosed)
	assert.ErrorIs(t, c.ResetSession(), ErrStreamClosed)
}
