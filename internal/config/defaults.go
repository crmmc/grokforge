package config

func boolPtr(v bool) *bool {
	return &v
}

// BrowserUAMap maps browser fingerprint names to their paired User-Agent strings.
// Browser and UA must always be used as a pair.
var BrowserUAMap = map[string]string{
	"chrome_133":  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36",
	"chrome_144":  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36",
	"chrome_146":  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36",
	"firefox_135": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:135.0) Gecko/20100101 Firefox/135.0",
	"firefox_147": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:147.0) Gecko/20100101 Firefox/147.0",
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
		},
		Image: ImageConfig{
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
			Browser:            "chrome_146",
			UserAgent:          "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36",
		},
		Retry: RetryConfig{
			MaxTokens:               5,
			PerTokenRetries:         2,
			ResetSessionStatusCodes: []int{403},
			CoolingStatusCodes:      []int{429},
			RetryBackoffBase:        0.5,
			RetryBackoffFactor:      2.0,
			RetryBackoffMax:         20.0,
			RetryBudget:             60.0,
		},
		Token: TokenConfig{
			FailThreshold:         5,
			UsageFlushIntervalSec: 30,
			CoolCheckIntervalSec:  30,
			BasicCoolDurationMin:  240,
			SuperCoolDurationMin:  120,
			HeavyCoolDurationMin:  60,
			DefaultChatQuota:      50,
			DefaultImageQuota:     20,
			DefaultVideoQuota:     10,
			QuotaRecoveryMode:     "auto",
			SelectionAlgorithm:    "high_quota_first",
		},
	}
}
