package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/flow"
)

func TestRouteTimeout(t *testing.T) {
	cfg := &config.Config{
		App:   config.AppConfig{RequestTimeout: 90},
		Proxy: config.ProxyConfig{Timeout: 180},
	}

	tests := []struct {
		name   string
		method string
		path   string
		want   time.Duration
	}{
		{name: "chat uses proxy.timeout", method: http.MethodPost, path: "/v1/chat/completions", want: 180 * time.Second},
		{name: "other POST uses app.request_timeout", method: http.MethodPost, path: "/admin/tokens/batch", want: 90 * time.Second},
		{name: "GET uses app.request_timeout", method: http.MethodGet, path: "/v1/chat/completions", want: 90 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := routeTimeout(cfg, tt.method, tt.path)
			if got != tt.want {
				t.Fatalf("routeTimeout() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestRouteTimeout_NilConfig(t *testing.T) {
	got := routeTimeout(nil, http.MethodPost, "/v1/chat/completions")
	if got != defaultRequestTimeout {
		t.Fatalf("routeTimeout(nil) = %s, want %s", got, defaultRequestTimeout)
	}
}

func TestRouteTimeout_ZeroProxyTimeout(t *testing.T) {
	cfg := &config.Config{Proxy: config.ProxyConfig{Timeout: 0}}
	got := routeTimeout(cfg, http.MethodPost, "/v1/chat/completions")
	if got != defaultRequestTimeout {
		t.Fatalf("routeTimeout(zero) = %s, want %s", got, defaultRequestTimeout)
	}
}

func TestRouteTimeout_ZeroRequestTimeout(t *testing.T) {
	cfg := &config.Config{App: config.AppConfig{RequestTimeout: 0}}
	got := routeTimeout(cfg, http.MethodGet, "/admin/tokens")
	if got != defaultRequestTimeout {
		t.Fatalf("routeTimeout(zero request_timeout) = %s, want %s", got, defaultRequestTimeout)
	}
}

func TestRequestTimeoutMiddleware_UsesRouteSpecificDeadline(t *testing.T) {
	cfg := &config.Config{
		App:   config.AppConfig{RequestTimeout: 90},
		Proxy: config.ProxyConfig{Timeout: 180},
	}

	handler := requestTimeoutMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deadline, ok := r.Context().Deadline()
		if !ok {
			t.Fatal("expected context deadline")
		}
		remaining := time.Until(deadline)
		_, _ = w.Write([]byte(strconv.FormatInt(int64(remaining/time.Second), 10)))
	}))

	tests := []struct {
		name string
		path string
		want time.Duration
	}{
		{name: "chat route uses proxy.timeout", path: "/v1/chat/completions", want: 180 * time.Second},
		{name: "default route uses app.request_timeout", path: "/admin/tokens", want: 90 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			gotSeconds, err := strconv.Atoi(rr.Body.String())
			if err != nil {
				t.Fatalf("parse response: %v", err)
			}
			got := time.Duration(gotSeconds) * time.Second
			if got < tt.want-2*time.Second || got > tt.want {
				t.Fatalf("deadline = %s, want around %s", got, tt.want)
			}
		})
	}
}

func TestRequestTimeoutMiddleware_HotReload(t *testing.T) {
	cfg := &config.Config{Proxy: config.ProxyConfig{Timeout: 120}}
	handler := requestTimeoutMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deadline, _ := r.Context().Deadline()
		remaining := time.Until(deadline)
		_, _ = w.Write([]byte(strconv.FormatInt(int64(remaining/time.Second), 10)))
	}))

	// First request: 120s
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	got, _ := strconv.Atoi(rr.Body.String())
	if got < 118 || got > 120 {
		t.Fatalf("before hot-reload: deadline = %ds, want ~120s", got)
	}

	// Hot-reload: change timeout to 600s
	cfg.Proxy.Timeout = 600

	req = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	got, _ = strconv.Atoi(rr.Body.String())
	if got < 598 || got > 600 {
		t.Fatalf("after hot-reload: deadline = %ds, want ~600s", got)
	}
}

func TestBridgeFlowContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), apiKeyIDKey, uint(7))
	bridged := BridgeFlowContext(ctx)
	id := flow.FlowAPIKeyIDFromContext(bridged)
	if id != 7 {
		t.Fatalf("FlowAPIKeyIDFromContext() = %d, want 7", id)
	}
}
