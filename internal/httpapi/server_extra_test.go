package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
)

// activeStoreToken builds a valid active store.Token for server tests.
func activeStoreToken() *store.Token {
	return &store.Token{Token: longToken("srv_"), Pool: "ssoBasic", Status: store.TokenStatusActive}
}

// recordingChatProvider installs echo routes and records SetupRoutes calls.
type recordingChatProvider struct {
	setupCalled bool
}

func (p *recordingChatProvider) SetupRoutes(r chi.Router) {
	p.setupCalled = true
	r.Post("/chat/completions", func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]string{"ok": "true"})
	})
	r.Get("/models", func(w http.ResponseWriter, _ *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]any{"data": []string{"m1"}})
	})
}

// newFullTestServer builds a Server wired with every dependency and runtime config.
func newFullTestServer(t *testing.T) (*Server, string) {
	t.Helper()

	cacheSvc, cacheBase := newTestCacheService(t)
	createCacheFile(t, cacheBase, "video", "smoke.mp4", 16)

	cfg := config.DefaultConfig()
	cfg.App.AppKey = "full-app-key"

	ts := newMockTokenStore()
	require.NoError(t, ts.CreateToken(context.Background(), activeStoreToken()))
	aks := newMockAPIKeyStore()
	require.NoError(t, aks.Create(context.Background(), &store.APIKey{
		Name:   "test-key",
		Key:    "gf-live-api-key",
		Status: "active",
	}))
	usage := &mockUsageLogStore{
		todayCounts:     map[string]int{"chat": 1},
		yesterdayCounts: map[string]int{},
	}
	provider := &recordingChatProvider{}

	srv := NewServer(&ServerConfig{
		AppKey:          cfg.App.AppKey,
		Version:         "test-1.0",
		Config:          cfg,
		Runtime:         config.NewRuntime(cfg),
		ChatProvider:    provider,
		TokenStore:      ts,
		TokenRefresher:  &errRefresher{token: activeStoreToken()},
		TokenPoolSyncer: &fakePoolSyncer{},
		TokenInflight:   &fakeInflight{},
		TokenCfgSync:    func(*config.TokenConfig) {},
		NsfwEnabler:     nil,
		UsageLogStore:   usage,
		APIKeyStore:     aks,
		CacheService:    cacheSvc,
		ModelRegistry:   adminTestRegistry(),
	})
	require.True(t, provider.setupCalled, "chat provider routes must be installed")
	return srv, aks.keys[0].Key
}

func TestNewServer_NilConfig(t *testing.T) {
	srv := NewServer(nil)
	require.NotNil(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestNewServer_NilServerConfigUsesNoopProvider(t *testing.T) {
	srv := NewServer(&ServerConfig{})

	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`)))
	assert.Equal(t, http.StatusNotImplemented, w.Code)
	assert.Contains(t, w.Body.String(), "not_implemented")
}

func TestFullServer_HealthEndpoints(t *testing.T) {
	srv, _ := newFullTestServer(t)

	for _, path := range []string{"/health", "/healthz"} {
		t.Run(path, func(t *testing.T) {
			w := httptest.NewRecorder()
			srv.Router().ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
			require.Equal(t, http.StatusOK, w.Code)

			var resp HealthResponse
			require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
			assert.Equal(t, "ok", resp.Status)
			assert.NotEmpty(t, resp.Uptime)
			assert.NotEmpty(t, resp.Timestamp)
		})
	}
}

func TestFullServer_AdminRoutesSmoke(t *testing.T) {
	srv, _ := newFullTestServer(t)

	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
	}{
		{name: "verify", method: http.MethodGet, path: "/admin/verify", wantStatus: http.StatusOK},
		{name: "system status", method: http.MethodGet, path: "/admin/system/status", wantStatus: http.StatusOK},
		{name: "get config runtime", method: http.MethodGet, path: "/admin/config", wantStatus: http.StatusOK},
		{name: "put config runtime", method: http.MethodPut, path: "/admin/config", body: `{"console":{"enabled":true}}`, wantStatus: http.StatusOK},
		{name: "list tokens", method: http.MethodGet, path: "/admin/tokens", wantStatus: http.StatusOK},
		{name: "list token ids", method: http.MethodGet, path: "/admin/tokens/ids", wantStatus: http.StatusOK},
		{name: "get token", method: http.MethodGet, path: "/admin/tokens/1", wantStatus: http.StatusOK},
		{name: "update token", method: http.MethodPut, path: "/admin/tokens/1", body: `{"remark":"hi"}`, wantStatus: http.StatusOK},
		{name: "refresh token", method: http.MethodPost, path: "/admin/tokens/1/refresh", wantStatus: http.StatusOK},
		{name: "delete token", method: http.MethodDelete, path: "/admin/tokens/1", wantStatus: http.StatusNoContent},
		{name: "batch tokens", method: http.MethodPost, path: "/admin/tokens/batch", body: `{"operation":"import","tokens":["batch_long_token_abcdef123456"],"pool":"basic"}`, wantStatus: http.StatusOK},
		{name: "batch refresh", method: http.MethodPost, path: "/admin/tokens/batch/refresh", body: `{"ids":[1]}`, wantStatus: http.StatusOK},
		{name: "token stats", method: http.MethodGet, path: "/admin/stats/tokens", wantStatus: http.StatusOK},
		{name: "quota stats", method: http.MethodGet, path: "/admin/stats/quota", wantStatus: http.StatusOK},
		{name: "usage stats", method: http.MethodGet, path: "/admin/stats/usage", wantStatus: http.StatusOK},
		{name: "system usage", method: http.MethodGet, path: "/admin/system/usage", wantStatus: http.StatusOK},
		{name: "usage logs", method: http.MethodGet, path: "/admin/usage/logs", wantStatus: http.StatusOK},
		{name: "list api keys", method: http.MethodGet, path: "/admin/apikeys/", wantStatus: http.StatusOK},
		{name: "api key stats", method: http.MethodGet, path: "/admin/apikeys/stats", wantStatus: http.StatusOK},
		{name: "create api key", method: http.MethodPost, path: "/admin/apikeys/", body: `{"name":"n"}`, wantStatus: http.StatusCreated},
		{name: "get api key", method: http.MethodGet, path: "/admin/apikeys/1", wantStatus: http.StatusOK},
		{name: "patch api key", method: http.MethodPatch, path: "/admin/apikeys/1", body: `{"name":"m"}`, wantStatus: http.StatusOK},
		{name: "regenerate api key", method: http.MethodPost, path: "/admin/apikeys/1/regenerate", wantStatus: http.StatusOK},
		{name: "delete api key", method: http.MethodDelete, path: "/admin/apikeys/1", wantStatus: http.StatusNoContent},
		{name: "cache stats", method: http.MethodGet, path: "/admin/cache/stats", wantStatus: http.StatusOK},
		{name: "cache files", method: http.MethodGet, path: "/admin/cache/files?type=video", wantStatus: http.StatusOK},
		{name: "cache file serve", method: http.MethodGet, path: "/admin/cache/files/video/smoke.mp4", wantStatus: http.StatusOK},
		{name: "cache delete", method: http.MethodPost, path: "/admin/cache/delete", body: `{"type":"video","names":["smoke.mp4"]}`, wantStatus: http.StatusOK},
		{name: "cache clear", method: http.MethodPost, path: "/admin/cache/clear", body: `{"type":"video"}`, wantStatus: http.StatusOK},
		{name: "proxy browsers", method: http.MethodGet, path: "/admin/proxy/browsers", wantStatus: http.StatusOK},
		{name: "model catalog", method: http.MethodGet, path: "/admin/models", wantStatus: http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var req *http.Request
			if tc.body != "" {
				req = httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			} else {
				req = httptest.NewRequest(tc.method, tc.path, nil)
			}
			req.Header = http.Header{"Authorization": {"Bearer full-app-key"}}
			w := httptest.NewRecorder()
			srv.Router().ServeHTTP(w, req)
			require.Equal(t, tc.wantStatus, w.Code, "body: %s", w.Body.String())
		})
	}
}

func TestFullServer_PublicRoutes(t *testing.T) {
	srv, liveAPIKey := newFullTestServer(t)

	t.Run("public cache file without auth", func(t *testing.T) {
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/files/video/smoke.mp4", nil))
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("chat completions with api key auth", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`))
		req.Header.Set("Authorization", "Bearer "+liveAPIKey)
		srv.Router().ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("chat completions without api key rejected", func(t *testing.T) {
		w := httptest.NewRecorder()
		srv.Router().ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{}`)))
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("v1 models with api key", func(t *testing.T) {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req.Header.Set("Authorization", "Bearer "+liveAPIKey)
		srv.Router().ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestFullServer_TokenCfgSyncCalledOnConfigUpdate(t *testing.T) {
	synced := make(chan *config.TokenConfig, 1)
	cfg := config.DefaultConfig()
	cfg.App.AppKey = "sync-key"

	srv := NewServer(&ServerConfig{
		AppKey:     cfg.App.AppKey,
		Runtime:    config.NewRuntime(cfg),
		Config:     cfg,
		TokenStore: newMockTokenStore(),
		TokenCfgSync: func(tc *config.TokenConfig) {
			synced <- tc
		},
	})

	req := httptest.NewRequest(http.MethodPut, "/admin/config", strings.NewReader(`{"token":{"fail_threshold":3}}`))
	req.Header.Set("Authorization", "Bearer sync-key")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code)

	select {
	case tc := <-synced:
		assert.Equal(t, 3, tc.FailThreshold)
	default:
		t.Fatal("tokenCfgSync was not called")
	}
}

func TestFullServer_ResourcesWithoutDepsReturn404(t *testing.T) {
	// A server without the relevant dependencies must not panic on missing routes;
	// chi returns 404 for unregistered paths.
	srv := NewServer(&ServerConfig{AppKey: "k"})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/tokens", nil)
	req.Header.Set("Authorization", "Bearer k")
	srv.Router().ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestServer_UnknownAdminTokenRefreshErrorMapsTo502(t *testing.T) {
	srv := NewServer(&ServerConfig{
		AppKey:         "k",
		TokenStore:     newMockTokenStore(),
		TokenRefresher: &errRefresher{err: errors.New("upstream down")},
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/tokens/1/refresh", nil)
	req.Header.Set("Authorization", "Bearer k")
	w := httptest.NewRecorder()
	srv.Router().ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadGateway, w.Code)
}
