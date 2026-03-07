package httpapi

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/crmmc/grokforge/internal/config"
)

// ctxKey is a context key type for middleware values.
type ctxKey string

const apiKeyIDKey ctxKey = "apiKeyID"
const modelWhitelistKey ctxKey = "modelWhitelist"

// APIKeyIDFromContext extracts the API key ID from the request context.
func APIKeyIDFromContext(ctx context.Context) (uint, bool) {
	id, ok := ctx.Value(apiKeyIDKey).(uint)
	return id, ok
}

// ModelWhitelistFromContext extracts the model whitelist from the request context.
// Returns nil if no whitelist is set (meaning all models are allowed).
func ModelWhitelistFromContext(ctx context.Context) []string {
	wl, _ := ctx.Value(modelWhitelistKey).([]string)
	return wl
}

// CheckModelWhitelist validates the requested model against the API key's whitelist.
// Returns true if the model is allowed. An empty/nil whitelist allows all models.
func CheckModelWhitelist(ctx context.Context, model string) bool {
	wl := ModelWhitelistFromContext(ctx)
	if len(wl) == 0 {
		return true
	}
	for _, m := range wl {
		if m == model {
			return true
		}
	}
	return false
}

// rateLimitEntry tracks per-minute request count for an API key.
type rateLimitEntry struct {
	count       atomic.Int64
	windowStart atomic.Int64 // unix timestamp
}

// APIKeyAuth returns a middleware that authenticates requests via API key DB lookup.
// Enforces daily_limit and rate_limit, returning 429 when exceeded.
func APIKeyAuth(akStore APIKeyStoreInterface) func(http.Handler) http.Handler {
	var rateLimitMap sync.Map // map[uint]*rateLimitEntry

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract Bearer token
			auth := r.Header.Get("Authorization")
			if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
				WriteError(w, 401, "authentication_error", "invalid_api_key", "Missing API key")
				return
			}
			token := strings.TrimPrefix(auth, "Bearer ")

			// DB lookup
			apiKey, err := akStore.GetByKey(r.Context(), token)
			if err != nil {
				WriteError(w, 401, "authentication_error", "invalid_api_key", "Invalid API key")
				return
			}

			// Check status
			if apiKey.Status != "active" {
				WriteError(w, 401, "authentication_error", "invalid_api_key", "API key is not active")
				return
			}

			// Check expiration
			if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
				WriteError(w, 401, "authentication_error", "invalid_api_key", "API key has expired")
				return
			}

			// Check daily limit
			if apiKey.DailyLimit > 0 && apiKey.DailyUsed >= apiKey.DailyLimit {
				WriteError(w, 429, "rate_limit_error", "daily_limit_exceeded", "daily limit exceeded")
				return
			}

			// Check per-minute rate limit
			if apiKey.RateLimit > 0 {
				now := time.Now().Unix()
				entryI, _ := rateLimitMap.LoadOrStore(apiKey.ID, &rateLimitEntry{})
				entry := entryI.(*rateLimitEntry)

				for {
					windowStart := entry.windowStart.Load()
					if now-windowStart >= 60 {
						// New minute window — CAS to prevent concurrent reset
						if entry.windowStart.CompareAndSwap(windowStart, now) {
							// Reset then increment — avoids stale old-window count race
							entry.count.Store(0)
							entry.count.Add(1)
							break
						}
						// CAS failed, another goroutine reset — retry check
						continue
					}
					count := entry.count.Add(1)
					if count > int64(apiKey.RateLimit) {
						WriteError(w, 429, "rate_limit_error", "rate_limit_exceeded", "rate limit exceeded")
						return
					}
					break
				}
			}

			// Auth passed — set context (usage increment moved to flow layer on success)
			ctx := context.WithValue(r.Context(), apiKeyIDKey, apiKey.ID)
			if len(apiKey.ModelWhitelist) > 0 {
				ctx = context.WithValue(ctx, modelWhitelistKey, []string(apiKey.ModelWhitelist))
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AppKeyAuth returns a middleware that validates App Key authentication for admin endpoints.
// AppKeyAuth rejects all requests when appKey is empty (secure by default).
// Authentication priority: cookie "gf_session" first, then Bearer header (API/script compat).
// Uses constant-time comparison to prevent timing attacks.
func AppKeyAuth(appKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Empty appKey means admin API is not configured - reject all
			if appKey == "" {
				WriteError(w, 403, "forbidden", "app_key_not_configured",
					"Admin API is not configured")
				return
			}

			// 1. Try cookie first
			if c, err := r.Cookie(adminCookieName); err == nil && c.Value != "" {
				if subtle.ConstantTimeCompare([]byte(c.Value), []byte(appKey)) == 1 {
					next.ServeHTTP(w, r)
					return
				}
			}

			// 2. Fallback to Bearer header (script/API compatibility)
			auth := r.Header.Get("Authorization")
			if auth != "" && strings.HasPrefix(auth, "Bearer ") {
				token := strings.TrimPrefix(auth, "Bearer ")
				if subtle.ConstantTimeCompare([]byte(token), []byte(appKey)) == 1 {
					next.ServeHTTP(w, r)
					return
				}
			}

			// 3. Neither cookie nor valid Bearer
			WriteError(w, 401, "authentication_error", "missing_app_key",
				"Missing app key")
		})
	}
}

// statusCapture wraps http.ResponseWriter to capture the response status code.
type statusCapture struct {
	http.ResponseWriter
	status int
}

func (sc *statusCapture) WriteHeader(code int) {
	sc.status = code
	sc.ResponseWriter.WriteHeader(code)
}

// AdminRateLimit returns a middleware that rate-limits admin endpoints by client IP.
// Only 401 responses count toward the failure limit. When the limit is exceeded,
// subsequent requests from that IP get 429 until the time window expires.
// Config values are read per-request for hot-reload support.
func AdminRateLimit(cfg *config.Config) func(http.Handler) http.Handler {
	var failMap sync.Map // map[string]*rateLimitEntry (IP → entry)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg == nil {
				next.ServeHTTP(w, r)
				return
			}
			maxFails := cfg.App.AdminMaxFails
			windowSec := cfg.App.AdminWindowSec
			if maxFails <= 0 || windowSec <= 0 {
				next.ServeHTTP(w, r)
				return
			}

			ip := r.RemoteAddr
			now := time.Now().Unix()

			entryI, _ := failMap.LoadOrStore(ip, &rateLimitEntry{})
			entry := entryI.(*rateLimitEntry)

			// Check if currently locked out
			windowStart := entry.windowStart.Load()
			if windowStart > 0 && now-windowStart < int64(windowSec) {
				if entry.count.Load() >= int64(maxFails) {
					retryAfter := int64(windowSec) - (now - windowStart)
					w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
					WriteError(w, 429, "rate_limit_error", "too_many_failures",
						"Too many failed authentication attempts, try again later")
					slog.Warn("admin rate limit: IP locked out",
						"ip", ip, "retry_after", retryAfter)
					return
				}
			} else if windowStart > 0 && now-windowStart >= int64(windowSec) {
				// Window expired — reset
				entry.count.Store(0)
				entry.windowStart.Store(0)
			}

			// Wrap response writer to capture status
			sc := &statusCapture{ResponseWriter: w, status: 200}
			next.ServeHTTP(sc, r)

			// Only count 401 responses as failures
			if sc.status == 401 {
				// Start window on first failure
				if entry.windowStart.Load() == 0 {
					entry.windowStart.CompareAndSwap(0, now)
				}
				count := entry.count.Add(1)
				slog.Debug("admin rate limit: auth failure recorded",
					"ip", ip, "count", count, "max", maxFails)
			}
		})
	}
}
