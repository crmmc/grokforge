package transport

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewStatelessDoer_ResolvesBrowserOrDefault(t *testing.T) {
	tests := []struct {
		name        string
		browser     string
		wantBrowser string
	}{
		{"supported profile kept", "Firefox-135", "firefox135"},
		{"unsupported profile falls back to default", "edge120", DefaultProfile},
		{"empty falls back to default", "", DefaultProfile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewStatelessDoer(Options{Browser: tt.browser})
			require.NoError(t, err)
			curl, ok := client.(*curlClient)
			require.True(t, ok, "NewStatelessDoer should return *curlClient")
			assert.Equal(t, tt.wantBrowser, curl.opts.Browser)
		})
	}

	t.Run("options preserved", func(t *testing.T) {
		opts := Options{
			RequestTimeout:     3210 * time.Millisecond,
			ProxyURL:           "http://127.0.0.1:9",
			SkipProxySSLVerify: true,
		}
		client, err := NewStatelessDoer(opts)
		require.NoError(t, err)
		curl := client.(*curlClient)
		assert.Equal(t, opts.RequestTimeout, curl.opts.RequestTimeout)
		assert.Equal(t, opts.ProxyURL, curl.opts.ProxyURL)
		assert.True(t, curl.opts.SkipProxySSLVerify)
	})
}

func TestCurlClient_Do_NilRequest(t *testing.T) {
	client, err := newCurlClient(Options{})
	require.NoError(t, err)

	resp, err := client.Do(nil)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "nil request")
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

type trackingBody struct {
	io.Reader
	closed bool
}

func (b *trackingBody) Close() error {
	b.closed = true
	return nil
}

func TestReadRequestBody(t *testing.T) {
	t.Run("nil body", func(t *testing.T) {
		body, err := readRequestBody(nil)
		require.NoError(t, err)
		assert.Nil(t, body)
	})

	t.Run("reads and closes", func(t *testing.T) {
		tb := &trackingBody{Reader: io.NopCloser(strings.NewReader("payload"))}
		body, err := readRequestBody(tb)
		require.NoError(t, err)
		assert.Equal(t, []byte("payload"), body)
		assert.True(t, tb.closed, "body must be closed after reading")
	})

	t.Run("read error propagates", func(t *testing.T) {
		wantErr := errors.New("boom")
		_, err := readRequestBody(&trackingBody{Reader: errReader{err: wantErr}})
		require.ErrorIs(t, err, wantErr)
	})
}

func TestOrderedHeaders(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
		want   []string
	}{
		{"nil header", nil, nil},
		{"empty header", http.Header{}, []string{}},
		{
			"keys sorted without order hint",
			http.Header{"B": {"2"}, "A": {"1"}},
			[]string{"A: 1", "B: 2"},
		},
		{
			"multiple values keep order",
			http.Header{"A": {"1", "2"}},
			[]string{"A: 1", "A: 2"},
		},
		{
			"order hint respected and pseudo header stripped",
			http.Header{
				HeaderOrderKey: {"X-B", "X-A"},
				"X-A":          {"a"},
				"X-B":          {"b"},
			},
			[]string{"X-B: b", "X-A: a"},
		},
		{
			"order hint canonicalizes keys",
			http.Header{
				HeaderOrderKey: {"x-a"},
				"X-A":          {"a"},
			},
			[]string{"X-A: a"},
		},
		{
			"order hint falls back to raw stored key",
			http.Header{
				HeaderOrderKey: {"x-lower"},
				"x-lower":      {"v"},
			},
			[]string{"x-lower: v"},
		},
		{
			"order hint ignores unknown keys",
			http.Header{
				HeaderOrderKey: {"X-Missing", "X-A"},
				"X-A":          {"a"},
			},
			[]string{"X-A: a"},
		},
		{
			"remaining keys sorted after hinted ones",
			http.Header{
				HeaderOrderKey: {"X-Z"},
				"X-Z":          {"z"},
				"X-A":          {"a"},
				"X-M":          {"m"},
			},
			[]string{"X-Z: z", "X-A: a", "X-M: m"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, orderedHeaders(tt.header))
		})
	}
}

func TestResponseFromParts(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://example.test/", nil)
	require.NoError(t, err)

	t.Run("with header and status", func(t *testing.T) {
		header := http.Header{"X-A": {"1"}}
		resp := responseFromParts(req, 201, header, nil)
		assert.Equal(t, 201, resp.StatusCode)
		assert.Equal(t, "201 Created", resp.Status)
		assert.Equal(t, 2, resp.ProtoMajor)
		assert.Equal(t, "HTTP/2.0", resp.Proto)
		assert.Equal(t, header, resp.Header)
		assert.Nil(t, resp.Body)
		assert.Same(t, req, resp.Request)
	})

	t.Run("zero status renders as 0", func(t *testing.T) {
		resp := responseFromParts(req, 0, nil, nil)
		assert.Equal(t, 0, resp.StatusCode)
		assert.Equal(t, "0", resp.Status)
	})

	t.Run("nil header replaced with empty header", func(t *testing.T) {
		resp := responseFromParts(req, 200, nil, nil)
		require.NotNil(t, resp.Header)
		assert.Empty(t, resp.Header)
	})
}

func TestBufferedErrorBody(t *testing.T) {
	body := bufferedErrorBody("something failed")
	data, err := io.ReadAll(body)
	require.NoError(t, err)
	assert.Equal(t, "something failed", string(data))
	assert.NoError(t, body.Close())
}

func TestFakeDoer_PassesThrough(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://example.test/", nil)
	require.NoError(t, err)
	wantResp := &http.Response{StatusCode: http.StatusTeapot}
	wantErr := errors.New("upstream down")

	fake := FakeDoer{DoFunc: func(r *http.Request) (*http.Response, error) {
		assert.Same(t, req, r)
		return wantResp, wantErr
	}}

	gotResp, gotErr := fake.Do(req)
	assert.Same(t, wantResp, gotResp)
	assert.ErrorIs(t, gotErr, wantErr)
}

func TestDynamicStatelessDoer_Do_NilProvider(t *testing.T) {
	d := NewDynamicStatelessDoer(nil)
	req, err := http.NewRequest(http.MethodGet, "http://example.test/", nil)
	require.NoError(t, err)

	resp, err := d.Do(req)
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "nil options provider")
}

func TestDynamicStatelessDoer_Do_UsesProviderOptions(t *testing.T) {
	var gotHeader string
	handlerDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)
		gotHeader = r.Header.Get("X-Test")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewDynamicStatelessDoer(func() Options { return Options{} })
	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	req.Header.Set("X-Test", "v1")

	resp, err := d.Do(req)
	require.NoError(t, err)
	if resp != nil {
		resp.Body.Close()
	}
	<-handlerDone
	assert.Equal(t, "v1", gotHeader)
}
