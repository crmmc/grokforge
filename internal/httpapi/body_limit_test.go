package httpapi

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/config"
)

func TestBodySizeLimitMiddleware_NormalRequest(t *testing.T) {
	cfg := config.DefaultConfig()
	handler := bodySizeLimitMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))

	body := bytes.NewReader(make([]byte, 1024)) // 1KB
	req := httptest.NewRequest(http.MethodPost, "/v1/tokens", body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestBodySizeLimitMiddleware_OversizedGeneral(t *testing.T) {
	cfg := config.DefaultConfig()
	handler := bodySizeLimitMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	body := bytes.NewReader(make([]byte, 2<<20)) // 2MB > 1MB limit
	req := httptest.NewRequest(http.MethodPost, "/v1/tokens", body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d", rec.Code)
	}
}

func TestBodySizeLimitMiddleware_ChatCompletionsLargeBody(t *testing.T) {
	cfg := config.DefaultConfig()
	handler := bodySizeLimitMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))

	body := bytes.NewReader(make([]byte, 5<<20)) // 5MB < 10MB chat limit
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestRouteBodyLimit(t *testing.T) {
	cfg := config.DefaultConfig()

	tests := []struct {
		method string
		path   string
		want   int64
	}{
		{http.MethodPost, "/v1/chat/completions", cfg.App.ChatBodyLimit},
		{http.MethodPost, "/v1/tokens", cfg.App.BodyLimit},
		{http.MethodGet, "/v1/chat/completions", cfg.App.BodyLimit},
		{http.MethodPut, "/api/config", cfg.App.BodyLimit},
	}

	for _, tt := range tests {
		got := routeBodyLimit(cfg, tt.method, tt.path)
		if got != tt.want {
			t.Errorf("routeBodyLimit(%s, %s) = %d, want %d", tt.method, tt.path, got, tt.want)
		}
	}
}

func TestRouteBodyLimit_CustomConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.App.BodyLimit = 512 * 1024       // 512KB
	cfg.App.ChatBodyLimit = 20 << 20     // 20MB

	if got := routeBodyLimit(cfg, http.MethodPost, "/v1/tokens"); got != 512*1024 {
		t.Errorf("custom body limit = %d, want %d", got, 512*1024)
	}
	if got := routeBodyLimit(cfg, http.MethodPost, "/v1/chat/completions"); got != 20<<20 {
		t.Errorf("custom chat body limit = %d, want %d", got, 20<<20)
	}
}
