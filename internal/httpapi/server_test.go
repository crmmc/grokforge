package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/cache"
)

func TestServer_AdminRoutesRequireAppKey(t *testing.T) {
	srv := NewServer(&ServerConfig{
		AppKey:     "test-app-key",
		TokenStore: &mockTokenStore{},
	})

	tests := []struct {
		name       string
		path       string
		method     string
		authHeader string
		wantStatus int
	}{
		{
			name:       "admin tokens without auth returns 401",
			path:       "/admin/tokens",
			method:     http.MethodGet,
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "admin tokens with wrong key returns 401",
			path:       "/admin/tokens",
			method:     http.MethodGet,
			authHeader: "Bearer wrong-key",
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			rr := httptest.NewRecorder()
			srv.Router().ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("got status %d, want %d", rr.Code, tt.wantStatus)
			}
		})
	}
}

func TestServer_AdminRoutesWithValidKey(t *testing.T) {
	srv := NewServer(&ServerConfig{
		AppKey:     "test-app-key",
		TokenStore: &mockTokenStore{},
	})

	// With valid key, should get 200 for tokens list
	req := httptest.NewRequest(http.MethodGet, "/admin/tokens", nil)
	req.Header.Set("Authorization", "Bearer test-app-key")

	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestServer_FilesRouteWithoutAuth(t *testing.T) {
	cacheSvc := cache.NewService(t.TempDir())
	name, err := cacheSvc.SaveFile("video", []byte("mp4-data"), ".mp4")
	if err != nil {
		t.Fatalf("SaveFile() error = %v", err)
	}

	srv := NewServer(&ServerConfig{
		CacheService: cacheSvc,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/files/video/"+name, nil)
	rr := httptest.NewRecorder()
	srv.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("got status %d, want %d", rr.Code, http.StatusOK)
	}
}

func TestServer_NoChatProvider_ReturnsNotImplemented(t *testing.T) {
	srv := NewServer(&ServerConfig{})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
	req.Header.Set("Authorization", "Bearer test-api-key")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.Router().ServeHTTP(rr, req)

	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("got status %d, want %d", rr.Code, http.StatusNotImplemented)
	}
}
