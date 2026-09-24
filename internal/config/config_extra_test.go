package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestLoad_EmptyPath_ReturnsDefaults(t *testing.T) {
	cfg, err := Load("")
	require.NoError(t, err)
	assert.Equal(t, DefaultConfig(), cfg)
}

func TestLoad_NonExistentFile_ReturnsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.toml")
	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, DefaultConfig(), cfg)
}

func TestLoad_MalformedTOML_ReturnsError(t *testing.T) {
	path := writeConfigFile(t, "this is ][ not toml")
	cfg, err := Load(path)
	require.Error(t, err)
	assert.Nil(t, cfg)
}

func TestLoad_AppliesFileValuesOverDefaults(t *testing.T) {
	path := writeConfigFile(t, `
[app]
app_key = "file-key"
port = 9090
request_timeout = 120

[image]
format = "local_url"
nsfw = true

[proxy]
enabled = true
flaresolverr_url = "http://fs:8191"

[retry]
max_tokens = 9

[token]
selection_algorithm = "round_robin"

[cache]
image_max_mb = 128
video_max_mb = 256

[console]
enabled = true
web_search = true
`)
	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "file-key", cfg.App.AppKey)
	assert.Equal(t, 9090, cfg.App.Port)
	assert.Equal(t, 120, cfg.App.RequestTimeout)
	assert.Equal(t, ImageFormatLocalURL, cfg.Image.Format)
	assert.True(t, cfg.Image.NSFW)
	assert.True(t, cfg.Proxy.Enabled)
	assert.Equal(t, "http://fs:8191", cfg.Proxy.FlareSolverrURL)
	assert.Equal(t, 9, cfg.Retry.MaxTokens)
	assert.Equal(t, "round_robin", cfg.Token.SelectionAlgorithm)
	assert.Equal(t, 128, cfg.Cache.ImageMaxMB)
	assert.Equal(t, 256, cfg.Cache.VideoMaxMB)
	assert.True(t, cfg.Console.Enabled)
	assert.True(t, cfg.Console.WebSearch)
	// Untouched sections keep defaults.
	assert.True(t, cfg.App.Stream)
	assert.Equal(t, 2, cfg.Retry.PerTokenRetries)
}

func TestLoad_ValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{
			name:    "negative global rate limit rpm",
			content: "[app]\nglobal_rate_limit_rpm = -1\n",
			wantErr: "app.global_rate_limit_rpm must be >= 0",
		},
		{
			name:    "positive rpm without window",
			content: "[app]\nglobal_rate_limit_rpm = 10\nglobal_rate_limit_window = 0\n",
			wantErr: "app.global_rate_limit_window must be > 0",
		},
		{
			name:    "negative recent use penalty",
			content: "[token]\nrecent_use_penalty_sec = -3\n",
			wantErr: "token.recent_use_penalty_sec must be >= 0",
		},
		{
			name:    "negative cache video limit",
			content: "[cache]\nvideo_max_mb = -2\n",
			wantErr: "cache.video_max_mb must be >= 0",
		},
		{
			name:    "invalid image format",
			content: "[image]\nformat = \"bogus\"\n",
			wantErr: "image.format must be one of",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := writeConfigFile(t, tc.content)
			cfg, err := Load(path)
			require.Error(t, err)
			assert.Nil(t, cfg)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestLoad_ZeroRateLimitRPMIsValidWithoutWindow(t *testing.T) {
	path := writeConfigFile(t, "[app]\nglobal_rate_limit_rpm = 0\nglobal_rate_limit_window = 0\n")
	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, 0, cfg.App.GlobalRateLimitRPM)
}

func TestValidateImageFormat(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		wantErr bool
	}{
		{"base64", "base64", false},
		{"local_url", "local_url", false},
		{"trimmed and lowercased", "  Local_URL  ", false},
		{"empty is rejected", "", true},
		{"unknown", "url", true},
		{"unknown mixed case", "BASE64X", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateImageFormat(tc.format)
			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "image.format must be one of")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEffectiveImageFormat(t *testing.T) {
	tests := []struct {
		name   string
		cfg    *ImageConfig
		format string
		want   string
	}{
		{"nil config defaults to base64", nil, "", ImageFormatBase64},
		{"empty format defaults to base64", &ImageConfig{Format: ""}, "", ImageFormatBase64},
		{"whitespace defaults to base64", &ImageConfig{Format: "   "}, "", ImageFormatBase64},
		{"normalized local_url", &ImageConfig{Format: " Local_URL "}, "", ImageFormatLocalURL},
		{"normalized base64", &ImageConfig{Format: "Base64"}, "", ImageFormatBase64},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, EffectiveImageFormat(tc.cfg))
		})
	}
}
