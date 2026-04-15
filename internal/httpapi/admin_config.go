package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/crmmc/grokforge/internal/config"
)

const maskedConfigSecret = "********"

// maskSecret masks a secret string, showing first 4 and last 4 chars.
// Returns "***" for strings shorter than 4 chars, empty for empty strings.
func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) < 4 {
		return "***"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

func maskConfigSecret(s string) string {
	if s == "" {
		return ""
	}
	return maskedConfigSecret
}

// configToResponse converts config.Config to ConfigResponse with masked secrets.
func configToResponse(cfg *config.Config) ConfigResponse {
	return ConfigResponse{
		App: AppConfigResponse{
			AppKey:                 maskConfigSecret(cfg.App.AppKey),
			MediaGenerationEnabled: cfg.App.MediaGenerationEnabled,
			Temporary:              cfg.App.Temporary,
			DisableMemory:          cfg.App.DisableMemory,
			Stream:                 cfg.App.Stream,
			Thinking:               cfg.App.Thinking,
			DynamicStatsig:         cfg.App.DynamicStatsig,
			CustomInstruction:      cfg.App.CustomInstruction,
			FilterTags:             cfg.App.FilterTags,
			Host:                   cfg.App.Host,
			Port:                   cfg.App.Port,
			LogJSON:                cfg.App.LogJSON,
			LogLevel:               cfg.App.LogLevel,
			DBDriver:               cfg.App.DBDriver,
			DBPath:                 cfg.App.DBPath,
			DBDSN:                  maskConfigSecret(cfg.App.DBDSN),
			RequestTimeout:         cfg.App.RequestTimeout,
			ReadHeaderTimeout:      cfg.App.ReadHeaderTimeout,
			MaxHeaderBytes:         cfg.App.MaxHeaderBytes,
			BodyLimit:              cfg.App.BodyLimit,
			ChatBodyLimit:          cfg.App.ChatBodyLimit,
			AdminMaxFails:          cfg.App.AdminMaxFails,
			AdminWindowSec:         cfg.App.AdminWindowSec,
		},
		Image: ImageConfigResponse{
			NSFW:                    cfg.Image.NSFW,
			BlockedParallelAttempts: cfg.Image.BlockedParallelAttempts,
			BlockedParallelEnabled:  config.EffectiveBlockedParallelEnabled(&cfg.Image),
		},
		ImagineFast: ImagineFastConfigResponse{
			N:    cfg.ImagineFast.N,
			Size: cfg.ImagineFast.Size,
		},
		Proxy: ProxyConfigResponse{
			BaseProxyURL:       cfg.Proxy.BaseProxyURL,
			AssetProxyURL:      cfg.Proxy.AssetProxyURL,
			CFCookies:          maskConfigSecret(cfg.Proxy.CFCookies),
			SkipProxySSLVerify: cfg.Proxy.SkipProxySSLVerify,
			Enabled:            cfg.Proxy.Enabled,
			FlareSolverrURL:    cfg.Proxy.FlareSolverrURL,
			RefreshInterval:    cfg.Proxy.RefreshInterval,
			Timeout:            cfg.Proxy.Timeout,
			CFClearance:        maskConfigSecret(cfg.Proxy.CFClearance),
			Browser:            cfg.Proxy.Browser,
			UserAgent:          cfg.Proxy.UserAgent,
		},
		Retry: RetryConfigResponse{
			MaxTokens:               cfg.Retry.MaxTokens,
			PerTokenRetries:         cfg.Retry.PerTokenRetries,
			ResetSessionStatusCodes: cfg.Retry.ResetSessionStatusCodes,
			CoolingStatusCodes:      cfg.Retry.CoolingStatusCodes,
			RetryBackoffBase:        cfg.Retry.RetryBackoffBase,
			RetryBackoffFactor:      cfg.Retry.RetryBackoffFactor,
			RetryBackoffMax:         cfg.Retry.RetryBackoffMax,
			RetryBudget:             cfg.Retry.RetryBudget,
		},
		Token: TokenConfigResponse{
			FailThreshold:         cfg.Token.FailThreshold,
			UsageFlushIntervalSec: cfg.Token.UsageFlushIntervalSec,
			CoolCheckIntervalSec:  cfg.Token.CoolCheckIntervalSec,
			BasicCoolDurationMin:  cfg.Token.BasicCoolDurationMin,
			SuperCoolDurationMin:  cfg.Token.SuperCoolDurationMin,
			HeavyCoolDurationMin:  cfg.Token.HeavyCoolDurationMin,
			DefaultChatQuota:      cfg.Token.DefaultChatQuota,
			DefaultImageQuota:     cfg.Token.DefaultImageQuota,
			DefaultVideoQuota:     cfg.Token.DefaultVideoQuota,
			QuotaRecoveryMode:     cfg.Token.QuotaRecoveryMode,
			SelectionAlgorithm:    cfg.Token.SelectionAlgorithm,
		},
	}
}

// handleGetConfig returns a handler that returns the current config with masked secrets.
func handleGetConfig(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := configToResponse(cfg)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func handleGetConfigRuntime(runtime *config.Runtime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := configToResponse(runtime.Get())
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}
