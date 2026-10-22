package token

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/modelconfig"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduler_Run_ExitsViaStop(t *testing.T) {
	manager := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	scheduler := NewScheduler(manager, nil, "https://example.invalid")

	scheduler.Start(context.Background())
	scheduler.Stop() // scan loop must exit through the stopped channel
}

func TestScheduler_Run_ExitsViaContextCancel(t *testing.T) {
	manager := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	scheduler := NewScheduler(manager, nil, "https://example.invalid")

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)
	cancel()
	// No Stop here: with the context the only ready case, the loop must exit
	// through ctx.Done on its own.
}

func TestScheduler_Run_TickerDrivesScan(t *testing.T) {
	manager := newRefreshTestManager()
	var calls atomic.Int64
	server := rateLimitServer(t, RateLimitsResponse{RemainingQueries: 10, TotalQueries: 50}, &calls)
	defer server.Close()

	// Single supported mode so exactly one refresh target exists per scan.
	scheduler := NewScheduler(manager, []modelconfig.ModeSpec{testRefreshMode(7200)}, server.URL)
	scheduler.scanInterval = 5 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)

	waitUntil(t, 2*time.Second, func() bool {
		return manager.GetModeQuota(1, "auto") == 10
	}, "periodic scan must refresh the exhausted mode")

	cancel()
	scheduler.Stop()
	assert.Equal(t, int64(1), calls.Load(), "debounce must prevent repeated refreshes")
}

func TestScheduler_Scan_DebouncesSecondScanWhileRefreshInFlight(t *testing.T) {
	manager := newRefreshTestManager()
	release := make(chan struct{})
	var releaseOnce sync.Once

	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		<-release
		_ = json.NewEncoder(w).Encode(RateLimitsResponse{RemainingQueries: 10, TotalQueries: 50})
	}))
	defer server.Close() // registered first, runs last
	// Defers run LIFO: the release below runs before server.Close, unblocking
	// any handler still waiting, so cleanup cannot deadlock on a failed assert.
	defer releaseOnce.Do(func() { close(release) })

	// Single supported mode so exactly one refresh target exists per scan.
	scheduler := NewScheduler(manager, []modelconfig.ModeSpec{testRefreshMode(7200)}, server.URL)
	scheduler.scan(context.Background()) // starts an async refresh that blocks in the handler

	waitUntil(t, 2*time.Second, func() bool { return calls.Load() == 1 }, "first refresh must reach upstream")

	scheduler.scan(context.Background()) // token still exhausted, but debounced while refresh is pending
	assert.Equal(t, int64(1), calls.Load(), "second scan must be debounced")

	releaseOnce.Do(func() { close(release) })
	waitUntil(t, 2*time.Second, func() bool {
		return manager.GetModeQuota(1, "auto") == 10
	}, "released refresh must apply the new quota")
	assert.Equal(t, int64(1), calls.Load())
}

func TestScheduler_RequestRefresh_GuardsSkipRefresh(t *testing.T) {
	activeSuper := &store.Token{ID: 1, Token: "t1", Pool: PoolSuper, Status: string(StatusActive), Quotas: store.IntMap{"auto": 0}}
	cases := []struct {
		name    string
		token   *store.Token
		tokenID uint
		mode    string
	}{
		{name: "empty mode", token: activeSuper, tokenID: 1, mode: ""},
		{name: "unknown token", token: activeSuper, tokenID: 42, mode: "auto"},
		{name: "inactive token", token: &store.Token{ID: 1, Token: "t1", Pool: PoolSuper, Status: string(StatusDisabled)}, tokenID: 1, mode: "auto"},
		{name: "unsupported mode", token: activeSuper, tokenID: 1, mode: "nope"},
		{name: "mode unsupported by pool", token: &store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive)}, tokenID: 1, mode: "auto"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manager := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
			manager.AddToken(tc.token)

			var calls atomic.Int64
			server := rateLimitServer(t, RateLimitsResponse{RemainingQueries: 10, TotalQueries: 50}, &calls)
			defer server.Close()

			scheduler := NewScheduler(manager, testModeSpecs(), server.URL)
			scheduler.RequestRefresh(tc.tokenID, tc.mode)

			assert.Equal(t, int64(0), calls.Load(), "guard must prevent any upstream refresh")
		})
	}
}

func TestScheduler_RequestRefresh_SkipsRefreshWhenContextCanceled(t *testing.T) {
	manager := newRefreshTestManager()
	scheduler := NewScheduler(manager, testModeSpecs(), "https://example.invalid")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	scheduler.Start(ctx)
	fillGlobalSem(scheduler)

	scheduler.RequestRefresh(1, "auto")
	scheduler.Stop()

	assert.Equal(t, 0, manager.GetModeQuota(1, "auto"), "canceled lifecycle must suppress the async refresh")
}

func TestScheduler_RefreshToken_UnknownTokenReturnsErr(t *testing.T) {
	manager := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	scheduler := NewScheduler(manager, testModeSpecs(), "")

	err := scheduler.RefreshToken(context.Background(), 999)
	require.ErrorIs(t, err, ErrTokenNotFound)
}

func fillGlobalSem(s *Scheduler) {
	for i := 0; i < maxConcurrentGlobal; i++ {
		s.globalSem <- struct{}{}
	}
}

func TestScheduler_RefreshToken_AcquireFailsOnCanceledContext(t *testing.T) {
	manager := newRefreshTestManager()
	scheduler := NewScheduler(manager, testModeSpecs(), "https://example.invalid")
	fillGlobalSem(scheduler) // full semaphore makes the select deterministic

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := scheduler.RefreshToken(ctx, 1)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 0, manager.GetModeQuota(1, "auto"))
}

func TestScheduler_RefreshToken_AcquireFailsWhenStopped(t *testing.T) {
	manager := newRefreshTestManager()
	scheduler := NewScheduler(manager, testModeSpecs(), "https://example.invalid")
	fillGlobalSem(scheduler)
	scheduler.Stop()

	err := scheduler.RefreshToken(context.Background(), 1)
	require.ErrorIs(t, err, errSchedulerStopped)
}

func TestScheduler_RefreshToken_UpstreamFailureJoinsErrors(t *testing.T) {
	manager := newRefreshTestManager()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream failed", http.StatusTooManyRequests)
	}))
	defer server.Close()

	scheduler := NewScheduler(manager, []modelconfig.ModeSpec{testRefreshMode(7200)}, server.URL)
	err := scheduler.RefreshToken(context.Background(), 1)
	require.Error(t, err)
	assert.Equal(t, 0, manager.GetModeQuota(1, "auto"))
	assert.Greater(t, manager.GetResumeAt(1, "auto"), 0, "failure must schedule a retry backoff")
}

func TestScheduler_RefreshToken_SkipsModesWithoutPoolQuota(t *testing.T) {
	manager := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	manager.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 0}})

	var calls atomic.Int64
	server := rateLimitServer(t, RateLimitsResponse{RemainingQueries: 5, TotalQueries: 10}, &calls)
	defer server.Close()

	scheduler := NewScheduler(manager, testModeSpecs(), server.URL)
	err := scheduler.RefreshToken(context.Background(), 1)
	require.NoError(t, err)

	assert.Equal(t, int64(1), calls.Load(), "only the supported mode must be refreshed")
	assert.Equal(t, 5, manager.GetModeQuota(1, "fast"))
	assert.Equal(t, 0, manager.GetModeQuota(1, "auto"))
}
