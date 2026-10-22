package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helperRateLimitEntry builds a rateLimitEntry with a preset window start.
func helperRateLimitEntry(start int64) *rateLimitEntry {
	e := &rateLimitEntry{}
	e.windowStart.Store(start)
	return e
}

func TestModelWhitelistFromContext(t *testing.T) {
	t.Run("absent returns nil", func(t *testing.T) {
		assert.Nil(t, ModelWhitelistFromContext(context.Background()))
	})

	t.Run("present returns slice", func(t *testing.T) {
		wl := []string{"m1", "m2"}
		ctx := context.WithValue(context.Background(), modelWhitelistKey, wl)
		assert.Equal(t, wl, ModelWhitelistFromContext(ctx))
	})
}

func TestCheckModelWhitelist(t *testing.T) {
	tests := []struct {
		name    string
		allowed []string
		model   string
		want    bool
	}{
		{name: "empty whitelist allows all", allowed: nil, model: "any", want: true},
		{name: "model in whitelist", allowed: []string{"a", "b"}, model: "a", want: true},
		{name: "model not in whitelist", allowed: []string{"a", "b"}, model: "c", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.allowed != nil {
				ctx = context.WithValue(ctx, modelWhitelistKey, tc.allowed)
			}
			assert.Equal(t, tc.want, CheckModelWhitelist(ctx, tc.model))
		})
	}
}

func TestAPIKeyAuth_MissingOrMalformedAuthorization(t *testing.T) {
	tests := []struct {
		name string
		auth string
	}{
		{name: "no header", auth: ""},
		{name: "wrong scheme", auth: "Basic abc"},
		{name: "bearer without token", auth: "Bearer "},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			handler := APIKeyAuth(&middlewareMockStore{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
			}))

			req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
			if tc.auth != "" {
				req.Header.Set("Authorization", tc.auth)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			assert.Equal(t, http.StatusUnauthorized, w.Code)
			assert.False(t, called)
		})
	}
}

func TestAPIKeyAuth_InactiveKey(t *testing.T) {
	ms := &middlewareMockStore{
		key: &store.APIKey{ID: 2, Key: "gf-inactive", Status: "disabled"},
	}

	handler := APIKeyAuth(ms)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer gf-inactive")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "not active")
}

func TestAPIKeyAuth_WhitelistStoredInContext(t *testing.T) {
	ms := &middlewareMockStore{
		key: &store.APIKey{
			ID:             3,
			Key:            "gf-whitelisted",
			Status:         "active",
			ModelWhitelist: store.StringSlice{"grok-3", "grok-4"},
		},
	}

	var got []string
	handler := APIKeyAuth(ms)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = ModelWhitelistFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer gf-whitelisted")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{"grok-3", "grok-4"}, got)
}

func TestAPIKeyAuth_NoWhitelistContext(t *testing.T) {
	ms := &middlewareMockStore{
		key: &store.APIKey{ID: 4, Key: "gf-nowhitelist", Status: "active"},
	}

	handler := APIKeyAuth(ms)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Nil(t, ModelWhitelistFromContext(r.Context()))
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer gf-nowhitelist")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAPIKeyRateLimitWindowFn(t *testing.T) {
	assert.Equal(t, apiKeyRateLimitWindow, apiKeyRateLimitWindowFn())
}

func TestAppKeyAuthRuntime_Extra(t *testing.T) {
	t.Run("nil config rejects all", func(t *testing.T) {
		runtime := config.NewRuntime(nil)
		handler := AppKeyAuthRuntime(runtime)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/admin/config", nil)
		req.Header.Set("Authorization", "Bearer k")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("valid bearer passes", func(t *testing.T) {
		runtime := config.NewRuntime(&config.Config{App: config.AppConfig{AppKey: "runtime-key"}})
		called := false
		handler := AppKeyAuthRuntime(runtime)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/admin/config", nil)
		req.Header.Set("Authorization", "Bearer runtime-key")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, called)
	})

	t.Run("invalid bearer rejected", func(t *testing.T) {
		runtime := config.NewRuntime(&config.Config{App: config.AppConfig{AppKey: "runtime-key"}})
		handler := AppKeyAuthRuntime(runtime)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "/admin/config", nil)
		req.Header.Set("Authorization", "Bearer wrong")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestStatusCapture(t *testing.T) {
	rec := httptest.NewRecorder()
	sc := &statusCapture{ResponseWriter: rec}

	// WriteHeader records the status and forwards it.
	sc.WriteHeader(http.StatusTeapot)
	assert.Equal(t, http.StatusTeapot, sc.status)

	// Write after explicit WriteHeader does not change status.
	_, err := sc.Write([]byte("body"))
	require.NoError(t, err)
	assert.Equal(t, http.StatusTeapot, sc.status)

	// Write before any WriteHeader defaults to 200.
	rec2 := httptest.NewRecorder()
	sc2 := &statusCapture{ResponseWriter: rec2}
	_, _ = sc2.Write([]byte("x"))
	assert.Equal(t, http.StatusOK, sc2.status)
}

func TestAdminRateLimit_VerifyFailuresDoNotCount(t *testing.T) {
	cfg := &config.Config{App: config.AppConfig{AdminMaxFails: 1, AdminWindowSec: 300}}
	handler := AdminRateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))

	for i := 0; i < 10; i++ {
		req := httptest.NewRequest(http.MethodGet, "/admin/verify", nil)
		req.RemoteAddr = "9.9.9.9:1"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code, "verify 401s must not trigger lockout")
	}
}

func TestAdminRateLimit_NilConfigPassesThrough(t *testing.T) {
	handler := AdminRateLimit(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodGet, "/admin/config", nil)
	req.RemoteAddr = "8.8.8.8:2"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
}

func TestAdminRateLimit_WindowReset(t *testing.T) {
	orig := timeNow
	defer func() { timeNow = orig }()

	now := int64(1000)
	timeNow = func() time.Time { return time.Unix(now, 0) }

	cfg := &config.Config{App: config.AppConfig{AdminMaxFails: 2, AdminWindowSec: 60}}
	handler := AdminRateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))

	do := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/admin/config", nil)
		req.RemoteAddr = "7.7.7.7:5"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}

	// Two failures fill the quota inside the window.
	assert.Equal(t, http.StatusUnauthorized, do().Code)
	assert.Equal(t, http.StatusUnauthorized, do().Code)
	// Third is locked out.
	assert.Equal(t, http.StatusTooManyRequests, do().Code)

	// Advance past the window: lockout resets and the request passes through.
	now += 120
	assert.Equal(t, http.StatusUnauthorized, do().Code)
}

func TestGlobalRateLimit_NilConfigPassesThrough(t *testing.T) {
	handler := GlobalRateLimit(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.RemoteAddr = "6.6.6.6:7"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusAccepted, w.Code)
}

func TestGlobalRateLimit_HealthzExempt(t *testing.T) {
	cfg := &config.Config{App: config.AppConfig{GlobalRateLimitRPM: 1, GlobalRateLimitWindow: 60}}
	handler := GlobalRateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		req.RemoteAddr = "5.5.5.5:9"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "/healthz must be exempt from global rate limit")
	}
}

func TestGlobalRateLimit_WindowReset(t *testing.T) {
	orig := timeNow
	defer func() { timeNow = orig }()

	now := int64(2000)
	timeNow = func() time.Time { return time.Unix(now, 0) }

	cfg := &config.Config{App: config.AppConfig{GlobalRateLimitRPM: 1, GlobalRateLimitWindow: 30}}
	handler := GlobalRateLimit(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	do := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		req.RemoteAddr = "4.4.4.4:3"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}

	assert.Equal(t, http.StatusOK, do().Code)
	assert.Equal(t, http.StatusTooManyRequests, do().Code)

	now += 60
	assert.Equal(t, http.StatusOK, do().Code, "window reset must allow requests again")
}

func TestCleanupExpiryResolvers(t *testing.T) {
	t.Run("admin expiry", func(t *testing.T) {
		assert.Equal(t, time.Duration(0), adminCleanupExpiry(func() *config.Config { return nil })())
		assert.Equal(t, time.Duration(0), adminCleanupExpiry(func() *config.Config { return &config.Config{} })())
		cfg := &config.Config{App: config.AppConfig{AdminWindowSec: 90}}
		assert.Equal(t, 90*time.Second, adminCleanupExpiry(func() *config.Config { return cfg })())
	})

	t.Run("global expiry", func(t *testing.T) {
		assert.Equal(t, time.Duration(0), globalCleanupExpiry(func() *config.Config { return nil })())
		assert.Equal(t, time.Duration(0), globalCleanupExpiry(func() *config.Config { return &config.Config{} })())
		cfg := &config.Config{App: config.AppConfig{GlobalRateLimitWindow: 45}}
		assert.Equal(t, 45*time.Second, globalCleanupExpiry(func() *config.Config { return cfg })())
	})
}

func TestCleanupTickerLoop(t *testing.T) {
	t.Run("cleans entries on tick", func(t *testing.T) {
		entries := &sync.Map{}
		entries.Store("old", helperRateLimitEntry(time.Unix(100, 0).Unix()))
		entries.Store("fresh", helperRateLimitEntry(time.Now().Unix()))

		tick := make(chan time.Time)
		done := make(chan struct{})
		go func() {
			cleanupTickerLoop("test", tick, entries, func() time.Duration { return time.Minute })
			close(done)
		}()

		tick <- time.Now()
		close(tick)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("cleanupTickerLoop did not exit")
		}

		_, oldExists := entries.Load("old")
		assert.False(t, oldExists, "expired entry must be removed")
		_, freshExists := entries.Load("fresh")
		assert.True(t, freshExists, "recent entry must be kept")
	})

	t.Run("zero window skips cleanup", func(t *testing.T) {
		entries := &sync.Map{}
		entries.Store("keep", helperRateLimitEntry(1))

		tick := make(chan time.Time)
		done := make(chan struct{})
		go func() {
			cleanupTickerLoop("test", tick, entries, func() time.Duration { return 0 })
			close(done)
		}()

		tick <- time.Now()
		close(tick)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("cleanupTickerLoop did not exit")
		}

		_, exists := entries.Load("keep")
		assert.True(t, exists, "zero window must skip cleanup")
	})

	t.Run("panicking expiry is recovered", func(t *testing.T) {
		tick := make(chan time.Time)
		done := make(chan struct{})
		go func() {
			defer close(done)
			cleanupTickerLoop("panic-test", tick, &sync.Map{}, func() time.Duration {
				panic(errors.New("boom"))
			})
		}()

		tick <- time.Now()
		close(tick)
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("panic was not recovered")
		}
	})
}

func TestCleanupRateLimitMap(t *testing.T) {
	orig := timeNow
	defer func() { timeNow = orig }()
	now := time.Unix(5000, 0)
	timeNow = func() time.Time { return now }

	entries := &sync.Map{}
	entries.Store("stale", helperRateLimitEntry(4000)) // older than cutoff
	entries.Store("fresh", helperRateLimitEntry(4995)) // within expiry
	entries.Store("junk", "not-an-entry")              // wrong type

	cleanupRateLimitMap(entries, time.Minute)

	_, exists := entries.Load("stale")
	assert.False(t, exists)
	_, exists = entries.Load("fresh")
	assert.True(t, exists)
	_, exists = entries.Load("junk")
	assert.False(t, exists, "non-entry values must be deleted")
}
