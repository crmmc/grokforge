package config

import (
	"fmt"
	"strconv"
	"strings"
)

var validSelectionAlgorithms = map[string]struct{}{
	"high_quota_first": {},
	"random":           {},
	"round_robin":      {},
}

// ApplyDBOverrides applies database config entries on top of file-based config.
// Priority: DB > config file > defaults.
func (c *Config) ApplyDBOverrides(kvs map[string]string) error {
	if err := validateProxyOverrides(kvs); err != nil {
		return err
	}

	for k, v := range kvs {
		if _, deprecated := deprecatedConfigKeys[k]; deprecated {
			continue
		}
		switch k {
		case "app.app_key":
			c.App.AppKey = v
		case "app.media_generation_enabled":
			parsed, err := parseBoolOverride(k, v)
			if err != nil {
				return err
			}
			c.App.MediaGenerationEnabled = parsed
		case "app.temporary":
			parsed, err := parseBoolOverride(k, v)
			if err != nil {
				return err
			}
			c.App.Temporary = parsed
		case "app.stream":
			parsed, err := parseBoolOverride(k, v)
			if err != nil {
				return err
			}
			c.App.Stream = parsed
		case "app.thinking":
			parsed, err := parseBoolOverride(k, v)
			if err != nil {
				return err
			}
			c.App.Thinking = parsed
		case "app.dynamic_statsig":
			parsed, err := parseBoolOverride(k, v)
			if err != nil {
				return err
			}
			c.App.DynamicStatsig = parsed
		case "app.custom_instruction":
			c.App.CustomInstruction = v
		case "app.filter_tags":
			parsed, err := parseStringListOverride(k, v)
			if err != nil {
				return err
			}
			c.App.FilterTags = parsed
		case "app.disable_memory":
			parsed, err := parseBoolOverride(k, v)
			if err != nil {
				return err
			}
			c.App.DisableMemory = parsed
		case "app.request_timeout":
			parsed, err := parseIntOverride(k, v)
			if err != nil {
				return err
			}
			c.App.RequestTimeout = parsed
		case "app.read_header_timeout":
			parsed, err := parseIntOverride(k, v)
			if err != nil {
				return err
			}
			c.App.ReadHeaderTimeout = parsed
		case "app.max_header_bytes":
			parsed, err := parseIntOverride(k, v)
			if err != nil {
				return err
			}
			c.App.MaxHeaderBytes = parsed
		case "app.body_limit":
			parsed, err := parseInt64Override(k, v)
			if err != nil {
				return err
			}
			c.App.BodyLimit = parsed
		case "app.chat_body_limit":
			parsed, err := parseInt64Override(k, v)
			if err != nil {
				return err
			}
			c.App.ChatBodyLimit = parsed
		case "app.admin_max_fails":
			parsed, err := parseIntOverride(k, v)
			if err != nil {
				return err
			}
			c.App.AdminMaxFails = parsed
		case "app.admin_window_sec":
			parsed, err := parseIntOverride(k, v)
			if err != nil {
				return err
			}
			c.App.AdminWindowSec = parsed
		case "proxy.base_proxy_url":
			c.Proxy.BaseProxyURL = v
		case "proxy.asset_proxy_url":
			c.Proxy.AssetProxyURL = v
		case "proxy.cf_cookies":
			c.Proxy.CFCookies = v
		case "proxy.skip_proxy_ssl_verify":
			parsed, err := parseBoolOverride(k, v)
			if err != nil {
				return err
			}
			c.Proxy.SkipProxySSLVerify = parsed
		case "proxy.enabled":
			parsed, err := parseBoolOverride(k, v)
			if err != nil {
				return err
			}
			c.Proxy.Enabled = parsed
		case "proxy.flaresolverr_url":
			c.Proxy.FlareSolverrURL = v
		case "proxy.refresh_interval":
			parsed, err := parseIntOverride(k, v)
			if err != nil {
				return err
			}
			c.Proxy.RefreshInterval = parsed
		case "proxy.timeout":
			parsed, err := parseIntOverride(k, v)
			if err != nil {
				return err
			}
			c.Proxy.Timeout = parsed
		case "proxy.cf_clearance":
			c.Proxy.CFClearance = v
		case "proxy.browser":
			c.Proxy.Browser = v
		case "proxy.user_agent":
			c.Proxy.UserAgent = v
		case "retry.max_tokens":
			parsed, err := parseIntOverride(k, v)
			if err != nil {
				return err
			}
			c.Retry.MaxTokens = parsed
		case "retry.per_token_retries":
			parsed, err := parseIntOverride(k, v)
			if err != nil {
				return err
			}
			c.Retry.PerTokenRetries = parsed
		case "retry.reset_session_status_codes":
			parsed, err := parseIntListOverride(k, v)
			if err != nil {
				return err
			}
			c.Retry.ResetSessionStatusCodes = parsed
		case "retry.retry_backoff_base":
			parsed, err := parseFloatOverride(k, v)
			if err != nil {
				return err
			}
			c.Retry.RetryBackoffBase = parsed
		case "retry.retry_backoff_factor":
			parsed, err := parseFloatOverride(k, v)
			if err != nil {
				return err
			}
			c.Retry.RetryBackoffFactor = parsed
		case "retry.retry_backoff_max":
			parsed, err := parseFloatOverride(k, v)
			if err != nil {
				return err
			}
			c.Retry.RetryBackoffMax = parsed
		case "retry.retry_budget":
			parsed, err := parseFloatOverride(k, v)
			if err != nil {
				return err
			}
			c.Retry.RetryBudget = parsed
		case "image.nsfw":
			parsed, err := parseBoolOverride(k, v)
			if err != nil {
				return err
			}
			c.Image.NSFW = parsed
		case "image.blocked_parallel_attempts":
			parsed, err := parseIntOverride(k, v)
			if err != nil {
				return err
			}
			c.Image.BlockedParallelAttempts = parsed
		case "image.blocked_parallel_enabled":
			parsed, err := parseBoolOverride(k, v)
			if err != nil {
				return err
			}
			c.Image.BlockedParallelEnabled = &parsed
		case "token.fail_threshold":
			parsed, err := parseIntOverride(k, v)
			if err != nil {
				return err
			}
			c.Token.FailThreshold = parsed
		case "token.usage_flush_interval_sec":
			parsed, err := parseIntOverride(k, v)
			if err != nil {
				return err
			}
			c.Token.UsageFlushIntervalSec = parsed
		case "token.selection_algorithm":
			if _, ok := validSelectionAlgorithms[v]; !ok {
				return fmt.Errorf("config: invalid value %q for %s", v, k)
			}
			c.Token.SelectionAlgorithm = v
		case "token.max_inflight":
			parsed, err := parseIntOverride(k, v)
			if err != nil {
				return err
			}
			c.Token.MaxInflight = parsed
		case "token.recent_use_penalty_sec":
			parsed, err := parseIntOverride(k, v)
			if err != nil {
				return err
			}
			if parsed < 0 {
				return fmt.Errorf("token.recent_use_penalty_sec must be >= 0, got %d", parsed)
			}
			c.Token.RecentUsePenaltySec = parsed
		case "cache.image_max_mb":
			parsed, err := parseIntOverride(k, v)
			if err != nil {
				return err
			}
			if parsed < 0 {
				return fmt.Errorf("config: %s must be >= 0, got %d", k, parsed)
			}
			c.Cache.ImageMaxMB = parsed
		case "cache.video_max_mb":
			parsed, err := parseIntOverride(k, v)
			if err != nil {
				return err
			}
			if parsed < 0 {
				return fmt.Errorf("config: %s must be >= 0, got %d", k, parsed)
			}
			c.Cache.VideoMaxMB = parsed
		default:
			return fmt.Errorf("config: unknown db override key %q", k)
		}
	}

	return nil
}

func validateProxyOverrides(kvs map[string]string) error {
	browser, hasBrowser := kvs["proxy.browser"]
	userAgent, hasUserAgent := kvs["proxy.user_agent"]
	if hasBrowser != hasUserAgent {
		return fmt.Errorf("config: proxy.browser and proxy.user_agent must be overridden together")
	}
	if hasBrowser && strings.TrimSpace(browser) == "" {
		return fmt.Errorf("config: proxy.browser override cannot be empty")
	}
	if hasUserAgent && strings.TrimSpace(userAgent) == "" {
		return fmt.Errorf("config: proxy.user_agent override cannot be empty")
	}
	return nil
}

func parseBoolOverride(key, value string) (bool, error) {
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("config: invalid boolean for %s: %q", key, value)
	}
	return parsed, nil
}

func parseIntOverride(key, value string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("config: invalid integer for %s: %q", key, value)
	}
	return parsed, nil
}

func parseInt64Override(key, value string) (int64, error) {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("config: invalid int64 for %s: %q", key, value)
	}
	return parsed, nil
}

func parseFloatOverride(key, value string) (float64, error) {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("config: invalid float for %s: %q", key, value)
	}
	return parsed, nil
}

func parseIntListOverride(key, value string) ([]int, error) {
	if value == "" {
		return []int{}, nil
	}
	parts := strings.Split(value, ",")
	parsed := make([]int, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			return nil, fmt.Errorf("config: invalid integer list for %s: %q", key, value)
		}
		item, err := strconv.Atoi(trimmed)
		if err != nil {
			return nil, fmt.Errorf("config: invalid integer list for %s: %q", key, value)
		}
		parsed = append(parsed, item)
	}
	return parsed, nil
}

func parseStringListOverride(key, value string) ([]string, error) {
	if value == "" {
		return []string{}, nil
	}
	parts := strings.Split(value, ",")
	parsed := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			return nil, fmt.Errorf("config: invalid string list for %s: %q", key, value)
		}
		parsed = append(parsed, trimmed)
	}
	return parsed, nil
}
