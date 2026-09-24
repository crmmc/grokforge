package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// applyOne applies a single DB override key/value on a fresh default config.
func applyOne(t *testing.T, key, value string) (*Config, error) {
	t.Helper()
	cfg := DefaultConfig()
	err := cfg.ApplyDBOverrides(map[string]string{key: value})
	return cfg, err
}

func TestApplyDBOverrides_HappyPath(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		value  string
		verify func(t *testing.T, cfg *Config)
	}{
		{"app key", "app.app_key", "secret-key", func(t *testing.T, c *Config) { assert.Equal(t, "secret-key", c.App.AppKey) }},
		{"media generation disabled", "app.media_generation_enabled", "false", func(t *testing.T, c *Config) { assert.False(t, c.App.MediaGenerationEnabled) }},
		{"temporary off", "app.temporary", "false", func(t *testing.T, c *Config) { assert.False(t, c.App.Temporary) }},
		{"stream off", "app.stream", "false", func(t *testing.T, c *Config) { assert.False(t, c.App.Stream) }},
		{"thinking off", "app.thinking", "false", func(t *testing.T, c *Config) { assert.False(t, c.App.Thinking) }},
		{"dynamic statsig off", "app.dynamic_statsig", "false", func(t *testing.T, c *Config) { assert.False(t, c.App.DynamicStatsig) }},
		{"custom instruction", "app.custom_instruction", "be concise", func(t *testing.T, c *Config) { assert.Equal(t, "be concise", c.App.CustomInstruction) }},
		{"disable memory off", "app.disable_memory", "false", func(t *testing.T, c *Config) { assert.False(t, c.App.DisableMemory) }},
		{"request timeout", "app.request_timeout", "120", func(t *testing.T, c *Config) { assert.Equal(t, 120, c.App.RequestTimeout) }},
		{"read header timeout", "app.read_header_timeout", "30", func(t *testing.T, c *Config) { assert.Equal(t, 30, c.App.ReadHeaderTimeout) }},
		{"max header bytes", "app.max_header_bytes", "4096", func(t *testing.T, c *Config) { assert.Equal(t, int64(4096), int64(c.App.MaxHeaderBytes)) }},
		{"body limit", "app.body_limit", "4096", func(t *testing.T, c *Config) { assert.Equal(t, int64(4096), c.App.BodyLimit) }},
		{"chat body limit", "app.chat_body_limit", "8192", func(t *testing.T, c *Config) { assert.Equal(t, int64(8192), c.App.ChatBodyLimit) }},
		{"admin max fails", "app.admin_max_fails", "3", func(t *testing.T, c *Config) { assert.Equal(t, 3, c.App.AdminMaxFails) }},
		{"admin window sec", "app.admin_window_sec", "60", func(t *testing.T, c *Config) { assert.Equal(t, 60, c.App.AdminWindowSec) }},
		{"global rate limit rpm", "app.global_rate_limit_rpm", "100", func(t *testing.T, c *Config) { assert.Equal(t, 100, c.App.GlobalRateLimitRPM) }},
		{"global rate limit window", "app.global_rate_limit_window", "30", func(t *testing.T, c *Config) { assert.Equal(t, 30, c.App.GlobalRateLimitWindow) }},
		{"base proxy url", "proxy.base_proxy_url", "http://base:1", func(t *testing.T, c *Config) { assert.Equal(t, "http://base:1", c.Proxy.BaseProxyURL) }},
		{"asset proxy url", "proxy.asset_proxy_url", "http://asset:2", func(t *testing.T, c *Config) { assert.Equal(t, "http://asset:2", c.Proxy.AssetProxyURL) }},
		{"cf cookies", "proxy.cf_cookies", "a=b; c=d", func(t *testing.T, c *Config) { assert.Equal(t, "a=b; c=d", c.Proxy.CFCookies) }},
		{"skip proxy ssl verify", "proxy.skip_proxy_ssl_verify", "true", func(t *testing.T, c *Config) { assert.True(t, c.Proxy.SkipProxySSLVerify) }},
		{"proxy enabled", "proxy.enabled", "true", func(t *testing.T, c *Config) { assert.True(t, c.Proxy.Enabled) }},
		{"flaresolverr url", "proxy.flaresolverr_url", "http://fs:8191", func(t *testing.T, c *Config) { assert.Equal(t, "http://fs:8191", c.Proxy.FlareSolverrURL) }},
		{"refresh interval", "proxy.refresh_interval", "120", func(t *testing.T, c *Config) { assert.Equal(t, 120, c.Proxy.RefreshInterval) }},
		{"proxy timeout", "proxy.timeout", "90", func(t *testing.T, c *Config) { assert.Equal(t, 90, c.Proxy.Timeout) }},
		{"cf clearance", "proxy.cf_clearance", "clear-1", func(t *testing.T, c *Config) { assert.Equal(t, "clear-1", c.Proxy.CFClearance) }},
		{"browser", "proxy.browser", "chrome136", func(t *testing.T, c *Config) { assert.Equal(t, "chrome136", c.Proxy.Browser) }},
		{"user agent", "proxy.user_agent", "UA/1.0", func(t *testing.T, c *Config) { assert.Equal(t, "UA/1.0", c.Proxy.UserAgent) }},
		{"retry max tokens", "retry.max_tokens", "9", func(t *testing.T, c *Config) { assert.Equal(t, 9, c.Retry.MaxTokens) }},
		{"per token retries", "retry.per_token_retries", "4", func(t *testing.T, c *Config) { assert.Equal(t, 4, c.Retry.PerTokenRetries) }},
		{"backoff base", "retry.retry_backoff_base", "1.5", func(t *testing.T, c *Config) { assert.Equal(t, 1.5, c.Retry.RetryBackoffBase) }},
		{"backoff factor", "retry.retry_backoff_factor", "3.5", func(t *testing.T, c *Config) { assert.Equal(t, 3.5, c.Retry.RetryBackoffFactor) }},
		{"backoff max", "retry.retry_backoff_max", "30.5", func(t *testing.T, c *Config) { assert.Equal(t, 30.5, c.Retry.RetryBackoffMax) }},
		{"retry budget", "retry.retry_budget", "90.5", func(t *testing.T, c *Config) { assert.Equal(t, 90.5, c.Retry.RetryBudget) }},
		{"image nsfw", "image.nsfw", "true", func(t *testing.T, c *Config) { assert.True(t, c.Image.NSFW) }},
		{"image format normalized", "image.format", " Local_URL ", func(t *testing.T, c *Config) { assert.Equal(t, ImageFormatLocalURL, c.Image.Format) }},
		{"blocked parallel attempts", "image.blocked_parallel_attempts", "7", func(t *testing.T, c *Config) { assert.Equal(t, 7, c.Image.BlockedParallelAttempts) }},
		{"fail threshold", "token.fail_threshold", "3", func(t *testing.T, c *Config) { assert.Equal(t, 3, c.Token.FailThreshold) }},
		{"usage flush interval", "token.usage_flush_interval_sec", "45", func(t *testing.T, c *Config) { assert.Equal(t, 45, c.Token.UsageFlushIntervalSec) }},
		{"selection algorithm", "token.selection_algorithm", "random", func(t *testing.T, c *Config) { assert.Equal(t, "random", c.Token.SelectionAlgorithm) }},
		{"max inflight", "token.max_inflight", "16", func(t *testing.T, c *Config) { assert.Equal(t, 16, c.Token.MaxInflight) }},
		{"recent use penalty", "token.recent_use_penalty_sec", "30", func(t *testing.T, c *Config) { assert.Equal(t, 30, c.Token.RecentUsePenaltySec) }},
		{"cache image limit", "cache.image_max_mb", "128", func(t *testing.T, c *Config) { assert.Equal(t, 128, c.Cache.ImageMaxMB) }},
		{"cache video limit", "cache.video_max_mb", "256", func(t *testing.T, c *Config) { assert.Equal(t, 256, c.Cache.VideoMaxMB) }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := applyOne(t, tc.key, tc.value)
			require.NoError(t, err)
			tc.verify(t, cfg)
		})
	}
}

func TestApplyDBOverrides_ListValues(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  []string
	}{
		{"filter tags with spaces", "app.filter_tags", "alpha, beta", []string{"alpha", "beta"}},
		{"filter tags single", "app.filter_tags", "solo", []string{"solo"}},
		{"filter tags empty clears", "app.filter_tags", "", []string{}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := applyOne(t, tc.key, tc.value)
			require.NoError(t, err)
			assert.Equal(t, tc.want, cfg.App.FilterTags)
		})
	}

	intTests := []struct {
		name  string
		value string
		want  []int
	}{
		{"status codes with spaces", "403, 429", []int{403, 429}},
		{"status codes single", "500", []int{500}},
		{"status codes empty clears", "", []int{}},
	}
	for _, tc := range intTests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := applyOne(t, "retry.reset_session_status_codes", tc.value)
			require.NoError(t, err)
			assert.Equal(t, tc.want, cfg.Retry.ResetSessionStatusCodes)
		})
	}
}

func TestApplyDBOverrides_ListValueErrors(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{"filter tags empty item", "app.filter_tags", "a,,b"},
		{"reset codes empty item", "retry.reset_session_status_codes", "403,,429"},
		{"reset codes non numeric", "retry.reset_session_status_codes", "403,abc"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := applyOne(t, tc.key, tc.value)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid")
		})
	}
}

func TestApplyDBOverrides_RangeValidation(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantErr string
	}{
		{"negative rpm", "app.global_rate_limit_rpm", "-1", "app.global_rate_limit_rpm must be >= 0"},
		{"zero window with parse ok", "app.global_rate_limit_window", "0", "app.global_rate_limit_window must be > 0"},
		{"negative penalty", "token.recent_use_penalty_sec", "-2", "token.recent_use_penalty_sec must be >= 0"},
		{"negative cache image", "cache.image_max_mb", "-1", "cache.image_max_mb must be >= 0"},
		{"negative cache video", "cache.video_max_mb", "-1", "cache.video_max_mb must be >= 0"},
		{"invalid selection algorithm", "token.selection_algorithm", "bogus", `invalid value "bogus" for token.selection_algorithm`},
		{"invalid image format", "image.format", "url", "image.format must be one of"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := applyOne(t, tc.key, tc.value)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestApplyDBOverrides_ParseErrorPerKey(t *testing.T) {
	boolKeys := []string{
		"app.media_generation_enabled", "app.temporary", "app.stream", "app.thinking",
		"app.dynamic_statsig", "app.disable_memory", "proxy.skip_proxy_ssl_verify",
		"proxy.enabled", "image.nsfw", "image.blocked_parallel_enabled",
		"console.enabled", "console.web_search",
	}
	intKeys := []string{
		"app.request_timeout", "app.read_header_timeout", "app.max_header_bytes",
		"app.admin_max_fails", "app.admin_window_sec", "app.global_rate_limit_rpm",
		"app.global_rate_limit_window", "proxy.refresh_interval", "proxy.timeout",
		"retry.max_tokens", "retry.per_token_retries", "image.blocked_parallel_attempts",
		"token.fail_threshold", "token.usage_flush_interval_sec", "token.max_inflight",
		"token.recent_use_penalty_sec", "cache.image_max_mb", "cache.video_max_mb",
	}
	int64Keys := []string{"app.body_limit", "app.chat_body_limit"}
	floatKeys := []string{
		"retry.retry_backoff_base", "retry.retry_backoff_factor",
		"retry.retry_backoff_max", "retry.retry_budget",
	}

	sweep := []struct {
		name  string
		keys  []string
		value string
		want  string
	}{
		{"bool keys", boolKeys, "not-a-bool", "invalid boolean"},
		{"int keys", intKeys, "not-an-int", "invalid integer"},
		{"int64 keys", int64Keys, "not-an-int64", "invalid int64"},
		{"float keys", floatKeys, "not-a-float", "invalid float"},
	}

	for _, s := range sweep {
		t.Run(s.name, func(t *testing.T) {
			for _, key := range s.keys {
				key := key
				t.Run(key, func(t *testing.T) {
					_, err := applyOne(t, key, s.value)
					require.Error(t, err, "expected parse error for %s", key)
					assert.Contains(t, err.Error(), s.want)
					assert.Contains(t, err.Error(), key)
				})
			}
		})
	}
}

func TestApplyDBOverrides_ProxyOverridesMustNotBeBlank(t *testing.T) {
	tests := []struct {
		name string
		kvs  map[string]string
		want string
	}{
		{"blank browser", map[string]string{"proxy.browser": "   "}, "proxy.browser override cannot be empty"},
		{"empty user agent", map[string]string{"proxy.user_agent": ""}, "proxy.user_agent override cannot be empty"},
		{"both blank", map[string]string{"proxy.browser": "", "proxy.user_agent": " "}, "proxy.browser override cannot be empty"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			err := cfg.ApplyDBOverrides(tc.kvs)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestApplyDBOverrides_MultipleKeysApplyTogether(t *testing.T) {
	cfg := DefaultConfig()
	err := cfg.ApplyDBOverrides(map[string]string{
		"proxy.enabled":          "true",
		"proxy.flaresolverr_url": "http://fs:8191",
		"console.enabled":        "true",
		"app.app_key":            "multi-key",
	})
	require.NoError(t, err)
	assert.True(t, cfg.Proxy.Enabled)
	assert.Equal(t, "http://fs:8191", cfg.Proxy.FlareSolverrURL)
	assert.True(t, cfg.Console.Enabled)
	assert.Equal(t, "multi-key", cfg.App.AppKey)
}
