package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleGetConfigRuntime(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			AppKey: "super-secret-key",
			Port:   9090,
			DBDSN:  "postgres://user:pass@host/db",
		},
		Proxy: config.ProxyConfig{
			CFCookies:   "cf-cookie-value",
			CFClearance: "cf-clearance-value",
			Enabled:     true,
		},
	}
	runtime := config.NewRuntime(cfg)

	w := httptest.NewRecorder()
	handleGetConfigRuntime(runtime)(w, httptest.NewRequest(http.MethodGet, "/admin/config", nil))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var resp ConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, maskedConfigSecret, resp.App.AppKey, "app_key must be masked")
	assert.Equal(t, maskedConfigSecret, resp.App.DBDSN, "db_dsn must be masked")
	assert.Equal(t, maskedConfigSecret, resp.Proxy.CFCookies, "cf_cookies must be masked")
	assert.Equal(t, maskedConfigSecret, resp.Proxy.CFClearance, "cf_clearance must be masked")
	assert.Equal(t, 9090, resp.App.Port)
	assert.True(t, resp.Proxy.Enabled)
}

func TestHandleGetConfig_MasksSecrets(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{AppKey: "abcd1234", DBDSN: "secret-dsn"},
	}
	w := httptest.NewRecorder()
	handleGetConfig(cfg)(w, httptest.NewRequest(http.MethodGet, "/admin/config", nil))

	require.Equal(t, http.StatusOK, w.Code)
	var resp ConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "********", resp.App.AppKey)
	assert.Equal(t, "********", resp.App.DBDSN)
}

func TestMaskConfigSecret(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty stays empty", in: "", want: ""},
		{name: "short string masked", in: "ab", want: "********"},
		{name: "long string masked", in: "long-secret-value", want: "********"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, maskConfigSecret(tc.in))
		})
	}
}
