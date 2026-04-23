// Package config provides configuration loading and management.
package config

import (
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Config is the root configuration structure.
type Config struct {
	App   AppConfig   `toml:"app"`
	Image ImageConfig `toml:"image"`
	Proxy ProxyConfig `toml:"proxy"`
	Retry RetryConfig `toml:"retry"`
	Token TokenConfig `toml:"token"`
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
	AdminMaxFails     int   `toml:"admin_max_fails"`     // max auth failures before temporary IP lockout
	AdminWindowSec    int   `toml:"admin_window_sec"`    // time window in seconds for counting admin auth failures
}

// ImageConfig contains image-generation behavior flags.
type ImageConfig struct {
	NSFW                    bool  `toml:"nsfw"`
	BlockedParallelAttempts int   `toml:"blocked_parallel_attempts"`
	BlockedParallelEnabled  *bool `toml:"blocked_parallel_enabled"`
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
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return nil, fmt.Errorf("unknown config keys: %v", undecoded)
	}

	return cfg, nil
}
