package token

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/modelconfig"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduler_Stop(t *testing.T) {
	manager := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	scheduler := NewScheduler(manager, nil, "https://example.com")

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)
	time.Sleep(30 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		cancel()
		scheduler.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Error("Stop() blocked for too long")
	}
}

func TestScheduler_ScanRefreshesDueExhaustedMode(t *testing.T) {
	manager := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	manager.AddToken(&store.Token{
		ID: 1, Token: "test-token", Pool: PoolSuper, Status: string(StatusActive),
		Quotas: store.IntMap{"auto": 0}, ResumeAts: store.IntMap{"auto": 0},
	})

	server := rateLimitServer(t, RateLimitsResponse{
		RemainingQueries:  42,
		TotalQueries:      80,
		WindowSizeSeconds: 7200,
	}, nil)
	defer server.Close()

	scheduler := NewScheduler(manager, testModeSpecs(), server.URL)
	scheduler.scan(context.Background())

	waitForCondition(t, func() bool {
		return manager.GetToken(1).Quotas["auto"] == 42
	})
	assert.Equal(t, 80, manager.GetToken(1).LimitQuotas["auto"])
	assert.Empty(t, manager.GetToken(1).ResumeAts)
}

func TestScheduler_RequestRefreshDebouncesConcurrent429(t *testing.T) {
	manager := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	manager.AddToken(&store.Token{
		ID: 1, Token: "test-token", Pool: PoolSuper, Status: string(StatusActive),
		Quotas: store.IntMap{"auto": 0},
	})

	var calls atomic.Int64
	server := rateLimitServer(t, RateLimitsResponse{
		RemainingQueries: 10,
		TotalQueries:     50,
	}, &calls)
	defer server.Close()

	scheduler := NewScheduler(manager, testModeSpecs(), server.URL)
	scheduler.RequestRefresh(1, "auto")
	scheduler.RequestRefresh(1, "auto")

	waitForCondition(t, func() bool {
		return manager.GetToken(1).Quotas["auto"] == 10
	})
	assert.Equal(t, int64(1), calls.Load())
}

func TestScheduler_RequestRefreshUsesLifecycleContext(t *testing.T) {
	manager := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	manager.AddToken(&store.Token{
		ID: 1, Token: "test-token", Pool: PoolSuper, Status: string(StatusActive),
		Quotas: store.IntMap{"auto": 0},
	})

	var calls atomic.Int64
	server := rateLimitServer(t, RateLimitsResponse{
		RemainingQueries: 10,
		TotalQueries:     50,
	}, &calls)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	scheduler := NewScheduler(manager, testModeSpecs(), server.URL)
	scheduler.Start(ctx)
	cancel()

	scheduler.RequestRefresh(1, "auto")
	scheduler.Stop()

	assert.Equal(t, int64(0), calls.Load())
	assert.Equal(t, 0, manager.GetToken(1).Quotas["auto"])
}

func TestScheduler_RefreshModeQuotaSetsResumeAt(t *testing.T) {
	tests := []struct {
		name        string
		resp        RateLimitsResponse
		wantSeconds int
	}{
		{
			name:        "wait time wins",
			resp:        RateLimitsResponse{RemainingQueries: 0, TotalQueries: 50, WaitTimeSeconds: 67, WindowSizeSeconds: 89},
			wantSeconds: 67,
		},
		{
			name:        "window time fallback",
			resp:        RateLimitsResponse{RemainingQueries: 0, TotalQueries: 50, WindowSizeSeconds: 89},
			wantSeconds: 89,
		},
		{
			name:        "mode window fallback",
			resp:        RateLimitsResponse{RemainingQueries: 0, TotalQueries: 50},
			wantSeconds: 123,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := newRefreshTestManager()
			server := rateLimitServer(t, tt.resp, nil)
			defer server.Close()
			mode := testRefreshMode(123)
			scheduler := NewScheduler(manager, []modelconfig.ModeSpec{mode}, server.URL)

			now := time.Now()
			err := scheduler.refreshModeQuota(context.Background(), refreshTarget(), mode, time.Now())
			require.NoError(t, err)

			token := manager.GetToken(1)
			assert.Equal(t, 0, token.Quotas["auto"])
			assert.Equal(t, 50, token.LimitQuotas["auto"])
			assert.InDelta(t, now.Add(time.Duration(tt.wantSeconds)*time.Second).Unix(), token.ResumeAts["auto"], 2)
		})
	}
}

func TestScheduler_RefreshModeQuotaClearsResumeAtWhenRemaining(t *testing.T) {
	manager := newRefreshTestManager()
	manager.SetResumeAt(1, "auto", int(time.Now().Add(time.Hour).Unix()))
	server := rateLimitServer(t, RateLimitsResponse{
		RemainingQueries:  12,
		TotalQueries:      50,
		WindowSizeSeconds: 7200,
	}, nil)
	defer server.Close()

	mode := testRefreshMode(7200)
	scheduler := NewScheduler(manager, []modelconfig.ModeSpec{mode}, server.URL)
	err := scheduler.refreshModeQuota(context.Background(), refreshTarget(), mode, time.Now())
	require.NoError(t, err)

	token := manager.GetToken(1)
	assert.Equal(t, 12, token.Quotas["auto"])
	assert.Equal(t, 50, token.LimitQuotas["auto"])
	assert.Empty(t, token.ResumeAts)
}

func TestScheduler_RefreshModeQuotaFailureKeepsResumeAt(t *testing.T) {
	manager := newRefreshTestManager()
	resumeAt := int(time.Now().Add(time.Hour).Unix())
	manager.SetResumeAt(1, "auto", resumeAt)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream failed", http.StatusTooManyRequests)
	}))
	defer server.Close()

	mode := testRefreshMode(7200)
	scheduler := NewScheduler(manager, []modelconfig.ModeSpec{mode}, server.URL)
	err := scheduler.refreshModeQuota(context.Background(), refreshTarget(), mode, time.Now())
	require.Error(t, err)

	token := manager.GetToken(1)
	assert.Equal(t, 0, token.Quotas["auto"])
	assert.Equal(t, resumeAt, token.ResumeAts["auto"])
}

func TestScheduler_RefreshModeQuotaFailureSetsBackoffWhenDue(t *testing.T) {
	manager := newRefreshTestManager()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream failed", http.StatusTooManyRequests)
	}))
	defer server.Close()

	mode := testRefreshMode(7200)
	scheduler := NewScheduler(manager, []modelconfig.ModeSpec{mode}, server.URL)
	now := time.Now()
	err := scheduler.refreshModeQuota(context.Background(), refreshTarget(), mode, time.Now())
	require.Error(t, err)

	token := manager.GetToken(1)
	assert.Equal(t, 0, token.Quotas["auto"])
	assert.InDelta(t, now.Add(30*time.Minute).Unix(), token.ResumeAts["auto"], 2)
}

func TestScheduler_RefreshModeQuotaProtocolErrorKeepsState(t *testing.T) {
	manager := newRefreshTestManager()
	resumeAt := int(time.Now().Add(time.Hour).Unix())
	manager.SetResumeAt(1, "auto", resumeAt)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	mode := testRefreshMode(7200)
	scheduler := NewScheduler(manager, []modelconfig.ModeSpec{mode}, server.URL)
	err := scheduler.refreshModeQuota(context.Background(), refreshTarget(), mode, time.Now())
	require.Error(t, err)

	token := manager.GetToken(1)
	assert.Equal(t, 0, token.Quotas["auto"])
	assert.Equal(t, 50, token.LimitQuotas["auto"])
	assert.Equal(t, resumeAt, token.ResumeAts["auto"])
}

func TestScheduler_RefreshTokenBypassesDebounce(t *testing.T) {
	manager := newRefreshTestManager()
	var calls atomic.Int64
	server := rateLimitServer(t, RateLimitsResponse{
		RemainingQueries: 7,
		TotalQueries:     50,
	}, &calls)
	defer server.Close()

	mode := testRefreshMode(7200)
	scheduler := NewScheduler(manager, []modelconfig.ModeSpec{mode}, server.URL)
	require.True(t, scheduler.checkAndMarkLastRefresh(1, "auto", time.Now()))

	err := scheduler.RefreshToken(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), calls.Load())
	assert.Equal(t, 7, manager.GetToken(1).Quotas["auto"])
}

func newRefreshTestManager() *TokenManager {
	manager := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	manager.AddToken(&store.Token{
		ID: 1, Token: "test-token", Pool: PoolSuper, Status: string(StatusActive),
		Quotas: store.IntMap{"auto": 0}, LimitQuotas: store.IntMap{"auto": 50},
	})
	return manager
}

func refreshTarget() ExhaustedModeTarget {
	return ExhaustedModeTarget{
		TokenID:   1,
		AuthToken: "test-token",
		Pool:      PoolSuper,
		Mode:      "auto",
	}
}

func testRefreshMode(windowSeconds int) modelconfig.ModeSpec {
	return modelconfig.ModeSpec{
		ID:            "auto",
		UpstreamName:  "auto",
		WindowSeconds: windowSeconds,
		DefaultQuota:  map[string]int{"super": 50},
	}
}

func rateLimitServer(
	t *testing.T,
	resp RateLimitsResponse,
	calls *atomic.Int64,
) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls != nil {
			calls.Add(1)
		}
		require.Equal(t, rateLimitsPath, r.URL.Path)
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
}

func waitForCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.True(t, condition())
}

func TestCalculateResumeAt_AllZeroFallsToModeWindow(t *testing.T) {
	now := time.Now()
	resp := &RateLimitsResponse{RemainingQueries: 0, TotalQueries: 30}
	mode := modelconfig.ModeSpec{WindowSeconds: 7200}
	resumeAt := calculateResumeAt(now, resp, mode)
	assert.InDelta(t, now.Add(7200*time.Second).Unix(), int64(resumeAt), 2)
}

func TestScheduler_ForgetTokenClearsLastRefresh(t *testing.T) {
	manager := newRefreshTestManager()
	server := rateLimitServer(t, RateLimitsResponse{
		RemainingQueries: 10,
		TotalQueries:     50,
	}, nil)
	defer server.Close()

	scheduler := NewScheduler(manager, testModeSpecs(), server.URL)

	// Mark debounce for token 1, mode "auto"
	require.True(t, scheduler.checkAndMarkLastRefresh(1, "auto", time.Now()))

	// Forget the token
	scheduler.ForgetToken(1)

	// Should allow refresh again (debounce cleared)
	require.True(t, scheduler.checkAndMarkLastRefresh(1, "auto", time.Now()))
}

// --- Local Quota refresh paths ---

func TestScheduler_RefreshLocalQuota_FirstExhaustionStartsWindow(t *testing.T) {
	manager := newRefreshTestManager()
	mode := modelconfig.ModeSpec{
		ID: "image_lite", UpstreamName: "fast", WindowSeconds: 86400,
		DefaultQuota: map[string]int{"super": 20}, LocalQuota: true,
	}
	scheduler := NewScheduler(manager, []modelconfig.ModeSpec{mode}, "")

	err := scheduler.refreshModeQuota(context.Background(), refreshTarget(), mode, time.Now())
	require.NoError(t, err)

	token := manager.GetToken(1)
	assert.Equal(t, 0, token.Quotas["image_lite"]) // quota NOT reset
	assert.Greater(t, token.ResumeAts["image_lite"], int(time.Now().Unix()))
}

func TestScheduler_RefreshLocalQuota_WindowNotExpiredSkips(t *testing.T) {
	manager := newRefreshTestManager()
	future := int(time.Now().Add(time.Hour).Unix())
	manager.SetResumeAt(1, "image_lite", future)

	mode := modelconfig.ModeSpec{
		ID: "image_lite", UpstreamName: "fast", WindowSeconds: 86400,
		DefaultQuota: map[string]int{"super": 20}, LocalQuota: true,
	}
	scheduler := NewScheduler(manager, []modelconfig.ModeSpec{mode}, "")

	err := scheduler.refreshModeQuota(context.Background(), refreshTarget(), mode, time.Now())
	require.NoError(t, err)

	token := manager.GetToken(1)
	assert.Equal(t, 0, token.Quotas["image_lite"])
	assert.Equal(t, future, token.ResumeAts["image_lite"])
}

func TestScheduler_RefreshLocalQuota_WindowExpiredResets(t *testing.T) {
	manager := newRefreshTestManager()
	past := int(time.Now().Add(-time.Hour).Unix())
	manager.SetResumeAt(1, "image_lite", past)

	mode := modelconfig.ModeSpec{
		ID: "image_lite", UpstreamName: "fast", WindowSeconds: 86400,
		DefaultQuota: map[string]int{"super": 20}, LocalQuota: true,
	}
	scheduler := NewScheduler(manager, []modelconfig.ModeSpec{mode}, "")

	err := scheduler.refreshModeQuota(context.Background(), refreshTarget(), mode, time.Now())
	require.NoError(t, err)

	token := manager.GetToken(1)
	assert.Equal(t, 20, token.Quotas["image_lite"])
	assert.Equal(t, 20, token.LimitQuotas["image_lite"])
	assert.Empty(t, token.ResumeAts["image_lite"])
}

func TestScheduler_RefreshLocalQuota_NonExhaustedDoesNothing(t *testing.T) {
	manager := newRefreshTestManager()
	manager.UpdateModeQuota(1, "image_lite", 3, 20)

	mode := modelconfig.ModeSpec{
		ID: "image_lite", UpstreamName: "fast", WindowSeconds: 86400,
		DefaultQuota: map[string]int{"super": 20}, LocalQuota: true,
	}
	scheduler := NewScheduler(manager, []modelconfig.ModeSpec{mode}, "")

	err := scheduler.refreshModeQuota(context.Background(), refreshTarget(), mode, time.Now())
	require.NoError(t, err)

	token := manager.GetToken(1)
	assert.Equal(t, 3, token.Quotas["image_lite"])
	assert.Empty(t, token.ResumeAts["image_lite"])
}

func TestScheduler_RefreshLocalQuota_ForceResetWithWindow(t *testing.T) {
	manager := newRefreshTestManager()
	future := int(time.Now().Add(time.Hour).Unix())
	manager.SetResumeAt(1, "image_lite", future)

	mode := modelconfig.ModeSpec{
		ID: "image_lite", UpstreamName: "fast", WindowSeconds: 86400,
		DefaultQuota: map[string]int{"super": 20}, LocalQuota: true,
	}
	scheduler := NewScheduler(manager, []modelconfig.ModeSpec{mode}, "")

	target := ExhaustedModeTarget{TokenID: 1, AuthToken: "test-token", Pool: PoolSuper, Mode: "image_lite", Force: true}
	err := scheduler.refreshModeQuota(context.Background(), target, mode, time.Now())
	require.NoError(t, err)

	token := manager.GetToken(1)
	assert.Equal(t, 20, token.Quotas["image_lite"])
	assert.Equal(t, 20, token.LimitQuotas["image_lite"])
	assert.Empty(t, token.ResumeAts["image_lite"])
}

func TestScheduler_RefreshLocalQuota_ForceResetWithoutWindow(t *testing.T) {
	manager := newRefreshTestManager()

	mode := modelconfig.ModeSpec{
		ID: "image_lite", UpstreamName: "fast", WindowSeconds: 86400,
		DefaultQuota: map[string]int{"super": 20}, LocalQuota: true,
	}
	scheduler := NewScheduler(manager, []modelconfig.ModeSpec{mode}, "")

	target := ExhaustedModeTarget{TokenID: 1, AuthToken: "test-token", Pool: PoolSuper, Mode: "image_lite", Force: true}
	err := scheduler.refreshModeQuota(context.Background(), target, mode, time.Now())
	require.NoError(t, err)

	token := manager.GetToken(1)
	assert.Equal(t, 20, token.Quotas["image_lite"])
	assert.Equal(t, 20, token.LimitQuotas["image_lite"])
	assert.Empty(t, token.ResumeAts["image_lite"])
}

func TestScheduler_RefreshLocalQuota_ForceSkipWhenNotExhausted(t *testing.T) {
	manager := newRefreshTestManager()
	manager.UpdateModeQuota(1, "image_lite", 5, 20)

	mode := modelconfig.ModeSpec{
		ID: "image_lite", UpstreamName: "fast", WindowSeconds: 86400,
		DefaultQuota: map[string]int{"super": 20}, LocalQuota: true,
	}
	scheduler := NewScheduler(manager, []modelconfig.ModeSpec{mode}, "")

	target := ExhaustedModeTarget{TokenID: 1, AuthToken: "test-token", Pool: PoolSuper, Mode: "image_lite", Force: true}
	err := scheduler.refreshModeQuota(context.Background(), target, mode, time.Now())
	require.NoError(t, err)

	token := manager.GetToken(1)
	assert.Equal(t, 5, token.Quotas["image_lite"])
}
