//go:build cgo

package transport

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/crmmc/grokforge/internal/upstream/transport/curlstub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	curlE2EOnce sync.Once
	curlE2EOkay bool
)

// disableProxyEnv removes proxy settings from the test process environment.
// libcurl honors http_proxy/https_proxy/all_proxy env vars by default, which
// would reroute test requests through a local proxy and make connection
// failures indistinguishable from successes.
func disableProxyEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"http_proxy", "HTTP_PROXY",
		"https_proxy", "HTTPS_PROXY",
		"all_proxy", "ALL_PROXY",
	} {
		t.Setenv(key, "")
	}
	t.Setenv("NO_PROXY", "*")
	t.Setenv("no_proxy", "*")
}

// requireCurlE2E skips the test when a request cannot be performed through
// libcurl in this environment. The curlstub import above normally provides
// the missing curl_easy_impersonate symbol; the probe guards environments
// where even with the stub the link does not expose it.
func requireCurlE2E(t *testing.T) {
	t.Helper()
	disableProxyEnv(t)
	curlE2EOnce.Do(func() {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()
		req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
		if err != nil {
			return
		}
		client, err := NewStatelessDoer(Options{})
		if err != nil {
			return
		}
		resp, err := client.Do(req)
		if err != nil {
			return
		}
		resp.Body.Close()
		curlE2EOkay = true
	})
	if !curlE2EOkay {
		t.Skip("curl end-to-end unavailable: libcurl request failed in this environment")
	}
}

func TestCurlClient_GetRoundTrip(t *testing.T) {
	requireCurlE2E(t)

	var (
		gotPath     string
		gotQuery    string
		gotHeader   string
		sawOrderKey bool
	)
	handlerDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotHeader = r.Header.Get("X-Test")
		_, sawOrderKey = r.Header[HeaderOrderKey]
		w.Header().Set("X-Reply", "ok")
		w.Header().Add("X-Multi", "a")
		w.Header().Add("X-Multi", "b")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/greet?q=1", nil)
	require.NoError(t, err)
	req.Header.Set("X-Test", "v1")
	req.Header[HeaderOrderKey] = []string{"X-Test"}

	client, err := newCurlClient(Options{Browser: "chrome136"})
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	<-handlerDone

	assert.Equal(t, "/greet", gotPath)
	assert.Equal(t, "q=1", gotQuery)
	assert.Equal(t, "v1", gotHeader)
	assert.False(t, sawOrderKey, "pseudo header order key must be stripped")
	assert.Equal(t, http.StatusCreated, resp.StatusCode)
	assert.Equal(t, "201 Created", resp.Status)
	assert.Equal(t, 2, resp.ProtoMajor)
	assert.Equal(t, "ok", resp.Header.Get("X-Reply"))
	assert.Equal(t, []string{"a", "b"}, resp.Header.Values("X-Multi"))
	assert.Same(t, req, resp.Request)
	assert.Equal(t, "hello", string(body))
}

func TestCurlClient_PostBody(t *testing.T) {
	requireCurlE2E(t)

	const payload = `{"message":"hi"}`
	var gotBody, gotContentType string
	handlerDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ack"))
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	client, err := newCurlClient(Options{RequestTimeout: 10 * time.Second})
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	<-handlerDone

	assert.Equal(t, payload, gotBody)
	assert.Equal(t, "application/json", gotContentType)
	assert.Equal(t, "ack", string(body))
}

func TestCurlClient_PutWithBody(t *testing.T) {
	requireCurlE2E(t)

	var gotMethod, gotBody string
	handlerDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handlerDone)
		gotMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodPut, srv.URL, strings.NewReader("update"))
	require.NoError(t, err)

	client, err := newCurlClient(Options{})
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	<-handlerDone

	assert.Equal(t, http.MethodPut, gotMethod)
	assert.Equal(t, "update", gotBody)
}

func TestCurlClient_ConnectionRefused(t *testing.T) {
	disableProxyEnv(t)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	closedPort := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())

	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:"+strconv.Itoa(closedPort)+"/", nil)
	require.NoError(t, err)

	client, err := NewStatelessDoer(Options{})
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "curl")
	if resp != nil {
		resp.Body.Close()
	}
}

func TestCurlClient_RequestTimeout(t *testing.T) {
	requireCurlE2E(t)

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	defer close(release)

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	client, err := newCurlClient(Options{RequestTimeout: 200 * time.Millisecond})
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.Error(t, err, "request should time out")
	if resp != nil {
		resp.Body.Close()
	}
}

func TestCurlClient_ContextCanceled(t *testing.T) {
	disableProxyEnv(t)

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	client, err := newCurlClient(Options{})
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.ErrorIs(t, err, context.Canceled)
	if resp != nil {
		resp.Body.Close()
	}
}

func TestDynamicStatelessDoer_Do(t *testing.T) {
	requireCurlE2E(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := NewDynamicStatelessDoer(func() Options { return Options{} })
	for i := 0; i < 2; i++ {
		req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
		require.NoError(t, err)
		resp, err := d.Do(req)
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		resp.Body.Close()
	}
}

func TestGoCurlCallbacks_UnknownHandle(t *testing.T) {
	assert.EqualValues(t, 0, gfGoCurlWrite(0, nil, 0))
	assert.EqualValues(t, 0, gfGoCurlHeader(0, nil, 0))
}
