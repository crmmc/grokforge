package token

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/crmmc/grokforge/internal/modelconfig"
)

const (
	defaultScanInterval   = 1 * time.Minute
	maxConcurrentPerToken = 5
	maxConcurrentGlobal   = 20
)

// Scheduler periodically scans tokens and refreshes mode quotas
// based on first_used_at + observed_window timing.
type Scheduler struct {
	manager *TokenManager
	modes   []modelconfig.ModeSpec
	baseURL string

	// firstUsedAt[token_id][mode_id] = timestamp of first use after last refresh
	firstUsedAt map[uint]map[string]time.Time
	// observedWindow[mode_id] = upstream-reported window in seconds
	observedWindow map[string]int

	mu        sync.Mutex // protects firstUsedAt and observedWindow
	globalSem chan struct{}
	wg        sync.WaitGroup
	stopOnce  sync.Once
	stopped   chan struct{}
}

// NewScheduler creates a new mode-based quota refresh scheduler.
func NewScheduler(manager *TokenManager, modes []modelconfig.ModeSpec, baseURL string) *Scheduler {
	return &Scheduler{
		manager:        manager,
		modes:          modes,
		baseURL:        baseURL,
		firstUsedAt:    make(map[uint]map[string]time.Time),
		observedWindow: make(map[string]int),
		globalSem:      make(chan struct{}, maxConcurrentGlobal),
		stopped:        make(chan struct{}),
	}
}

// RecordFirstUsed records the first use timestamp for a token+mode.
// Only sets if not already present (first use after last refresh).
func (s *Scheduler) RecordFirstUsed(tokenID uint, mode string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.firstUsedAt[tokenID]; !ok {
		s.firstUsedAt[tokenID] = make(map[string]time.Time)
	}
	if _, exists := s.firstUsedAt[tokenID][mode]; !exists {
		s.firstUsedAt[tokenID][mode] = time.Now()
	}
}

// SetFirstUsedAt explicitly sets first_used_at for a token+mode.
// Used by startup normalization for exhausted modes.
func (s *Scheduler) SetFirstUsedAt(tokenID uint, mode string, t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.firstUsedAt[tokenID]; !ok {
		s.firstUsedAt[tokenID] = make(map[string]time.Time)
	}
	s.firstUsedAt[tokenID][mode] = t
}

// Start begins the periodic scan loop.
func (s *Scheduler) Start(ctx context.Context) {
	s.wg.Add(1)
	safeGo("quota_refresh_scheduler", func() {
		s.run(ctx)
	})
}

// Stop waits for all refresh operations to complete.
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
	s.mu.Lock()
	// Build a snapshot of what needs refreshing
	type refreshTask struct {
		tokenID   uint
		authToken string
		mode      modelconfig.ModeSpec
	}
	var tasks []refreshTask
	now := time.Now()

	// Build mode lookup
	modeByID := make(map[string]modelconfig.ModeSpec, len(s.modes))
	for _, m := range s.modes {
		modeByID[m.ID] = m
	}

	for tokenID, modeMap := range s.firstUsedAt {
		token := s.manager.GetToken(tokenID)
		if token == nil || Status(token.Status) != StatusActive {
			continue
		}
		poolShort := poolToShort(token.Pool)

		for modeID, firstUsed := range modeMap {
			mode, ok := modeByID[modeID]
			if !ok {
				continue
			}
			// Skip modes not supported by this pool
			if mode.DefaultQuota[poolShort] <= 0 {
				continue
			}
			// Calculate deadline
			windowSec := mode.WindowSeconds
			if observed, ok := s.observedWindow[modeID]; ok {
				windowSec = observed
			}
			deadline := firstUsed.Add(time.Duration(windowSec) * time.Second)
			if now.Before(deadline) {
				continue
			}
			tasks = append(tasks, refreshTask{
				tokenID:   tokenID,
				authToken: token.Token,
				mode:      mode,
			})
		}
	}
	s.mu.Unlock()

	if len(tasks) == 0 {
		return
	}

	slog.Debug("refresh scheduler: scanning", "tasks", len(tasks))

	// Execute tasks with concurrency control
	var wg sync.WaitGroup
	// Group by token for per-token concurrency limit
	byToken := make(map[uint][]refreshTask)
	for _, t := range tasks {
		byToken[t.tokenID] = append(byToken[t.tokenID], t)
	}

	for tokenID, tokenTasks := range byToken {
		tokenID := tokenID
		tokenTasks := tokenTasks
		wg.Add(1)
		safeGo("refresh_token", func() {
			defer wg.Done()
			sem := make(chan struct{}, maxConcurrentPerToken)
			var innerWg sync.WaitGroup
			for _, task := range tokenTasks {
				select {
				case <-ctx.Done():
					return
				case <-s.stopped:
					return
				case s.globalSem <- struct{}{}:
					sem <- struct{}{}
					task := task
					innerWg.Add(1)
					safeGo("refresh_mode", func() {
						defer innerWg.Done()
						defer func() { <-s.globalSem; <-sem }()
						s.refreshMode(ctx, tokenID, task.authToken, task.mode)
					})
				}
			}
			innerWg.Wait()
		})
	}
	wg.Wait()
}

func (s *Scheduler) refreshMode(ctx context.Context, tokenID uint, authToken string, mode modelconfig.ModeSpec) {
	resp, err := s.manager.SyncModeQuota(ctx, tokenID, authToken, s.baseURL, mode.UpstreamName)
	if err != nil {
		slog.Warn("refresh: mode quota sync failed",
			"token_id", tokenID, "mode", mode.ID,
			"upstream_name", mode.UpstreamName, "error", err)
		// Failure: keep current values, don't clear first_used_at
		return
	}

	// Success: update quotas
	remaining := resp.RemainingQueries
	limit := resp.TotalQueries
	if limit <= 0 {
		limit = remaining // fallback if upstream doesn't report total
	}
	// Clamp: remaining <= limit
	if remaining > limit {
		remaining = limit
	}

	s.manager.UpdateModeQuota(tokenID, mode.ID, remaining, limit)

	// Learn observed window from upstream
	s.mu.Lock()
	if resp.WindowSizeSeconds > 0 {
		s.observedWindow[mode.ID] = resp.WindowSizeSeconds
	}
	// Clear first_used_at for this token+mode (successful refresh)
	if modeMap, ok := s.firstUsedAt[tokenID]; ok {
		delete(modeMap, mode.ID)
		if len(modeMap) == 0 {
			delete(s.firstUsedAt, tokenID)
		}
	}
	s.mu.Unlock()

	slog.Debug("refresh: mode quota updated",
		"token_id", tokenID, "mode", mode.ID,
		"remaining", remaining, "limit", limit,
		"observed_window", resp.WindowSizeSeconds)
}

// poolToShort converts canonical pool name to short form for catalog lookup.
func poolToShort(pool string) string {
	switch pool {
	case PoolBasic:
		return "basic"
	case PoolSuper:
		return "super"
	case PoolHeavy:
		return "heavy"
	default:
		return pool
	}
}
