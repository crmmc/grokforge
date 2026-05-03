package token

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/crmmc/grokforge/internal/modelconfig"
)

const (
	defaultScanInterval = time.Minute
	maxConcurrentGlobal = 20
	refreshDebounce     = time.Minute
)

var errSchedulerStopped = errors.New("scheduler stopped")

// Scheduler periodically refreshes exhausted token mode quotas from upstream.
type Scheduler struct {
	manager  *TokenManager
	modes    []modelconfig.ModeSpec
	modeByID map[string]modelconfig.ModeSpec
	baseURL  string

	runCtx      context.Context
	lastRefresh map[uint]map[string]time.Time

	mu        sync.Mutex
	globalSem chan struct{}
	wg        sync.WaitGroup
	stopOnce  sync.Once
	stopped   chan struct{}
}

// NewScheduler creates a new mode-based quota refresh scheduler.
func NewScheduler(manager *TokenManager, modes []modelconfig.ModeSpec, baseURL string) *Scheduler {
	mByID := make(map[string]modelconfig.ModeSpec, len(modes))
	for _, m := range modes {
		mByID[m.ID] = m
	}
	return &Scheduler{
		manager:     manager,
		modes:       append([]modelconfig.ModeSpec(nil), modes...),
		modeByID:    mByID,
		baseURL:     baseURL,
		lastRefresh: make(map[uint]map[string]time.Time),
		globalSem:   make(chan struct{}, maxConcurrentGlobal),
		stopped:     make(chan struct{}),
	}
}

// Start begins the periodic scan loop.
func (s *Scheduler) Start(ctx context.Context) {
	s.setRunContext(ctx)
	s.wg.Add(1)
	safeGo("quota_refresh_scheduler", func() {
		s.run(ctx)
	})
}

// Stop waits for the scan loop and active refresh operations to complete.
func (s *Scheduler) Stop() {
	s.stopOnce.Do(func() {
		close(s.stopped)
	})
	s.wg.Wait()
}

func (s *Scheduler) run(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(defaultScanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopped:
			return
		case <-ticker.C:
			s.scan(ctx)
		}
	}
}

func (s *Scheduler) scan(ctx context.Context) {
	now := time.Now()
	started := 0

	for _, target := range s.manager.ScanExhaustedModes(s.modes, now) {
		mode, ok := s.modeByID[target.Mode]
		if !ok || !s.checkAndMarkLastRefresh(target.TokenID, target.Mode, now) {
			continue
		}
		s.runRefreshAsync(ctx, target, mode)
		started++
	}
	if started > 0 {
		slog.Debug("refresh scheduler: scanning", "tasks", started)
	}
}

// RequestRefresh schedules an authoritative refresh after a mode-level 429.
func (s *Scheduler) RequestRefresh(tokenID uint, mode string) {
	if mode == "" {
		return
	}
	authToken, pool, ok := s.manager.GetActiveTokenInfo(tokenID)
	if !ok {
		slog.Debug("refresh: token not active", "token_id", tokenID, "mode", mode)
		return
	}
	modeSpec, ok := s.findMode(mode)
	if !ok {
		slog.Debug("refresh: unsupported mode", "token_id", tokenID, "mode", mode)
		return
	}
	if modeSpec.DefaultQuota[PoolToShort(pool)] <= 0 {
		slog.Debug("refresh: mode unsupported by pool", "token_id", tokenID, "mode", mode, "pool", pool)
		return
	}
	now := time.Now()
	if !s.checkAndMarkLastRefresh(tokenID, mode, now) {
		return
	}
	target := ExhaustedModeTarget{TokenID: tokenID, AuthToken: authToken, Pool: pool, Mode: mode}
	s.runRefreshAsync(s.refreshContext(), target, modeSpec)
}

// RefreshToken forces an immediate refresh for all supported modes of a token.
func (s *Scheduler) RefreshToken(ctx context.Context, tokenID uint) error {
	token := s.manager.GetToken(tokenID)
	if token == nil {
		return ErrTokenNotFound
	}

	var refreshErrs []error
	for _, mode := range s.supportedModesForPool(token.Pool) {
		if err := s.acquireGlobal(ctx); err != nil {
			refreshErrs = append(refreshErrs, err)
			break
		}
		func() {
			defer func() { <-s.globalSem }()
			target := ExhaustedModeTarget{TokenID: tokenID, AuthToken: token.Token, Pool: token.Pool, Mode: mode.ID}
			if err := s.refreshModeQuota(ctx, target, mode); err != nil {
				refreshErrs = append(refreshErrs, err)
			}
		}()
	}
	if len(refreshErrs) == 0 {
		return nil
	}
	return errors.Join(refreshErrs...)
}

func (s *Scheduler) runRefreshAsync(
	ctx context.Context,
	target ExhaustedModeTarget,
	mode modelconfig.ModeSpec,
) {
	s.wg.Add(1)
	safeGo("refresh_mode", func() {
		defer s.wg.Done()
		if err := s.acquireGlobal(ctx); err != nil {
			return
		}
		defer func() { <-s.globalSem }()
		_ = s.refreshModeQuota(ctx, target, mode)
	})
}

func (s *Scheduler) refreshModeQuota(
	ctx context.Context,
	target ExhaustedModeTarget,
	mode modelconfig.ModeSpec,
) error {
	resp, err := s.manager.SyncModeQuota(ctx, target.TokenID, target.AuthToken, s.baseURL, mode.UpstreamName)
	if err != nil {
		s.logRefreshFailure(target, mode, err)
		s.applyRefreshFailureBackoff(target, mode, time.Now())
		return fmt.Errorf("refresh mode %s: %w", mode.ID, err)
	}
	if resp == nil {
		err := errors.New("rate-limits response is nil")
		s.logRefreshFailure(target, mode, err)
		s.applyRefreshFailureBackoff(target, mode, time.Now())
		return fmt.Errorf("refresh mode %s: %w", mode.ID, err)
	}

	remaining, limit, resumeAt := s.applyRateLimits(target.TokenID, mode, resp, time.Now())
	slog.Debug("refresh: mode quota updated",
		"token_id", target.TokenID,
		"pool", target.Pool,
		"mode", mode.ID,
		"upstream_name", mode.UpstreamName,
		"action", "refresh_success",
		"remaining", remaining,
		"limit", limit,
		"resume_at", resumeAt)
	return nil
}

func (s *Scheduler) acquireGlobal(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.stopped:
		return errSchedulerStopped
	case s.globalSem <- struct{}{}:
		return nil
	}
}

func (s *Scheduler) checkAndMarkLastRefresh(tokenID uint, mode string, now time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	modeMap := s.lastRefresh[tokenID]
	if modeMap == nil {
		modeMap = make(map[string]time.Time)
		s.lastRefresh[tokenID] = modeMap
	}
	if last, ok := modeMap[mode]; ok && now.Sub(last) < refreshDebounce {
		return false
	}
	modeMap[mode] = now
	return true
}

func (s *Scheduler) setRunContext(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runCtx = ctx
}

func (s *Scheduler) refreshContext() context.Context {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runCtx == nil {
		return context.Background()
	}
	return s.runCtx
}

func (s *Scheduler) findMode(id string) (modelconfig.ModeSpec, bool) {
	m, ok := s.modeByID[id]
	return m, ok
}

func (s *Scheduler) supportedModesForPool(pool string) []modelconfig.ModeSpec {
	poolKey := PoolToShort(pool)
	modes := make([]modelconfig.ModeSpec, 0, len(s.modes))
	for _, mode := range s.modes {
		if mode.DefaultQuota[poolKey] <= 0 {
			continue
		}
		modes = append(modes, mode)
	}
	return modes
}

// ForgetToken removes scheduler-internal debounce state for a removed token.
func (s *Scheduler) ForgetToken(tokenID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.lastRefresh, tokenID)
}
