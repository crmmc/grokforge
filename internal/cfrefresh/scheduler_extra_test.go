package cfrefresh

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time proof that the production store satisfies the seam interface.
var _ configWriter = (*store.ConfigStore)(nil)

type solveCall struct {
	url     string
	timeout int
	proxy   string
}

// solveStub is a hand-written fake for the FlareSolverr call. It records
// arguments and optionally signals every invocation on setC (buffered) so
// tests can synchronize with the scheduler goroutine without sleeping.
type solveStub struct {
	mu     sync.Mutex
	calls  []solveCall
	result *SolveResult
	err    error
	setC   chan struct{}
}

func (s *solveStub) solve(url string, timeout int, proxy string) (*SolveResult, error) {
	s.mu.Lock()
	s.calls = append(s.calls, solveCall{url: url, timeout: timeout, proxy: proxy})
	s.mu.Unlock()
	if s.setC != nil {
		s.setC <- struct{}{}
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.result, nil
}

func (s *solveStub) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func (s *solveStub) lastCall() solveCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.calls) == 0 {
		return solveCall{}
	}
	return s.calls[len(s.calls)-1]
}

// fakeConfigStore implements configWriter, recording SetMany payloads.
type fakeConfigStore struct {
	mu    sync.Mutex
	calls []map[string]string
	err   error
	setC  chan struct{}
}

func (f *fakeConfigStore) SetMany(kvs map[string]string) error {
	f.mu.Lock()
	f.calls = append(f.calls, kvs)
	f.mu.Unlock()
	if f.setC != nil {
		f.setC <- struct{}{}
	}
	return f.err
}

func (f *fakeConfigStore) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeConfigStore) lastCall() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return nil
	}
	return f.calls[len(f.calls)-1]
}

func enabledConfig() *config.Config {
	cfg := config.DefaultConfig()
	cfg.Proxy.Enabled = true
	cfg.Proxy.FlareSolverrURL = "http://fs:8191"
	return cfg
}

func newTestScheduler(cfg *config.Config, cw configWriter) *Scheduler {
	return &Scheduler{
		runtime:     config.NewRuntime(cfg),
		configStore: cw,
		stopped:     make(chan struct{}),
		done:        make(chan struct{}),
		triggerCh:   make(chan struct{}, 1),
	}
}

func TestNewScheduler_InitializesChannels(t *testing.T) {
	r := config.NewRuntime(config.DefaultConfig())
	s := NewScheduler(r, nil)

	require.NotNil(t, s.stopped)
	require.NotNil(t, s.done)
	require.NotNil(t, s.triggerCh)
	assert.Equal(t, 1, cap(s.triggerCh))
	assert.Same(t, r, s.runtime)
}

func TestScheduler_SolverFallback(t *testing.T) {
	s := NewScheduler(config.NewRuntime(config.DefaultConfig()), nil)
	assert.Equal(t,
		reflect.ValueOf(SolveCFChallenge).Pointer(),
		reflect.ValueOf(s.solver()).Pointer(),
		"unset solveFn must fall back to the real solver",
	)

	stub := &solveStub{}
	s.solveFn = stub.solve
	assert.Equal(t,
		reflect.ValueOf(stub.solve).Pointer(),
		reflect.ValueOf(s.solver()).Pointer(),
	)
}

func TestScheduler_RefreshDelayAndSinceDefaults(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Proxy.RefreshInterval = 120
	s := NewScheduler(config.NewRuntime(cfg), nil)

	assert.Equal(t, 120*time.Second, s.refreshDelay(), "default path derives from configured interval")
	assert.Greater(t, s.since(time.Now().Add(-time.Hour)), time.Minute, "default path uses real clock")
}

func TestScheduler_RunDisabled_SkipsAllRefreshes(t *testing.T) {
	stub := &solveStub{}
	s := newTestScheduler(config.DefaultConfig(), &fakeConfigStore{}) // proxy disabled
	s.solveFn = stub.solve

	delayC := make(chan time.Duration)
	s.nextDelay = func() time.Duration { return <-delayC }

	s.Start()
	// iter1: hour timer never fires; a pending trigger is consumed with
	// proxy disabled (skip branch).
	delayC <- time.Hour
	s.triggerCh <- struct{}{}
	// iter2: 1ms timer fires and hits the disabled skip branch; iter3's
	// delay() call proves iter2's timer branch completed.
	delayC <- 1 * time.Millisecond
	// iter3: hour timer never fires; Stop closes the stopped channel.
	delayC <- time.Hour
	s.Stop()

	assert.Equal(t, 0, stub.callCount(), "disabled scheduler must never solve")
}

func TestScheduler_RunEnabled_InitialAndTriggeredRefreshes(t *testing.T) {
	cfg := enabledConfig()
	cfg.Proxy.Timeout = 90
	cfg.Proxy.BaseProxyURL = "http://proxy:1"

	stub := &solveStub{
		result: &SolveResult{
			Cookies:     "cf=1",
			CFClearance: "cl-1",
			UserAgent:   "UA-X",
			Browser:     "chrome136",
		},
		setC: make(chan struct{}, 8),
	}
	fake := &fakeConfigStore{setC: make(chan struct{}, 8)}
	s := newTestScheduler(cfg, fake)
	s.solveFn = stub.solve

	delayC := make(chan time.Duration)
	s.nextDelay = func() time.Duration { return <-delayC }

	s.Start()
	<-fake.setC // initial refresh completed through persist
	delayC <- time.Hour
	s.triggerCh <- struct{}{} // trigger branch with enabled proxy
	<-fake.setC               // triggered refresh completed through persist
	delayC <- time.Hour
	s.Stop()

	require.Equal(t, 2, stub.callCount())
	call := stub.lastCall()
	assert.Equal(t, "http://fs:8191", call.url)
	assert.Equal(t, 90, call.timeout)
	assert.Equal(t, "http://proxy:1", call.proxy)

	snap := s.runtime.Get()
	require.NotNil(t, snap)
	assert.Equal(t, "cf=1", snap.Proxy.CFCookies)
	assert.Equal(t, "cl-1", snap.Proxy.CFClearance)
	assert.Equal(t, "UA-X", snap.Proxy.UserAgent)
	assert.Equal(t, "chrome136", snap.Proxy.Browser)

	require.Equal(t, 2, fake.callCount())
	persisted := fake.lastCall()
	assert.Len(t, persisted, 4)
	assert.Equal(t, "cf=1", persisted["proxy.cf_cookies"])
	assert.Equal(t, "cl-1", persisted["proxy.cf_clearance"])
	assert.Equal(t, "UA-X", persisted["proxy.user_agent"])
	assert.Equal(t, "chrome136", persisted["proxy.browser"])

	assert.Positive(t, s.lastRefresh.Load())
}

func TestScheduler_RunEnabled_TimerDrivenRefresh(t *testing.T) {
	stub := &solveStub{
		result: &SolveResult{Cookies: "cf=2", CFClearance: "cl-2"},
		setC:   make(chan struct{}, 8),
	}
	fake := &fakeConfigStore{setC: make(chan struct{}, 8)}
	s := newTestScheduler(enabledConfig(), fake)
	s.solveFn = stub.solve

	delayC := make(chan time.Duration)
	s.nextDelay = func() time.Duration { return <-delayC }

	s.Start()
	<-fake.setC                    // initial refresh
	delayC <- 1 * time.Millisecond // iter1: short timer fires, refresh runs
	<-fake.setC                    // timer-driven refresh completed
	delayC <- time.Hour            // iter2 armed, never fires
	s.Stop()

	assert.Equal(t, 2, stub.callCount())
}

func TestScheduler_RefreshOnce_SolveErrorLeavesStateUntouched(t *testing.T) {
	stub := &solveStub{err: errors.New("flaresolverr down")}
	fake := &fakeConfigStore{}
	cfg := enabledConfig()
	cfg.Proxy.CFCookies = "original"
	s := newTestScheduler(cfg, fake)
	s.solveFn = stub.solve

	s.refreshOnce()

	assert.Equal(t, 1, stub.callCount())
	assert.Equal(t, 0, fake.callCount(), "failed solve must not persist")
	assert.Equal(t, int64(0), s.lastRefresh.Load())
	assert.Equal(t, "original", s.runtime.Get().Proxy.CFCookies)
}

func TestScheduler_RefreshOnce_PersistErrorIsLoggedNotFatal(t *testing.T) {
	stub := &solveStub{result: &SolveResult{Cookies: "cf=3", CFClearance: "cl-3"}}
	fake := &fakeConfigStore{err: errors.New("db down")}
	s := newTestScheduler(enabledConfig(), fake)
	s.solveFn = stub.solve

	s.refreshOnce()

	assert.Equal(t, 1, fake.callCount())
	snap := s.runtime.Get()
	assert.Equal(t, "cf=3", snap.Proxy.CFCookies, "runtime must be updated even if persistence fails")
	assert.Equal(t, "cl-3", snap.Proxy.CFClearance)
	assert.Positive(t, s.lastRefresh.Load())
}

func TestScheduler_RefreshOnce_EmptyUserAgentAndBrowserKeepDefaults(t *testing.T) {
	stub := &solveStub{result: &SolveResult{Cookies: "cf=4", CFClearance: "cl-4"}}
	fake := &fakeConfigStore{}
	cfg := enabledConfig()
	cfg.Proxy.UserAgent = "default-ua"
	cfg.Proxy.Browser = "default-browser"
	s := newTestScheduler(cfg, fake)
	s.solveFn = stub.solve

	s.refreshOnce()

	persisted := fake.lastCall()
	require.NotNil(t, persisted)
	assert.Len(t, persisted, 2)
	assert.Equal(t, "cf=4", persisted["proxy.cf_cookies"])
	assert.Equal(t, "cl-4", persisted["proxy.cf_clearance"])
	assert.NotContains(t, persisted, "proxy.user_agent")
	assert.NotContains(t, persisted, "proxy.browser")

	snap := s.runtime.Get()
	assert.Equal(t, "default-ua", snap.Proxy.UserAgent)
	assert.Equal(t, "default-browser", snap.Proxy.Browser)
}

func TestScheduler_TriggerRefresh(t *testing.T) {
	newEnabled := func() *Scheduler {
		s := newTestScheduler(enabledConfig(), &fakeConfigStore{})
		s.nowFn = func() time.Time { return time.Unix(1_000_000, 0) }
		return s
	}

	t.Run("disabled proxy returns early", func(t *testing.T) {
		s := newTestScheduler(config.DefaultConfig(), &fakeConfigStore{})
		s.TriggerRefresh()
		assert.Len(t, s.triggerCh, 0)
	})

	t.Run("first trigger is enqueued", func(t *testing.T) {
		s := newEnabled()
		s.TriggerRefresh()
		assert.Len(t, s.triggerCh, 1)
	})

	t.Run("pending trigger is not duplicated", func(t *testing.T) {
		s := newEnabled()
		s.TriggerRefresh()
		s.TriggerRefresh()
		assert.Len(t, s.triggerCh, 1)
	})

	t.Run("cooldown suppresses trigger", func(t *testing.T) {
		s := newEnabled()
		now := time.Unix(1_000_000, 0)
		s.lastRefresh.Store(now.Add(-30 * time.Second).Unix())
		s.TriggerRefresh()
		assert.Len(t, s.triggerCh, 0)
	})

	t.Run("beyond cooldown enqueues trigger", func(t *testing.T) {
		s := newEnabled()
		now := time.Unix(1_000_000, 0)
		s.lastRefresh.Store(now.Add(-120 * time.Second).Unix())
		s.TriggerRefresh()
		assert.Len(t, s.triggerCh, 1)
	})
}

func TestScheduler_GetIntervalAndGetTimeout(t *testing.T) {
	tests := []struct {
		name            string
		interval        int
		timeout         int
		wantIntervalSec int
		wantTimeoutSec  int
	}{
		{"zeros fall back to defaults", 0, 0, defaultInterval, defaultTimeout},
		{"below minimum falls back", 30, 59, defaultInterval, defaultTimeout},
		{"at minimum is kept", 60, 60, 60, 60},
		{"valid values pass through", 1200, 240, 1200, 240},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Proxy.RefreshInterval = tc.interval
			cfg.Proxy.Timeout = tc.timeout
			s := newTestScheduler(cfg, &fakeConfigStore{})

			assert.Equal(t, tc.wantIntervalSec, s.getInterval())
			assert.Equal(t, tc.wantTimeoutSec, s.getTimeout())
		})
	}
}

func TestScheduler_StopIsIdempotent(t *testing.T) {
	s := newTestScheduler(config.DefaultConfig(), &fakeConfigStore{})
	delayC := make(chan time.Duration)
	s.nextDelay = func() time.Duration { return <-delayC }

	s.Start()
	delayC <- time.Hour
	s.Stop()
	s.Stop() // stopOnce guards double close; done is already closed
}

func TestSafeGo_RunsFunction(t *testing.T) {
	done := make(chan struct{})
	safeGo("test-ok", func() { close(done) })
	<-done
}

func TestSafeGo_RecoversPanic(t *testing.T) {
	done := make(chan struct{})
	safeGo("test-panic", func() {
		defer close(done)
		panic("boom")
	})
	<-done
	// The recover in safeGo's deferred handler runs right after fn's defers;
	// it only touches the concurrency-safe default logger, so nothing else
	// to synchronize on here.
}
