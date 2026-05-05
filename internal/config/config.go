// Package config provides configuration loading and management.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

var deprecatedConfigKeys = map[string]struct{}{
	"token.cool_duration_basic_sec": {},
	"token.cool_duration_super_sec": {},
	"token.cool_duration_heavy_sec": {},
}

const (
	ImageFormatBase64   = "base64"
	ImageFormatLocalURL = "local_url"
)

// Config is the root configuration structure.
type Config struct {
	App   AppConfig   `toml:"app"`
	Image ImageConfig `toml:"image"`
	Proxy ProxyConfig `toml:"proxy"`
	Retry RetryConfig `toml:"retry"`
	Token TokenConfig `toml:"token"`
	Cache CacheConfig `toml:"cache"`
}

// AppConfig contains application settings.
type AppConfig struct {
	AppKey                 string   `toml:"app_key"`
	MediaGenerationEnabled bool     `toml:"media_generation_enabled"`
	Temporary              bool     `toml:"temporary"`
	DisableMemory          bool     `toml:"disable_memory"`
	Stream                 bool     `toml:"stream"`
	Thinking               bool     `toml:"thinking"`
	DynamicStatsig         bool     `toml:"dynamic_statsig"`
	CustomInstruction      string   `toml:"custom_instruction"`
	FilterTags             []string `toml:"filter_tags"`
	// Model catalog settings
	ModelsFile string `toml:"models_file"` // optional external model catalog file path
	// Server settings
	Host          string `toml:"host"`
	Port          int    `toml:"port"`
	LogJSON       bool   `toml:"log_json"`
	LogLevel      string `toml:"log_level"`
	LogFilePath   string `toml:"log_file_path"`
	LogMaxSizeMB  int    `toml:"log_max_size_mb"`
	LogMaxBackups int    `toml:"log_max_backups"`
	// Database settings
	DBDriver       string `toml:"db_driver"`
	DBPath         string `toml:"db_path"`
	DBDSN          string `toml:"db_dsn"`
	RequestTimeout int    `toml:"request_timeout"` // default request timeout in seconds (non-LLM routes)
	// Security settings
	ReadHeaderTimeout int   `toml:"read_header_timeout"` // seconds, max time to read request headers
	MaxHeaderBytes    int   `toml:"max_header_bytes"`    // max size of request headers in bytes
	BodyLimit         int64 `toml:"body_limit"`          // default max request body size in bytes
	ChatBodyLimit     int64 `toml:"chat_body_limit"`     // max body size for chat completions in bytes
	AdminMaxFails         int   `toml:"admin_max_fails"`          // max auth failures before temporary IP lockout
	AdminWindowSec        int   `toml:"admin_window_sec"`         // time window in seconds for counting admin auth failures
	GlobalRateLimitRPM    int   `toml:"global_rate_limit_rpm"`    // 0 = disabled
	GlobalRateLimitWindow int   `toml:"global_rate_limit_window"` // seconds
}

// ImageConfig contains image-generation behavior flags.
type ImageConfig struct {
	NSFW                    bool   `toml:"nsfw"`
	Format                  string `toml:"format"`
	BlockedParallelAttempts int    `toml:"blocked_parallel_attempts"`
	BlockedParallelEnabled  *bool  `toml:"blocked_parallel_enabled"`
}

// ProxyConfig contains proxy settings.
type ProxyConfig struct {
	BaseProxyURL       string `toml:"base_proxy_url"`
	AssetProxyURL      string `toml:"asset_proxy_url"`
	CFCookies          string `toml:"cf_cookies"`
	SkipProxySSLVerify bool   `toml:"skip_proxy_ssl_verify"`
	Enabled            bool   `toml:"enabled"`
	FlareSolverrURL    string `toml:"flaresolverr_url"`
	RefreshInterval    int    `toml:"refresh_interval"`
	Timeout            int    `toml:"timeout"`
	CFClearance        string `toml:"cf_clearance"`
	Browser            string `toml:"browser"`
	UserAgent          string `toml:"user_agent"`
}

// RetryConfig contains retry policy settings.
type RetryConfig struct {
	MaxTokens               int     `toml:"max_tokens"`
	PerTokenRetries         int     `toml:"per_token_retries"`
	ResetSessionStatusCodes []int   `toml:"reset_session_status_codes"`
	RetryBackoffBase        float64 `toml:"retry_backoff_base"`
	RetryBackoffFactor      float64 `toml:"retry_backoff_factor"`
	RetryBackoffMax         float64 `toml:"retry_backoff_max"`
	RetryBudget             float64 `toml:"retry_budget"`
}

// TokenConfig contains token pool settings.
type TokenConfig struct {
	FailThreshold         int    `toml:"fail_threshold"`
	UsageFlushIntervalSec int    `toml:"usage_flush_interval_sec"`
	SelectionAlgorithm    string `toml:"selection_algorithm" json:"selection_algorithm"`
	MaxInflight           int    `toml:"max_inflight"`
	RecentUsePenaltySec   int    `toml:"recent_use_penalty_sec" json:"recent_use_penalty_sec"`
}

// CacheConfig contains cache management settings.
type CacheConfig struct {
	ImageMaxMB int `toml:"image_max_mb" json:"image_max_mb"`
	VideoMaxMB int `toml:"video_max_mb" json:"video_max_mb"`
}

// Load loads configuration from the given path.
// If the file does not exist, returns default configuration.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path == "" {
		return cfg, nil
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return cfg, nil
	}

	md, err := toml.DecodeFile(path, cfg)
	if err != nil {
		return nil, err
	}
	if undecoded := activeUndecodedKeys(md.Undecoded()); len(undecoded) > 0 {
		return nil, fmt.Errorf("unknown config keys: %v", undecoded)
	}
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func activeUndecodedKeys(keys []toml.Key) []toml.Key {
	active := make([]toml.Key, 0, len(keys))
	for _, key := range keys {
		if _, deprecated := deprecatedConfigKeys[key.String()]; deprecated {
			continue
		}
		active = append(active, key)
	}
	return active
}

func validateConfig(cfg *Config) error {
	if err := ValidateImageFormat(cfg.Image.Format); err != nil {
		return err
	}
	if cfg.Token.RecentUsePenaltySec < 0 {
		return fmt.Errorf("token.recent_use_penalty_sec must be >= 0, got %d", cfg.Token.RecentUsePenaltySec)
	}
	if cfg.Cache.ImageMaxMB < 0 {
		return fmt.Errorf("cache.image_max_mb must be >= 0, got %d", cfg.Cache.ImageMaxMB)
	}
	if cfg.Cache.VideoMaxMB < 0 {
		return fmt.Errorf("cache.video_max_mb must be >= 0, got %d", cfg.Cache.VideoMaxMB)
	}
	if cfg.App.GlobalRateLimitRPM < 0 {
		return fmt.Errorf("app.global_rate_limit_rpm must be >= 0, got %d", cfg.App.GlobalRateLimitRPM)
	}
	if cfg.App.GlobalRateLimitRPM > 0 && cfg.App.GlobalRateLimitWindow <= 0 {
		return fmt.Errorf("app.global_rate_limit_window must be > 0 when global_rate_limit_rpm is enabled")
	}
	return nil
}

func EffectiveImageFormat(cfg *ImageConfig) string {
	if cfg == nil {
		return ImageFormatBase64
	}
	format := strings.ToLower(strings.TrimSpace(cfg.Format))
	if format == "" {
		return ImageFormatBase64
	}
	return format
}

func ValidateImageFormat(format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case ImageFormatBase64, ImageFormatLocalURL:
		return nil
	default:
		return fmt.Errorf("image.format must be one of %s, %s", ImageFormatBase64, ImageFormatLocalURL)
	}
}
