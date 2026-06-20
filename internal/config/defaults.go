package config

import "github.com/crmmc/grokforge/internal/upstream/transport"

func boolPtr(v bool) *bool {
	return &v
}

// DefaultConfig returns the default configuration.
func DefaultConfig() *Config {
	return &Config{
		App: AppConfig{
			AppKey:                 "",
			MediaGenerationEnabled: true,
			Temporary:              true,
			DisableMemory:          true,
			Stream:                 true,
			Thinking:               true,
			DynamicStatsig:         true,
			CustomInstruction:      "",
			FilterTags:             []string{"xaiartifact", "xai:tool_usage_card", "grok:render"},
			Host:                   "0.0.0.0",
			Port:                   8080,
			LogJSON:                false,
			LogLevel:               "info",
			LogFilePath:            "logs/grokforge.log",
			LogMaxSizeMB:           50,
			LogMaxBackups:          3,
			DBDriver:               "sqlite",
			DBPath:                 "data/grokforge.db",
			DBDSN:                  "",
			RequestTimeout:         60,
			ReadHeaderTimeout:      10,
			MaxHeaderBytes:         1 << 20,  // 1MB
			BodyLimit:              1 << 20,  // 1MB
			ChatBodyLimit:          10 << 20, // 10MB
			AdminMaxFails:          10,
			AdminWindowSec:         300, // 5 minutes
			GlobalRateLimitRPM:     0,   // disabled by default
			GlobalRateLimitWindow:  60,
		},
		Image: ImageConfig{
			Format:                  ImageFormatBase64,
			NSFW:                    false,
			BlockedParallelAttempts: 5,
			BlockedParallelEnabled:  boolPtr(true),
		},
		Proxy: ProxyConfig{
			BaseProxyURL:       "",
			AssetProxyURL:      "",
			CFCookies:          "",
			SkipProxySSLVerify: false,
			Enabled:            false,
			FlareSolverrURL:    "",
			RefreshInterval:    3600,
			Timeout:            300,
			CFClearance:        "",
			Browser:            transport.DefaultProfile,
			UserAgent:          transport.DefaultUserAgent(),
		},
		Retry: RetryConfig{
			MaxTokens:               5,
			PerTokenRetries:         2,
			ResetSessionStatusCodes: []int{403},
			RetryBackoffBase:        0.5,
			RetryBackoffFactor:      2.0,
			RetryBackoffMax:         20.0,
			RetryBudget:             60.0,
		},
		Token: TokenConfig{
			FailThreshold:         5,
			UsageFlushIntervalSec: 30,
			SelectionAlgorithm:    "high_quota_first",
			MaxInflight:           8,
			RecentUsePenaltySec:   15,
		},
		Cache: CacheConfig{
			ImageMaxMB: 0,
			VideoMaxMB: 0,
		},
		Console: ConsoleConfig{
			Enabled:   false,
			WebSearch: false,
		},
	}
}
