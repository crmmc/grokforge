package httpapi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/crmmc/grokforge/internal/token"
)

// handlePutConfig returns a handler that updates hot-reloadable config fields.
// Changes are persisted to the database via ConfigStore (DB > config file).
func handlePutConfig(cfg *config.Config, configStore *store.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ConfigUpdateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			WriteError(w, 400, "invalid_request", "invalid_json",
				"Invalid JSON in request body")
			return
		}

		// Apply hot-reloadable updates
		if req.App != nil {
			// Skip masked placeholder — only update if user entered a real value
			if req.App.AppKey != nil && *req.App.AppKey != maskedConfigSecret {
				if *req.App.AppKey == "" {
					WriteError(w, 400, "invalid_request", "invalid_value", "app_key cannot be empty")
					return
				}
				cfg.App.AppKey = *req.App.AppKey
			}
			if req.App.MediaGenerationEnabled != nil {
				cfg.App.MediaGenerationEnabled = *req.App.MediaGenerationEnabled
			}
			if req.App.RequestTimeout != nil {
				cfg.App.RequestTimeout = *req.App.RequestTimeout
			}
			if req.App.Temporary != nil {
				cfg.App.Temporary = *req.App.Temporary
			}
			if req.App.Stream != nil {
				cfg.App.Stream = *req.App.Stream
			}
			if req.App.Thinking != nil {
				cfg.App.Thinking = *req.App.Thinking
			}
			if req.App.DynamicStatsig != nil {
				cfg.App.DynamicStatsig = *req.App.DynamicStatsig
			}
			if req.App.CustomInstruction != nil {
				cfg.App.CustomInstruction = *req.App.CustomInstruction
			}
			if req.App.FilterTags != nil {
				cfg.App.FilterTags = filterEmptyStrings(*req.App.FilterTags)
			}
			if req.App.DisableMemory != nil {
				cfg.App.DisableMemory = *req.App.DisableMemory
			}
			if req.App.ReadHeaderTimeout != nil {
				cfg.App.ReadHeaderTimeout = *req.App.ReadHeaderTimeout
			}
			if req.App.MaxHeaderBytes != nil {
				cfg.App.MaxHeaderBytes = *req.App.MaxHeaderBytes
			}
			if req.App.BodyLimit != nil {
				cfg.App.BodyLimit = *req.App.BodyLimit
			}
			if req.App.ChatBodyLimit != nil {
				cfg.App.ChatBodyLimit = *req.App.ChatBodyLimit
			}
			if req.App.AdminMaxFails != nil {
				cfg.App.AdminMaxFails = *req.App.AdminMaxFails
			}
			if req.App.AdminWindowSec != nil {
				cfg.App.AdminWindowSec = *req.App.AdminWindowSec
			}
		}

		if req.Image != nil {
			if req.Image.NSFW != nil {
				cfg.Image.NSFW = *req.Image.NSFW
			}
			if req.Image.BlockedParallelAttempts != nil {
				cfg.Image.BlockedParallelAttempts = *req.Image.BlockedParallelAttempts
			}
			if req.Image.BlockedParallelEnabled != nil {
				enabled := *req.Image.BlockedParallelEnabled
				cfg.Image.BlockedParallelEnabled = &enabled
			}
		}

		if req.Proxy != nil {
			if req.Proxy.BaseProxyURL != nil {
				cfg.Proxy.BaseProxyURL = *req.Proxy.BaseProxyURL
			}
			if req.Proxy.AssetProxyURL != nil {
				cfg.Proxy.AssetProxyURL = *req.Proxy.AssetProxyURL
			}
			if req.Proxy.CFCookies != nil && *req.Proxy.CFCookies != maskedConfigSecret {
				cfg.Proxy.CFCookies = *req.Proxy.CFCookies
			}
			if req.Proxy.SkipProxySSLVerify != nil {
				cfg.Proxy.SkipProxySSLVerify = *req.Proxy.SkipProxySSLVerify
			}
			if req.Proxy.Enabled != nil {
				cfg.Proxy.Enabled = *req.Proxy.Enabled
			}
			if req.Proxy.FlareSolverrURL != nil {
				cfg.Proxy.FlareSolverrURL = *req.Proxy.FlareSolverrURL
			}
			if req.Proxy.RefreshInterval != nil {
				cfg.Proxy.RefreshInterval = *req.Proxy.RefreshInterval
			}
			if req.Proxy.Timeout != nil {
				cfg.Proxy.Timeout = *req.Proxy.Timeout
			}
			if req.Proxy.CFClearance != nil && *req.Proxy.CFClearance != maskedConfigSecret {
				cfg.Proxy.CFClearance = *req.Proxy.CFClearance
			}
			if req.Proxy.Browser != nil {
				cfg.Proxy.Browser = *req.Proxy.Browser
				// Auto-pair UA when browser changes and UA is not explicitly provided.
				if req.Proxy.UserAgent == nil {
					if pairedUA, ok := config.BrowserUAMap[*req.Proxy.Browser]; ok {
						cfg.Proxy.UserAgent = pairedUA
					}
				}
			}
			if req.Proxy.UserAgent != nil {
				cfg.Proxy.UserAgent = *req.Proxy.UserAgent
			}
		}

		if req.Retry != nil {
			if req.Retry.MaxTokens != nil {
				cfg.Retry.MaxTokens = *req.Retry.MaxTokens
			}
			if req.Retry.PerTokenRetries != nil {
				cfg.Retry.PerTokenRetries = *req.Retry.PerTokenRetries
			}
			if req.Retry.ResetSessionStatusCodes != nil {
				cfg.Retry.ResetSessionStatusCodes = *req.Retry.ResetSessionStatusCodes
			}
			if req.Retry.RetryBackoffBase != nil {
				cfg.Retry.RetryBackoffBase = *req.Retry.RetryBackoffBase
			}
			if req.Retry.RetryBackoffFactor != nil {
				cfg.Retry.RetryBackoffFactor = *req.Retry.RetryBackoffFactor
			}
			if req.Retry.RetryBackoffMax != nil {
				cfg.Retry.RetryBackoffMax = *req.Retry.RetryBackoffMax
			}
			if req.Retry.RetryBudget != nil {
				cfg.Retry.RetryBudget = *req.Retry.RetryBudget
			}
		}

		if req.Token != nil {
			if req.Token.FailThreshold != nil {
				cfg.Token.FailThreshold = *req.Token.FailThreshold
			}
			if req.Token.UsageFlushIntervalSec != nil {
				cfg.Token.UsageFlushIntervalSec = *req.Token.UsageFlushIntervalSec
			}
			if req.Token.SelectionAlgorithm != nil {
				if !token.ValidAlgorithm(*req.Token.SelectionAlgorithm) {
					WriteError(w, 400, "invalid_request", "invalid_algorithm",
						"Invalid selection algorithm. Must be one of: high_quota_first, random, round_robin")
					return
				}
				cfg.Token.SelectionAlgorithm = *req.Token.SelectionAlgorithm
			}
		}

		if req.Cache != nil {
			if req.Cache.ImageMaxMB != nil {
				if *req.Cache.ImageMaxMB < 0 {
					WriteError(w, 400, "invalid_request", "invalid_value", "cache.image_max_mb must be >= 0")
					return
				}
				cfg.Cache.ImageMaxMB = *req.Cache.ImageMaxMB
			}
			if req.Cache.VideoMaxMB != nil {
				if *req.Cache.VideoMaxMB < 0 {
					WriteError(w, 400, "invalid_request", "invalid_value", "cache.video_max_mb must be >= 0")
					return
				}
				cfg.Cache.VideoMaxMB = *req.Cache.VideoMaxMB
			}
		}

		// Persist hot-reloadable app fields to database
		dbUpdates := make(map[string]string)
		if req.App != nil {
			if req.App.MediaGenerationEnabled != nil {
				dbUpdates["app.media_generation_enabled"] = fmt.Sprintf("%t", *req.App.MediaGenerationEnabled)
			}
			if req.App.RequestTimeout != nil {
				dbUpdates["app.request_timeout"] = fmt.Sprintf("%d", *req.App.RequestTimeout)
			}
			if req.App.AppKey != nil && *req.App.AppKey != maskedConfigSecret && *req.App.AppKey != "" {
				dbUpdates["app.app_key"] = *req.App.AppKey
			}
			if req.App.Temporary != nil {
				dbUpdates["app.temporary"] = fmt.Sprintf("%t", *req.App.Temporary)
			}
			if req.App.Stream != nil {
				dbUpdates["app.stream"] = fmt.Sprintf("%t", *req.App.Stream)
			}
			if req.App.Thinking != nil {
				dbUpdates["app.thinking"] = fmt.Sprintf("%t", *req.App.Thinking)
			}
			if req.App.DynamicStatsig != nil {
				dbUpdates["app.dynamic_statsig"] = fmt.Sprintf("%t", *req.App.DynamicStatsig)
			}
			if req.App.CustomInstruction != nil {
				dbUpdates["app.custom_instruction"] = *req.App.CustomInstruction
			}
			if req.App.FilterTags != nil {
				dbUpdates["app.filter_tags"] = strings.Join(filterEmptyStrings(*req.App.FilterTags), ",")
			}
			if req.App.DisableMemory != nil {
				dbUpdates["app.disable_memory"] = fmt.Sprintf("%t", *req.App.DisableMemory)
			}
			if req.App.ReadHeaderTimeout != nil {
				dbUpdates["app.read_header_timeout"] = fmt.Sprintf("%d", *req.App.ReadHeaderTimeout)
			}
			if req.App.MaxHeaderBytes != nil {
				dbUpdates["app.max_header_bytes"] = fmt.Sprintf("%d", *req.App.MaxHeaderBytes)
			}
			if req.App.BodyLimit != nil {
				dbUpdates["app.body_limit"] = fmt.Sprintf("%d", *req.App.BodyLimit)
			}
			if req.App.ChatBodyLimit != nil {
				dbUpdates["app.chat_body_limit"] = fmt.Sprintf("%d", *req.App.ChatBodyLimit)
			}
			if req.App.AdminMaxFails != nil {
				dbUpdates["app.admin_max_fails"] = fmt.Sprintf("%d", *req.App.AdminMaxFails)
			}
			if req.App.AdminWindowSec != nil {
				dbUpdates["app.admin_window_sec"] = fmt.Sprintf("%d", *req.App.AdminWindowSec)
			}
		}
		if req.Image != nil {
			if req.Image.NSFW != nil {
				dbUpdates["image.nsfw"] = fmt.Sprintf("%t", *req.Image.NSFW)
			}
			if req.Image.BlockedParallelAttempts != nil {
				dbUpdates["image.blocked_parallel_attempts"] = fmt.Sprintf("%d", *req.Image.BlockedParallelAttempts)
			}
			if req.Image.BlockedParallelEnabled != nil {
				dbUpdates["image.blocked_parallel_enabled"] = fmt.Sprintf("%t", *req.Image.BlockedParallelEnabled)
			}
		}
		if req.Proxy != nil {
			if req.Proxy.BaseProxyURL != nil {
				dbUpdates["proxy.base_proxy_url"] = *req.Proxy.BaseProxyURL
			}
			if req.Proxy.AssetProxyURL != nil {
				dbUpdates["proxy.asset_proxy_url"] = *req.Proxy.AssetProxyURL
			}
			if req.Proxy.CFCookies != nil && *req.Proxy.CFCookies != maskedConfigSecret {
				dbUpdates["proxy.cf_cookies"] = *req.Proxy.CFCookies
			}
			if req.Proxy.SkipProxySSLVerify != nil {
				dbUpdates["proxy.skip_proxy_ssl_verify"] = fmt.Sprintf("%t", *req.Proxy.SkipProxySSLVerify)
			}
			if req.Proxy.Enabled != nil {
				dbUpdates["proxy.enabled"] = fmt.Sprintf("%t", *req.Proxy.Enabled)
			}
			if req.Proxy.FlareSolverrURL != nil {
				dbUpdates["proxy.flaresolverr_url"] = *req.Proxy.FlareSolverrURL
			}
			if req.Proxy.RefreshInterval != nil {
				dbUpdates["proxy.refresh_interval"] = fmt.Sprintf("%d", *req.Proxy.RefreshInterval)
			}
			if req.Proxy.Timeout != nil {
				dbUpdates["proxy.timeout"] = fmt.Sprintf("%d", *req.Proxy.Timeout)
			}
			if req.Proxy.CFClearance != nil && *req.Proxy.CFClearance != maskedConfigSecret {
				dbUpdates["proxy.cf_clearance"] = *req.Proxy.CFClearance
			}
			if req.Proxy.Browser != nil {
				dbUpdates["proxy.browser"] = *req.Proxy.Browser
				// Auto-pair UA when browser changes and UA is not explicitly provided.
				if req.Proxy.UserAgent == nil {
					if pairedUA, ok := config.BrowserUAMap[*req.Proxy.Browser]; ok {
						dbUpdates["proxy.user_agent"] = pairedUA
					}
				}
			}
			if req.Proxy.UserAgent != nil {
				dbUpdates["proxy.user_agent"] = *req.Proxy.UserAgent
			}
		}
		if req.Retry != nil {
			if req.Retry.MaxTokens != nil {
				dbUpdates["retry.max_tokens"] = fmt.Sprintf("%d", *req.Retry.MaxTokens)
			}
			if req.Retry.PerTokenRetries != nil {
				dbUpdates["retry.per_token_retries"] = fmt.Sprintf("%d", *req.Retry.PerTokenRetries)
			}
			if req.Retry.ResetSessionStatusCodes != nil {
				codes := make([]string, len(*req.Retry.ResetSessionStatusCodes))
				for i, c := range *req.Retry.ResetSessionStatusCodes {
					codes[i] = strconv.Itoa(c)
				}
				dbUpdates["retry.reset_session_status_codes"] = strings.Join(codes, ",")
			}
			if req.Retry.RetryBackoffBase != nil {
				dbUpdates["retry.retry_backoff_base"] = fmt.Sprintf("%g", *req.Retry.RetryBackoffBase)
			}
			if req.Retry.RetryBackoffFactor != nil {
				dbUpdates["retry.retry_backoff_factor"] = fmt.Sprintf("%g", *req.Retry.RetryBackoffFactor)
			}
			if req.Retry.RetryBackoffMax != nil {
				dbUpdates["retry.retry_backoff_max"] = fmt.Sprintf("%g", *req.Retry.RetryBackoffMax)
			}
			if req.Retry.RetryBudget != nil {
				dbUpdates["retry.retry_budget"] = fmt.Sprintf("%g", *req.Retry.RetryBudget)
			}
		}
		if req.Token != nil {
			if req.Token.FailThreshold != nil {
				dbUpdates["token.fail_threshold"] = fmt.Sprintf("%d", *req.Token.FailThreshold)
			}
			if req.Token.UsageFlushIntervalSec != nil {
				dbUpdates["token.usage_flush_interval_sec"] = fmt.Sprintf("%d", *req.Token.UsageFlushIntervalSec)
			}
			if req.Token.SelectionAlgorithm != nil {
				dbUpdates["token.selection_algorithm"] = *req.Token.SelectionAlgorithm
			}
		}
		if req.Cache != nil {
			if req.Cache.ImageMaxMB != nil {
				dbUpdates["cache.image_max_mb"] = fmt.Sprintf("%d", *req.Cache.ImageMaxMB)
			}
			if req.Cache.VideoMaxMB != nil {
				dbUpdates["cache.video_max_mb"] = fmt.Sprintf("%d", *req.Cache.VideoMaxMB)
			}
		}
		if configStore != nil && len(dbUpdates) > 0 {
			if err := configStore.SetMany(dbUpdates); err != nil {
				slog.Error("failed to persist config to database", "error", err)
				WriteError(w, 500, "server_error", "config_persist_failed", "Failed to persist config")
				return
			}
		}

		// Return updated config with masked secrets
		resp := configToResponse(cfg)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}
}

func handlePutConfigRuntime(runtime *config.Runtime, configStore *store.ConfigStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		snapshot := runtime.Snapshot()
		sc := &statusCapture{ResponseWriter: w}
		handlePutConfig(snapshot, configStore).ServeHTTP(sc, r)
		if sc.status == 0 || sc.status >= http.StatusBadRequest {
			return
		}
		runtime.Store(snapshot)
	}
}

// filterEmptyStrings filters empty strings and trims whitespace from a string slice.
func filterEmptyStrings(s []string) []string {
	result := make([]string, 0, len(s))
	for _, v := range s {
		v = strings.TrimSpace(v)
		if v != "" {
			result = append(result, v)
		}
	}
	return result
}
