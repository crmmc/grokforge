package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/crmmc/grokforge/internal/config"
)

const defaultRequestTimeout = 60 * time.Second

// requestTimeoutMiddleware creates a middleware that applies per-route
// request timeouts via context deadline.
//
// For chat completions the timeout is read from cfg.Proxy.Timeout (default 300s).
// All other routes use cfg.App.RequestTimeout (default 60s).
// Both values are hot-reloadable.
func requestTimeoutMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			timeout := routeTimeout(cfg, r.Method, r.URL.Path)

			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// routeTimeout returns the request timeout for the given route.
// Chat completions use the configurable proxy.timeout (default 300s);
// all other routes use the configurable app.request_timeout (default 60s).
func routeTimeout(cfg *config.Config, method, path string) time.Duration {
	if method == http.MethodPost && path == "/v1/chat/completions" {
		if cfg != nil && cfg.Proxy.Timeout > 0 {
			return time.Duration(cfg.Proxy.Timeout) * time.Second
		}
		return defaultRequestTimeout
	}
	if cfg != nil && cfg.App.RequestTimeout > 0 {
		return time.Duration(cfg.App.RequestTimeout) * time.Second
	}
	return defaultRequestTimeout
}
