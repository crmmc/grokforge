package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxyBrowserLabel(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		want    string
	}{
		{name: "empty profile", profile: "", want: ""},
		{name: "bare chrome family", profile: "chrome", want: "Chrome"},
		{name: "chrome with version", profile: "chrome136", want: "Chrome 136"},
		{name: "firefox with version", profile: "firefox133", want: "Firefox 133"},
		{name: "safari family", profile: "safari18", want: "Safari 18"},
		{name: "unknown family passthrough", profile: "edge101", want: "edge101"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, proxyBrowserLabel(tc.profile))
		})
	}
}

func TestHandleGetProxyBrowsers_Response(t *testing.T) {
	w := httptest.NewRecorder()
	handleGetProxyBrowsers()(w, httptest.NewRequest(http.MethodGet, "/admin/proxy/browsers", nil))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var resp ProxyBrowsersResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))

	assert.NotEmpty(t, resp.DefaultBrowser)
	assert.NotEmpty(t, resp.DefaultUserAgent)
	require.NotEmpty(t, resp.Browsers)

	// Sorted ascending by browser name.
	for i := 1; i < len(resp.Browsers); i++ {
		assert.LessOrEqual(t, resp.Browsers[i-1].Browser, resp.Browsers[i].Browser)
	}

	// Every option must carry a resolved user agent and label.
	for _, b := range resp.Browsers {
		assert.NotEmpty(t, b.UserAgent, b.Browser)
		assert.NotEmpty(t, b.Label, b.Browser)
	}
}
