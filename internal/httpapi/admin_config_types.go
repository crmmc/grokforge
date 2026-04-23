package httpapi

// ConfigResponse mirrors config.Config but with masked secrets for API responses.
type ConfigResponse struct {
	App   AppConfigResponse   `json:"app"`
	Image ImageConfigResponse `json:"image"`
	Proxy ProxyConfigResponse `json:"proxy"`
	Retry RetryConfigResponse `json:"retry"`
	Token TokenConfigResponse `json:"token"`
}

// AppConfigResponse is the API response for app config (secrets masked).
type AppConfigResponse struct {
	AppKey                 string   `json:"app_key"`
	MediaGenerationEnabled bool     `json:"media_generation_enabled"`
	Temporary              bool     `json:"temporary"`
	DisableMemory          bool     `json:"disable_memory"`
	Stream                 bool     `json:"stream"`
	Thinking               bool     `json:"thinking"`
	DynamicStatsig         bool     `json:"dynamic_statsig"`
	CustomInstruction      string   `json:"custom_instruction"`
	FilterTags             []string `json:"filter_tags"`
	Host                   string   `json:"host"`
	Port                   int      `json:"port"`
	LogJSON                bool     `json:"log_json"`
	LogLevel               string   `json:"log_level"`
	DBDriver               string   `json:"db_driver"`
	DBPath                 string   `json:"db_path"`
	DBDSN                  string   `json:"db_dsn"`
	RequestTimeout         int      `json:"request_timeout"`
	ReadHeaderTimeout      int      `json:"read_header_timeout"`
	MaxHeaderBytes         int      `json:"max_header_bytes"`
	BodyLimit              int64    `json:"body_limit"`
	ChatBodyLimit          int64    `json:"chat_body_limit"`
	AdminMaxFails          int      `json:"admin_max_fails"`
	AdminWindowSec         int      `json:"admin_window_sec"`
}

// ProxyConfigResponse is the API response for proxy config.
type ProxyConfigResponse struct {
	BaseProxyURL       string `json:"base_proxy_url"`
	AssetProxyURL      string `json:"asset_proxy_url"`
	CFCookies          string `json:"cf_cookies"`
	SkipProxySSLVerify bool   `json:"skip_proxy_ssl_verify"`
	Enabled            bool   `json:"enabled"`
	FlareSolverrURL    string `json:"flaresolverr_url"`
	RefreshInterval    int    `json:"refresh_interval"`
	Timeout            int    `json:"timeout"`
	CFClearance        string `json:"cf_clearance"`
	Browser            string `json:"browser"`
	UserAgent          string `json:"user_agent"`
}

// ImageConfigResponse is the API response for image config.
type ImageConfigResponse struct {
	NSFW                    bool `json:"nsfw"`
	BlockedParallelAttempts int  `json:"blocked_parallel_attempts"`
	BlockedParallelEnabled  bool `json:"blocked_parallel_enabled"`
}

// RetryConfigResponse is the API response for retry config.
type RetryConfigResponse struct {
	MaxTokens               int     `json:"max_tokens"`
	PerTokenRetries         int     `json:"per_token_retries"`
	ResetSessionStatusCodes []int   `json:"reset_session_status_codes"`
	RetryBackoffBase        float64 `json:"retry_backoff_base"`
	RetryBackoffFactor      float64 `json:"retry_backoff_factor"`
	RetryBackoffMax         float64 `json:"retry_backoff_max"`
	RetryBudget             float64 `json:"retry_budget"`
}

// TokenConfigResponse is the API response for token config.
type TokenConfigResponse struct {
	FailThreshold         int    `json:"fail_threshold"`
	UsageFlushIntervalSec int    `json:"usage_flush_interval_sec"`
	SelectionAlgorithm    string `json:"selection_algorithm"`
}

// ConfigUpdateRequest represents the fields that can be hot-reloaded.
type ConfigUpdateRequest struct {
	App   *AppConfigUpdate   `json:"app,omitempty"`
	Image *ImageConfigUpdate `json:"image,omitempty"`
	Proxy *ProxyConfigUpdate `json:"proxy,omitempty"`
	Retry *RetryConfigUpdate `json:"retry,omitempty"`
	Token *TokenConfigUpdate `json:"token,omitempty"`
}

// ImageConfigUpdate contains hot-reloadable image config fields.
type ImageConfigUpdate struct {
	NSFW                    *bool `json:"nsfw,omitempty"`
	BlockedParallelAttempts *int  `json:"blocked_parallel_attempts,omitempty"`
	BlockedParallelEnabled  *bool `json:"blocked_parallel_enabled,omitempty"`
}

// ProxyConfigUpdate contains hot-reloadable proxy config fields.
type ProxyConfigUpdate struct {
	BaseProxyURL       *string `json:"base_proxy_url,omitempty"`
	AssetProxyURL      *string `json:"asset_proxy_url,omitempty"`
	CFCookies          *string `json:"cf_cookies,omitempty"`
	SkipProxySSLVerify *bool   `json:"skip_proxy_ssl_verify,omitempty"`
	Enabled            *bool   `json:"enabled,omitempty"`
	FlareSolverrURL    *string `json:"flaresolverr_url,omitempty"`
	RefreshInterval    *int    `json:"refresh_interval,omitempty"`
	Timeout            *int    `json:"timeout,omitempty"`
	CFClearance        *string `json:"cf_clearance,omitempty"`
	Browser            *string `json:"browser,omitempty"`
	UserAgent          *string `json:"user_agent,omitempty"`
}

// AppConfigUpdate contains hot-reloadable app config fields.
type AppConfigUpdate struct {
	AppKey                 *string   `json:"app_key,omitempty"`
	MediaGenerationEnabled *bool     `json:"media_generation_enabled,omitempty"`
	RequestTimeout         *int      `json:"request_timeout,omitempty"`
	Temporary              *bool     `json:"temporary,omitempty"`
	Stream                 *bool     `json:"stream,omitempty"`
	Thinking               *bool     `json:"thinking,omitempty"`
	DynamicStatsig         *bool     `json:"dynamic_statsig,omitempty"`
	CustomInstruction      *string   `json:"custom_instruction,omitempty"`
	FilterTags             *[]string `json:"filter_tags,omitempty"`
	DisableMemory          *bool     `json:"disable_memory,omitempty"`
	ReadHeaderTimeout      *int      `json:"read_header_timeout,omitempty"`
	MaxHeaderBytes         *int      `json:"max_header_bytes,omitempty"`
	BodyLimit              *int64    `json:"body_limit,omitempty"`
	ChatBodyLimit          *int64    `json:"chat_body_limit,omitempty"`
	AdminMaxFails          *int      `json:"admin_max_fails,omitempty"`
	AdminWindowSec         *int      `json:"admin_window_sec,omitempty"`
}

// RetryConfigUpdate contains hot-reloadable retry config fields.
type RetryConfigUpdate struct {
	MaxTokens               *int     `json:"max_tokens,omitempty"`
	PerTokenRetries         *int     `json:"per_token_retries,omitempty"`
	ResetSessionStatusCodes *[]int   `json:"reset_session_status_codes,omitempty"`
	RetryBackoffBase        *float64 `json:"retry_backoff_base,omitempty"`
	RetryBackoffFactor      *float64 `json:"retry_backoff_factor,omitempty"`
	RetryBackoffMax         *float64 `json:"retry_backoff_max,omitempty"`
	RetryBudget             *float64 `json:"retry_budget,omitempty"`
}

// TokenConfigUpdate contains hot-reloadable token config fields.
type TokenConfigUpdate struct {
	FailThreshold         *int    `json:"fail_threshold,omitempty"`
	UsageFlushIntervalSec *int    `json:"usage_flush_interval_sec,omitempty"`
	SelectionAlgorithm    *string `json:"selection_algorithm,omitempty"`
}
