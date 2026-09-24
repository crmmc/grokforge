package upstream

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStdlibDoer_StripsHeaderOrderKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Header[transport.HeaderOrderKey]; ok {
			t.Errorf("pseudo header %q must be stripped", transport.HeaderOrderKey)
		}
		if r.Header.Get("X-Test") != "v1" {
			t.Errorf("regular header not forwarded: %q", r.Header.Get("X-Test"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	req.Header.Set("X-Test", "v1")
	req.Header[transport.HeaderOrderKey] = []string{"X-Test"}

	d := &StdlibDoer{Client: srv.Client()}
	resp, err := d.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestStdlibDoer_DefaultsToHTTPDefaultClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	d := &StdlibDoer{}
	resp, err := d.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusTeapot, resp.StatusCode)
}

func TestStdlibDoer_HeaderValuesCloned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	req.Header.Set("X-Test", "v1")

	d := &StdlibDoer{Client: srv.Client()}
	resp, err := d.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// Mutating the caller's header after Do must not affect the cloned request.
	req.Header.Set("X-Test", "mutated")
	assert.NotNil(t, resp.Request)
}
