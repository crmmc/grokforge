package cfrefresh

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/logging"
	"github.com/crmmc/grokforge/internal/store"
)

const (
	minRefreshInterval = 60               // seconds
	defaultInterval    = 600              // seconds
	minTimeout         = 60               // seconds
	defaultTimeout     = 60               // seconds
	triggerCooldown    = 60 * time.Second // minimum gap between triggered refreshes
)

// Scheduler periodically refreshes CF clearance via FlareSolverr.
type Scheduler struct {
	runtime     *config.Runtime
	configStore *store.ConfigStore
	stopOnce    sync.Once
	stopped     chan struct{}
	done        chan struct{}
	triggerCh   chan struct{} // external trigger (e.g. on 403)
	lastRefresh atomic.Int64  // unix seconds of last successful refresh
}

// NewScheduler creates a CF refresh scheduler.
func NewScheduler(runtime *config.Runtime, configStore *store.ConfigStore) *Scheduler {
	return &Scheduler{
		runtime:     runtime,
		configStore: configStore,
		stopped:     make(chan struct{}),
		done:        make(chan struct{}),
		triggerCh:   make(chan struct{}, 1),
	}
}

// Start launches the background refresh goroutine.
func (s *Scheduler) Start() {
	safeGo("cfrefresh_scheduler", s.run)
}

// Stop signals the background goroutine to stop and waits for it to finish.
func (s *Scheduler) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopped)
	})
	<-s.done
}

// TriggerRefresh requests an immediate CF refresh (non-blocking, debounced).
// Safe to call from any goroutine. Multiple rapid calls are coalesced.
func (s *Scheduler) TriggerRefresh() {
	if !s.isEnabled() {
		return
	}
	// Debounce: skip if last refresh was within cooldown period.
	last := s.lastRefresh.Load()
	if last > 0 && time.Since(time.Unix(last, 0)) < triggerCooldown {
		logging.Debug("cf_refresh: trigger ignored (cooldown)")
		return
	}
	// Non-blocking send — if channel already has a pending trigger, skip.
	select {
	case s.triggerCh <- struct{}{}:
		logging.Info("cf_refresh: immediate refresh triggered (403 detected)")
	default:
		logging.Debug("cf_refresh: trigger already pending, skip")
	}
}

func (s *Scheduler) run() {
	defer close(s.done)

	// Immediate first refresh if enabled (matches Python behavior).
	if s.isEnabled() {
		logging.Info("cf_refresh: performing initial refresh",
			"flaresolverr_url", s.runtime.Get().Proxy.FlareSolverrURL)
		s.refreshOnce()
	} else {
		logging.Info("cf_refresh: disabled or FlareSolverr URL not set, skipping")
	}

	for {
		interval := s.getInterval()
		timer := time.NewTimer(time.Duration(interval) * time.Second)

		select {
		case <-s.stopped:
			timer.Stop()
			logging.Info("cf_refresh: scheduler stopped")
			return
		case <-s.triggerCh:
			timer.Stop()
			if s.isEnabled() {
				logging.Info("cf_refresh: executing triggered refresh")
				s.refreshOnce()
			}
		case <-timer.C:
			if s.isEnabled() {
				s.refreshOnce()
			} else {
				logging.Debug("cf_refresh: disabled, skip refresh")
			}
		}
	}
}

func (s *Scheduler) refreshOnce() {
	cfg := s.runtime.Get()
	flareURL := cfg.Proxy.FlareSolverrURL
	timeout := s.getTimeout()
	proxyURL := cfg.Proxy.BaseProxyURL

	logging.Info("cf_refresh: refreshing cf_clearance...",
		"flaresolverr_url", flareURL, "timeout", timeout)

	result, err := SolveCFChallenge(flareURL, timeout, proxyURL)
	if err != nil {
		logging.Error("cf_refresh: refresh failed", "error", err)
		return
	}

	// Update runtime config.
	_ = s.runtime.Update(func(cfg *config.Config) error {
		cfg.Proxy.CFCookies = result.Cookies
		cfg.Proxy.CFClearance = result.CFClearance
		if result.UserAgent != "" {
			cfg.Proxy.UserAgent = result.UserAgent
		}
		if result.Browser != "" {
			cfg.Proxy.Browser = result.Browser
		}
		return nil
	})

	// Record last successful refresh time.
	s.lastRefresh.Store(time.Now().Unix())

	// Persist to DB so values survive restart.
	kvs := map[string]string{
		"proxy.cf_cookies":   result.Cookies,
		"proxy.cf_clearance": result.CFClearance,
	}
	if result.UserAgent != "" {
		kvs["proxy.user_agent"] = result.UserAgent
	}
	if result.Browser != "" {
		kvs["proxy.browser"] = result.Browser
	}
	if err := s.configStore.SetMany(kvs); err != nil {
		logging.Error("cf_refresh: failed to persist config", "error", err)
	}

	logging.Info("cf_refresh: refresh complete",
		"cookies_len", len(result.Cookies),
		"browser", result.Browser)
}

func (s *Scheduler) isEnabled() bool {
	cfg := s.runtime.Get()
	return cfg != nil && cfg.Proxy.Enabled && cfg.Proxy.FlareSolverrURL != ""
}

func (s *Scheduler) getInterval() int {
	v := s.runtime.Get().Proxy.RefreshInterval
	if v < minRefreshInterval {
		return defaultInterval
	}
	return v
}

func (s *Scheduler) getTimeout() int {
	v := s.runtime.Get().Proxy.Timeout
	if v < minTimeout {
		return defaultTimeout
	}
	return v
}
