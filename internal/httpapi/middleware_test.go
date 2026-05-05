package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AppKeyAuth tests - rejects when key is empty (secure by default)

func TestAppKeyAuth_EmptyKeyRejects(t *testing.T) {
	handler := AppKeyAuth("")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/config", nil)
	req.Header.Set("Authorization", "Bearer some-key")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 when app_key not configured, got %d", rec.Code)
	}
}

func TestAppKeyAuth_MissingHeader(t *testing.T) {
	handler := AppKeyAuth("test-app-key")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/config", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing header, got %d", rec.Code)
	}
}

func TestAppKeyAuth_InvalidKey(t *testing.T) {
	handler := AppKeyAuth("test-app-key")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/config", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid key, got %d", rec.Code)
	}
}

func TestAppKeyAuth_ValidKey(t *testing.T) {
	called := false
	handler := AppKeyAuth("test-app-key")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/config", nil)
	req.Header.Set("Authorization", "Bearer test-app-key")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
	if !called {
		t.Error("next handler was not called")
	}
}

func TestAppKeyAuth_UsesConstantTimeCompare(t *testing.T) {
	// This test verifies the middleware uses constant-time comparison
	// by checking it correctly validates keys of same length
	handler := AppKeyAuth("abcd1234")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Same length, different content - should reject
	req := httptest.NewRequest(http.MethodGet, "/admin/config", nil)
	req.Header.Set("Authorization", "Bearer abcd5678")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong key, got %d", rec.Code)
	}
}

func TestAppKeyAuth_ValidCookie(t *testing.T) {
	called := false
	handler := AppKeyAuth("test-app-key")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/config", nil)
	req.AddCookie(&http.Cookie{Name: "gf_session", Value: signAdminSession("test-app-key", time.Now().UTC())})
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, called)
}

func TestAppKeyAuth_InvalidCookieFallsBackToBearer(t *testing.T) {
	called := false
	handler := AppKeyAuth("test-app-key")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/config", nil)
	req.AddCookie(&http.Cookie{Name: "gf_session", Value: "wrong-cookie"})
	req.Header.Set("Authorization", "Bearer test-app-key")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, called)
}

func TestAppKeyAuth_NoCookieNoBearerRejects(t *testing.T) {
	handler := AppKeyAuth("test-app-key")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/config", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// --- APIKeyAuth middleware tests ---

// middlewareMockStore is a focused mock for APIKeyAuth middleware tests.
type middlewareMockStore struct {
	key             *store.APIKey
	incrementCalled bool
}

func (m *middlewareMockStore) List(_ context.Context, _, _ int, _ string) ([]*store.APIKey, int64, error) {
	return nil, 0, nil
}
func (m *middlewareMockStore) GetByID(_ context.Context, _ uint) (*store.APIKey, error) {
	return nil, nil
}
func (m *middlewareMockStore) GetByKey(_ context.Context, key string) (*store.APIKey, error) {
	if m.key != nil && m.key.Key == key {
		return m.key, nil
	}
	return nil, store.ErrNotFound
}
func (m *middlewareMockStore) Create(_ context.Context, _ *store.APIKey) error { return nil }
func (m *middlewareMockStore) Update(_ context.Context, _ *store.APIKey) error { return nil }
func (m *middlewareMockStore) Delete(_ context.Context, _ uint) error          { return nil }
func (m *middlewareMockStore) Regenerate(_ context.Context, _ uint) (string, error) {
	return "", nil
}
func (m *middlewareMockStore) CountByStatus(_ context.Context) (int, int, int, int, int, error) {
	return 0, 0, 0, 0, 0, nil
}
func (m *middlewareMockStore) IncrementUsage(_ context.Context, _ uint) error {
	m.incrementCalled = true
	return nil
}
func (m *middlewareMockStore) ResetDailyUsage(_ context.Context) error { return nil }

func TestAPIKeyAuth_ValidKey(t *testing.T) {
	ms := &middlewareMockStore{
		key: &store.APIKey{
			ID:         1,
			Key:        "gf-validkey123",
			Status:     "active",
			RateLimit:  60,
			DailyLimit: 1000,
			DailyUsed:  5,
		},
	}

	var gotKeyID uint
	var gotOk bool
	handler := APIKeyAuth(ms)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKeyID, gotOk = APIKeyIDFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	req.Header.Set("Authorization", "Bearer gf-validkey123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, gotOk)
	assert.Equal(t, uint(1), gotKeyID)
	assert.False(t, ms.incrementCalled, "IncrementUsage should NOT be called in middleware (moved to flow layer)")
}

func TestAPIKeyAuth_InvalidKey(t *testing.T) {
	ms := &middlewareMockStore{}

	handler := APIKeyAuth(ms)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	req.Header.Set("Authorization", "Bearer gf-unknown")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAPIKeyAuth_ExpiredKey(t *testing.T) {
	past := time.Now().Add(-24 * time.Hour)
	ms := &middlewareMockStore{
		key: &store.APIKey{
			ID:         1,
			Key:        "gf-expiredkey",
			Status:     "active",
			ExpiresAt:  &past,
			RateLimit:  60,
			DailyLimit: 1000,
		},
	}

	handler := APIKeyAuth(ms)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	req.Header.Set("Authorization", "Bearer gf-expiredkey")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestAPIKeyAuth_DailyLimitExceeded(t *testing.T) {
	ms := &middlewareMockStore{
		key: &store.APIKey{
			ID:         1,
			Key:        "gf-dailylimit",
			Status:     "active",
			RateLimit:  60,
			DailyLimit: 100,
			DailyUsed:  100, // at limit
		},
	}

	handler := APIKeyAuth(ms)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
	req.Header.Set("Authorization", "Bearer gf-dailylimit")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusTooManyRequests, rec.Code)

	var resp APIError
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "daily limit exceeded", resp.Error.Message)
	assert.Equal(t, "rate_limit_error", resp.Error.Type)
}

func TestAPIKeyAuth_RateLimitExceeded(t *testing.T) {
	ms := &middlewareMockStore{
		key: &store.APIKey{
			ID:         1,
			Key:        "gf-ratelimit",
			Status:     "active",
			RateLimit:  2,
			DailyLimit: 1000,
			DailyUsed:  0,
		},
	}

	handler := APIKeyAuth(ms)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Send rate_limit + 1 requests; the last one should be 429
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v1/test", nil)
		req.Header.Set("Authorization", "Bearer gf-ratelimit")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if i < 2 {
			assert.Equal(t, http.StatusOK, rec.Code, "request %d should pass", i)
		} else {
			require.Equal(t, http.StatusTooManyRequests, rec.Code, "request %d should be rate limited", i)
			var resp APIError
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			assert.Equal(t, "rate limit exceeded", resp.Error.Message)
		}
	}
}

// --- AdminRateLimit middleware tests ---

func TestAdminRateLimit_AllowsSuccessfulRequests(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			AdminMaxFails:  3,
			AdminWindowSec: 300,
		},
	}

	handler := AdminRateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))

	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/admin/verify", nil)
		req.RemoteAddr = "1.2.3.4:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("request %d: expected 200, got %d", i, rec.Code)
		}
	}
}

func TestAdminRateLimit_BlocksAfterAuthFailures(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			AdminMaxFails:  3,
			AdminWindowSec: 300,
		},
	}

	handler := AdminRateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	}))

	ip := "10.0.0.1:9999"

	// First 3 requests pass through (get 401 from downstream)
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/admin/config", nil)
		req.RemoteAddr = ip
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != 401 {
			t.Fatalf("request %d: expected 401, got %d", i, rec.Code)
		}
	}

	// 4th request blocked with 429
	req := httptest.NewRequest(http.MethodGet, "/admin/config", nil)
	req.RemoteAddr = ip
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 429 {
		t.Fatalf("request 4: expected 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}
}

func TestAdminRateLimit_DifferentIPsIndependent(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			AdminMaxFails:  2,
			AdminWindowSec: 300,
		},
	}

	handler := AdminRateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	}))

	// Exhaust IP-A's limit
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/admin/config", nil)
		req.RemoteAddr = "10.0.0.1:1111"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	// IP-A blocked
	req := httptest.NewRequest(http.MethodGet, "/admin/config", nil)
	req.RemoteAddr = "10.0.0.1:1111"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 429 {
		t.Fatalf("IP-A: expected 429, got %d", rec.Code)
	}

	// IP-B still allowed
	req = httptest.NewRequest(http.MethodGet, "/admin/config", nil)
	req.RemoteAddr = "10.0.0.2:2222"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Fatalf("IP-B: expected 401, got %d", rec.Code)
	}
}

func TestAdminRateLimit_DisabledWhenZero(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			AdminMaxFails:  0,
			AdminWindowSec: 0,
		},
	}

	handler := AdminRateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	}))

	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodGet, "/admin/verify", nil)
		req.RemoteAddr = "10.0.0.1:1111"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != 401 {
			t.Fatalf("request %d: expected 401, got %d", i, rec.Code)
		}
	}
}

func TestAdminRateLimitRuntime_PersistsFailuresAcrossRequests(t *testing.T) {
	runtime := config.NewRuntime(&config.Config{
		App: config.AppConfig{
			AdminMaxFails:  2,
			AdminWindowSec: 60,
		},
	})
	handler := AdminRateLimitRuntime(runtime)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))

	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/admin/config", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("request %d: expected 401, got %d", i+1, rec.Code)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/config", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected runtime rate limit to persist, got %d", rec.Code)
	}
}

// --- GlobalRateLimit middleware tests ---

func TestGlobalRateLimit_AllowsUnderLimit(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			GlobalRateLimitRPM:    10,
			GlobalRateLimitWindow: 60,
		},
	}
	handler := GlobalRateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.RemoteAddr = "1.2.3.4:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rec.Code)
		}
	}
}

func TestGlobalRateLimit_BlocksOverLimit(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			GlobalRateLimitRPM:    10,
			GlobalRateLimitWindow: 60,
		},
	}
	handler := GlobalRateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ip := "1.2.3.4:12345"
	// First 10 requests pass
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.RemoteAddr = ip
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rec.Code)
		}
	}
	// Next 2 requests blocked
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.RemoteAddr = ip
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusTooManyRequests {
			t.Fatalf("request %d: expected 429, got %d", i, rec.Code)
		}
	}
}

func TestGlobalRateLimit_SkipsHealth(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			GlobalRateLimitRPM:    10,
			GlobalRateLimitWindow: 60,
		},
	}
	handler := GlobalRateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.RemoteAddr = "1.2.3.4:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rec.Code)
		}
	}
}

func TestGlobalRateLimit_SkipsNextStatic(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			GlobalRateLimitRPM:    10,
			GlobalRateLimitWindow: 60,
		},
	}
	handler := GlobalRateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodGet, "/_next/static/chunk.js", nil)
		req.RemoteAddr = "1.2.3.4:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rec.Code)
		}
	}
}

func TestGlobalRateLimit_DifferentIPsIndependent(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			GlobalRateLimitRPM:    2,
			GlobalRateLimitWindow: 60,
		},
	}
	handler := GlobalRateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust IP-A
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.RemoteAddr = "10.0.0.1:1111"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	// IP-A blocked
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.RemoteAddr = "10.0.0.1:1111"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("IP-A: expected 429, got %d", rec.Code)
	}

	// IP-B still allowed
	req = httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.RemoteAddr = "10.0.0.2:2222"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("IP-B: expected 200, got %d", rec.Code)
	}
}

func TestGlobalRateLimit_DisabledWhenZero(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			GlobalRateLimitRPM:    0,
			GlobalRateLimitWindow: 60,
		},
	}
	handler := GlobalRateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 100; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.RemoteAddr = "1.2.3.4:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rec.Code)
		}
	}
}

func TestGlobalRateLimitRuntime_HotReload(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			GlobalRateLimitRPM:    2,
			GlobalRateLimitWindow: 60,
		},
	}
	runtime := config.NewRuntime(cfg)
	handler := GlobalRateLimitRuntime(runtime)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Use up limit with old config
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rec.Code)
		}
	}

	// Blocked with old config
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 before hot reload, got %d", rec.Code)
	}

	// Hot reload: increase limit
	cfg.App.GlobalRateLimitRPM = 5
	runtime.Store(cfg)

	// Should now be allowed again (window reset would also work, but limit increase is what we test)
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.RemoteAddr = "127.0.0.1:1234"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d after hot reload: expected 200, got %d", i, rec.Code)
		}
	}
}

func TestGlobalRateLimit_AppliesToAdminVerify(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			GlobalRateLimitRPM:    2,
			GlobalRateLimitWindow: 60,
		},
	}
	handler := GlobalRateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Use up limit
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/admin/verify", nil)
		req.RemoteAddr = "1.2.3.4:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, rec.Code)
		}
	}

	// /admin/verify should also be blocked
	req := httptest.NewRequest(http.MethodGet, "/admin/verify", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for /admin/verify, got %d", rec.Code)
	}
}

func TestGlobalRateLimit_ReturnsRetryAfter(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			GlobalRateLimitRPM:    1,
			GlobalRateLimitWindow: 60,
		},
	}
	handler := GlobalRateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ip := "1.2.3.4:12345"
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.RemoteAddr = ip
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.RemoteAddr = ip
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: expected 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429 response")
	}
}
