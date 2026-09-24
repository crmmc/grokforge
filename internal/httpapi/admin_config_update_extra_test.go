package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestConfigStore returns a ConfigStore backed by a migrated SQLite database.
func newTestConfigStore(t *testing.T) *store.ConfigStore {
	t.Helper()
	db, err := store.OpenSQLite(filepath.Join(t.TempDir(), "config.db"))
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&store.ConfigEntry{}))
	return store.NewConfigStore(db)
}

// newBrokenConfigStore returns a ConfigStore whose table was never migrated,
// forcing SetMany to fail.
func newBrokenConfigStore(t *testing.T) *store.ConfigStore {
	t.Helper()
	db, err := store.OpenSQLite(filepath.Join(t.TempDir(), "broken.db"))
	require.NoError(t, err)
	return store.NewConfigStore(db)
}

func TestHandlePutConfig_FullUpdateWithPersistence(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.App.AppKey = "original-key"
	configStore := newTestConfigStore(t)

	body := `{
		"app": {
			"app_key": "new-key",
			"media_generation_enabled": true,
			"request_timeout": 42,
			"temporary": true,
			"stream": false,
			"thinking": true,
			"dynamic_statsig": true,
			"custom_instruction": "be nice",
			"filter_tags": [" a ", "", "b"],
			"disable_memory": true,
			"read_header_timeout": 5,
			"max_header_bytes": 8192,
			"body_limit": 1048576,
			"chat_body_limit": 2097152,
			"admin_max_fails": 7,
			"admin_window_sec": 300,
			"global_rate_limit_rpm": 120,
			"global_rate_limit_window": 60
		},
		"image": {
			"nsfw": true,
			"format": "local_url",
			"blocked_parallel_attempts": 3,
			"blocked_parallel_enabled": true
		},
		"proxy": {
			"base_proxy_url": "http://proxy.local",
			"asset_proxy_url": "http://asset.local",
			"cf_cookies": "cookie-value",
			"skip_proxy_ssl_verify": true,
			"enabled": true,
			"flaresolverr_url": "http://flare.local",
			"refresh_interval": 600,
			"timeout": 33,
			"cf_clearance": "clearance-value",
			"browser": "chrome136",
			"user_agent": "custom-ua"
		},
		"retry": {
			"max_tokens": 500,
			"per_token_retries": 2,
			"reset_session_status_codes": [401, 403],
			"retry_backoff_base": 1.5,
			"retry_backoff_factor": 2.0,
			"retry_backoff_max": 30.0,
			"retry_budget": 60.0
		},
		"token": {
			"fail_threshold": 4,
			"usage_flush_interval_sec": 30,
			"selection_algorithm": "round_robin",
			"recent_use_penalty_sec": 120
		},
		"cache": {
			"image_max_mb": 256,
			"video_max_mb": 512
		},
		"console": {
			"enabled": true,
			"web_search": true
		}
	}`

	w := httptest.NewRecorder()
	handlePutConfig(cfg, configStore)(w, httptest.NewRequest(http.MethodPut, "/admin/config", strings.NewReader(body)))

	require.Equal(t, http.StatusOK, w.Code)

	assert.Equal(t, "new-key", cfg.App.AppKey)
	assert.True(t, cfg.App.MediaGenerationEnabled)
	assert.Equal(t, 42, cfg.App.RequestTimeout)
	assert.True(t, cfg.App.Temporary)
	assert.False(t, cfg.App.Stream)
	assert.True(t, cfg.App.Thinking)
	assert.True(t, cfg.App.DynamicStatsig)
	assert.Equal(t, "be nice", cfg.App.CustomInstruction)
	assert.Equal(t, []string{"a", "b"}, cfg.App.FilterTags)
	assert.True(t, cfg.App.DisableMemory)
	assert.Equal(t, 5, cfg.App.ReadHeaderTimeout)
	assert.Equal(t, 8192, cfg.App.MaxHeaderBytes)
	assert.Equal(t, int64(1048576), cfg.App.BodyLimit)
	assert.Equal(t, int64(2097152), cfg.App.ChatBodyLimit)
	assert.Equal(t, 7, cfg.App.AdminMaxFails)
	assert.Equal(t, 300, cfg.App.AdminWindowSec)
	assert.Equal(t, 120, cfg.App.GlobalRateLimitRPM)
	assert.Equal(t, 60, cfg.App.GlobalRateLimitWindow)

	assert.True(t, cfg.Image.NSFW)
	assert.Equal(t, "local_url", cfg.Image.Format)
	assert.Equal(t, 3, cfg.Image.BlockedParallelAttempts)
	require.NotNil(t, cfg.Image.BlockedParallelEnabled)
	assert.True(t, *cfg.Image.BlockedParallelEnabled)

	assert.Equal(t, "http://proxy.local", cfg.Proxy.BaseProxyURL)
	assert.Equal(t, "http://asset.local", cfg.Proxy.AssetProxyURL)
	assert.Equal(t, "cookie-value", cfg.Proxy.CFCookies)
	assert.True(t, cfg.Proxy.SkipProxySSLVerify)
	assert.True(t, cfg.Proxy.Enabled)
	assert.Equal(t, "http://flare.local", cfg.Proxy.FlareSolverrURL)
	assert.Equal(t, 600, cfg.Proxy.RefreshInterval)
	assert.Equal(t, 33, cfg.Proxy.Timeout)
	assert.Equal(t, "clearance-value", cfg.Proxy.CFClearance)
	assert.Equal(t, "chrome136", cfg.Proxy.Browser)
	assert.Equal(t, "custom-ua", cfg.Proxy.UserAgent)

	assert.Equal(t, 500, cfg.Retry.MaxTokens)
	assert.Equal(t, 2, cfg.Retry.PerTokenRetries)
	assert.Equal(t, []int{401, 403}, cfg.Retry.ResetSessionStatusCodes)
	assert.Equal(t, 1.5, cfg.Retry.RetryBackoffBase)
	assert.Equal(t, 2.0, cfg.Retry.RetryBackoffFactor)
	assert.Equal(t, 30.0, cfg.Retry.RetryBackoffMax)
	assert.Equal(t, 60.0, cfg.Retry.RetryBudget)

	assert.Equal(t, 4, cfg.Token.FailThreshold)
	assert.Equal(t, 30, cfg.Token.UsageFlushIntervalSec)
	assert.Equal(t, "round_robin", cfg.Token.SelectionAlgorithm)
	assert.Equal(t, 120, cfg.Token.RecentUsePenaltySec)

	assert.Equal(t, 256, cfg.Cache.ImageMaxMB)
	assert.Equal(t, 512, cfg.Cache.VideoMaxMB)

	assert.True(t, cfg.Console.Enabled)
	assert.True(t, cfg.Console.WebSearch)

	// Persisted keys should include representative values from every section.
	for _, key := range []string{
		"app.app_key", "app.media_generation_enabled", "app.request_timeout", "app.filter_tags",
		"image.nsfw", "image.format",
		"proxy.base_proxy_url", "proxy.user_agent",
		"retry.max_tokens", "retry.reset_session_status_codes",
		"token.selection_algorithm",
		"cache.image_max_mb",
		"console.enabled",
	} {
		val, err := configStore.Get(key)
		require.NoError(t, err, key)
		assert.NotEmpty(t, val, key)
	}
	assert.Equal(t, "new-key", mustGet(t, configStore, "app.app_key"))
	assert.Equal(t, "a,b", mustGet(t, configStore, "app.filter_tags"))
	assert.Equal(t, "401,403", mustGet(t, configStore, "retry.reset_session_status_codes"))
}

func mustGet(t *testing.T, s *store.ConfigStore, key string) string {
	t.Helper()
	val, err := s.Get(key)
	require.NoError(t, err)
	return val
}

func TestHandlePutConfig_ValidationFailures(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantCode  int
		wantCode2 string
	}{
		{name: "invalid selection algorithm", body: `{"token":{"selection_algorithm":"chaos"}}`, wantCode: http.StatusBadRequest},
		{name: "negative recent use penalty", body: `{"token":{"recent_use_penalty_sec":-1}}`, wantCode: http.StatusBadRequest},
		{name: "negative image max mb", body: `{"cache":{"image_max_mb":-1}}`, wantCode: http.StatusBadRequest},
		{name: "negative video max mb", body: `{"cache":{"video_max_mb":-1}}`, wantCode: http.StatusBadRequest},
		{name: "invalid image format", body: `{"image":{"format":"bmp"}}`, wantCode: http.StatusBadRequest},
		{name: "unsupported proxy browser", body: `{"proxy":{"browser":"netscape"}}`, wantCode: http.StatusBadRequest},
		{name: "empty app key", body: `{"app":{"app_key":""}}`, wantCode: http.StatusBadRequest},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			w := httptest.NewRecorder()
			handlePutConfig(cfg, nil)(w, httptest.NewRequest(http.MethodPut, "/admin/config", strings.NewReader(tc.body)))
			require.Equal(t, tc.wantCode, w.Code)
		})
	}
}

func TestHandlePutConfig_PersistFailureReturns500(t *testing.T) {
	cfg := config.DefaultConfig()
	configStore := newBrokenConfigStore(t)

	w := httptest.NewRecorder()
	handlePutConfig(cfg, configStore)(w, httptest.NewRequest(http.MethodPut, "/admin/config", strings.NewReader(`{"console":{"enabled":true}}`)))

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "config_persist_failed")
}

func TestHandlePutConfig_NilConfigStoreSkipsPersist(t *testing.T) {
	cfg := config.DefaultConfig()
	w := httptest.NewRecorder()
	handlePutConfig(cfg, nil)(w, httptest.NewRequest(http.MethodPut, "/admin/config", strings.NewReader(`{"console":{"enabled":true}}`)))

	require.Equal(t, http.StatusOK, w.Code)
	assert.True(t, cfg.Console.Enabled)
}

func TestHandlePutConfig_EmptyBodyStillReturnsConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.App.AppKey = "keep-me"
	configStore := newTestConfigStore(t)

	w := httptest.NewRecorder()
	handlePutConfig(cfg, configStore)(w, httptest.NewRequest(http.MethodPut, "/admin/config", strings.NewReader(`{}`)))

	require.Equal(t, http.StatusOK, w.Code)
	var resp ConfigResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, maskedConfigSecret, resp.App.AppKey)
}

func TestHandlePutConfig_MaskedSecretsIgnoredInDbUpdates(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Proxy.CFCookies = "real-cookie"
	configStore := newTestConfigStore(t)

	body := `{"proxy":{"cf_cookies":"********","cf_clearance":"********"},"app":{"app_key":"********"}}`
	w := httptest.NewRecorder()
	handlePutConfig(cfg, configStore)(w, httptest.NewRequest(http.MethodPut, "/admin/config", strings.NewReader(body)))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "real-cookie", cfg.Proxy.CFCookies)
	_, err := configStore.Get("proxy.cf_cookies")
	require.NoError(t, err)
	assert.Empty(t, mustGet(t, configStore, "proxy.cf_cookies"), "masked placeholder must not be persisted")
}

func TestHandlePutConfig_ProxyBrowserAutoPairsUA(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Proxy.Browser = "chrome136"
	cfg.Proxy.UserAgent = "old-ua"

	w := httptest.NewRecorder()
	handlePutConfig(cfg, nil)(w, httptest.NewRequest(http.MethodPut, "/admin/config", strings.NewReader(`{"proxy":{"browser":"firefox133"}}`)))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "firefox133", cfg.Proxy.Browser)
	assert.NotEqual(t, "old-ua", cfg.Proxy.UserAgent, "UA should be re-paired with the browser profile")
}

func TestHandlePutConfigRuntime_PersistsViaConfigStore(t *testing.T) {
	runtime := config.NewRuntime(config.DefaultConfig())
	configStore := newTestConfigStore(t)

	var syncedCfg *config.TokenConfig
	w := httptest.NewRecorder()
	handler := handlePutConfigRuntime(runtime, configStore, func(cfg *config.Config) {
		syncedCfg = &cfg.Token
	})
	handler(w, httptest.NewRequest(http.MethodPut, "/admin/config", strings.NewReader(`{"token":{"fail_threshold":9}}`)))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, 9, runtime.Get().Token.FailThreshold)
	require.NotNil(t, syncedCfg)
	assert.Equal(t, 9, syncedCfg.FailThreshold)
	assert.Equal(t, "9", mustGet(t, configStore, "token.fail_threshold"))
}

func TestHandlePutConfigRuntime_FailureDoesNotStore(t *testing.T) {
	runtime := config.NewRuntime(config.DefaultConfig())
	before := runtime.Get().Token.FailThreshold

	w := httptest.NewRecorder()
	handler := handlePutConfigRuntime(runtime, nil, nil)
	handler(w, httptest.NewRequest(http.MethodPut, "/admin/config", strings.NewReader(`{"token":{"selection_algorithm":"bogus"}}`)))

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Equal(t, before, runtime.Get().Token.FailThreshold)
}

func TestFilterEmptyStrings(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "nil input yields empty slice", in: nil, want: []string{}},
		{name: "trims and drops empties", in: []string{" a ", "", "  ", "b"}, want: []string{"a", "b"}},
		{name: "all empty", in: []string{"", "   "}, want: []string{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, filterEmptyStrings(tc.in))
		})
	}
}

func TestValidateImageFormatUpdate_NilSections(t *testing.T) {
	current := "png"

	t.Run("nil image section keeps current", func(t *testing.T) {
		got, err := validateImageFormatUpdate(&ConfigUpdateRequest{}, current)
		require.NoError(t, err)
		assert.Equal(t, current, got)
	})

	t.Run("nil format keeps current", func(t *testing.T) {
		got, err := validateImageFormatUpdate(&ConfigUpdateRequest{Image: &ImageConfigUpdate{}}, current)
		require.NoError(t, err)
		assert.Equal(t, current, got)
	})

	t.Run("format trimmed and lowercased", func(t *testing.T) {
		got, err := validateImageFormatUpdate(&ConfigUpdateRequest{Image: &ImageConfigUpdate{Format: strPtr("  LOCAL_URL ")}}, current)
		require.NoError(t, err)
		assert.Equal(t, "local_url", got)
	})
}

func TestValidateProxyBrowserUpdate_NilSections(t *testing.T) {
	browser, ua, err := validateProxyBrowserUpdate(&ConfigUpdateRequest{})
	require.NoError(t, err)
	assert.Empty(t, browser)
	assert.Empty(t, ua)

	browser, ua, err = validateProxyBrowserUpdate(&ConfigUpdateRequest{Proxy: &ProxyConfigUpdate{}})
	require.NoError(t, err)
	assert.Empty(t, browser)
	assert.Empty(t, ua)
}

// strPtr is a local helper for pointer literals in config update tests.
func strPtr(s string) *string { return &s }
