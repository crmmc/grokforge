package httpapi

import (
	"net/http"

	"github.com/crmmc/grokforge/internal/config"
)

// bodySizeLimitMiddleware wraps r.Body with http.MaxBytesReader based on the route.
// Downstream json.Decode will automatically surface http.MaxBytesError on overflow.
func bodySizeLimitMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limit := routeBodyLimit(cfg, r.Method, r.URL.Path)
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

func bodySizeLimitRuntimeMiddleware(runtime *config.Runtime) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			limit := routeBodyLimit(runtime.Get(), r.Method, r.URL.Path)
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}

// routeBodyLimit returns the maximum body size for a given route.
// Values are read from config for hot-reload support.
func routeBodyLimit(cfg *config.Config, method, path string) int64 {
	if method == http.MethodPost && path == "/v1/chat/completions" {
		if cfg != nil && cfg.App.ChatBodyLimit > 0 {
			return cfg.App.ChatBodyLimit
		}
		return 10 << 20 // 10MB fallback
	}
	if cfg != nil && cfg.App.BodyLimit > 0 {
		return cfg.App.BodyLimit
	}
	return 1 << 20 // 1MB fallback
}
