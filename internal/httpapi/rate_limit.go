package httpapi

import (
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/crmmc/grokforge/internal/config"
)

const (
	rateLimitCleanupInterval = 10 * time.Minute
	apiKeyRateLimitWindow    = 60 * time.Second
)

func buildAdminRateLimit(getConfig func() *config.Config) func(http.Handler) http.Handler {
	var failMap sync.Map
	startRateLimitCleanup("admin_failures_cleanup", &failMap, func() time.Duration {
		cfg := getConfig()
		if cfg == nil || cfg.App.AdminWindowSec <= 0 {
			return 0
		}
		return time.Duration(cfg.App.AdminWindowSec) * time.Second
	})

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cfg := getConfig()
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
				entry.count.Store(0)
				entry.windowStart.Store(0)
			}

			sc := &statusCapture{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sc, r)

			if sc.status != http.StatusUnauthorized {
				return
			}
			if entry.windowStart.Load() == 0 {
				entry.windowStart.CompareAndSwap(0, now)
			}
			count := entry.count.Add(1)
			slog.Debug("admin rate limit: auth failure recorded",
				"ip", ip, "count", count, "max", maxFails)
		})
	}
}

func startRateLimitCleanup(name string, entries *sync.Map, expiry func() time.Duration) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("rate limit cleanup panic recovered", "name", name, "panic", r)
			}
		}()

		ticker := time.NewTicker(rateLimitCleanupInterval)
		defer ticker.Stop()

		for range ticker.C {
			window := expiry()
			if window <= 0 {
				continue
			}
			cleanupRateLimitMap(entries, window)
		}
	}()
}

func cleanupRateLimitMap(entries *sync.Map, expiry time.Duration) {
	cutoff := time.Now().Add(-expiry).Unix()
	entries.Range(func(key, value any) bool {
		entry, ok := value.(*rateLimitEntry)
		if !ok {
			entries.Delete(key)
			return true
		}
		if entry.windowStart.Load() <= cutoff {
			entries.Delete(key)
		}
		return true
	})
}
