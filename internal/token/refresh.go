package token

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/crmmc/grokforge/internal/config"
)

const (
	// defaultRefreshInterval is the unified interval for quota recovery scanning.
	defaultRefreshInterval = 2 * time.Hour
	// maxConcurrentRefresh limits concurrent API calls.
	maxConcurrentRefresh = 5
)

// RecoveryMode defines how token quotas are recovered.
const (
	RecoveryModeUpstream = "upstream" // Sync from upstream API
	RecoveryModeAuto     = "auto"     // Restore to configured defaults when cooling expires
)

// Scheduler periodically scans cooling tokens and recovers them.
// Both modes share the same scan loop (check CoolUntil expiry):
//   - "upstream": fetches quota from upstream API
//   - "auto": restores to configured default quotas
type Scheduler struct {
	manager    *TokenManager
	cfg        *config.TokenConfig
	configFunc func() *config.TokenConfig
	baseURL    string
	interval   time.Duration
	sem        chan struct{}
	wg         sync.WaitGroup
	stopOnce   sync.Once
	stopped    chan struct{}
}

// NewScheduler creates a new quota recovery scheduler.
func NewScheduler(manager *TokenManager, cfg *config.TokenConfig, baseURL string) *Scheduler {
	return &Scheduler{
		manager:  manager,
		cfg:      cfg,
		baseURL:  baseURL,
		interval: defaultRefreshInterval,
		sem:      make(chan struct{}, maxConcurrentRefresh),
		stopped:  make(chan struct{}),
	}
}

// SetConfigProvider sets a dynamic token config provider.
func (s *Scheduler) SetConfigProvider(fn func() *config.TokenConfig) {
	s.configFunc = fn
}

// Start begins the periodic refresh loop.
func (s *Scheduler) Start(ctx context.Context) {
	s.wg.Add(1)
	safeGo("token_refresh_scheduler", func() {
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

// run is the main refresh loop.
func (s *Scheduler) run(ctx context.Context) {
	defer s.wg.Done()

	// Run immediately on start
	s.refreshExpiredCooling(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.stopped:
			return
		case <-ticker.C:
			s.refreshExpiredCooling(ctx)
		}
	}
}

// refreshExpiredCooling scans cooling tokens with expired CoolUntil and recovers them.
// In upstream mode, each token's quota is synced from the API.
// In auto mode, each token is restored to configured defaults.
func (s *Scheduler) refreshExpiredCooling(ctx context.Context) {
	tokens := s.manager.GetCoolingTokens()
	now := time.Now()

	var toRefresh []TokenSnapshot
	for _, t := range tokens {
		if t.CoolUntil != nil && t.CoolUntil.Before(now) {
			toRefresh = append(toRefresh, t)
		}
	}

	if len(toRefresh) == 0 {
		return
	}

	mode := s.currentConfig().QuotaRecoveryMode
	if mode == "" {
		mode = RecoveryModeAuto
	}

	slog.Debug("refreshing expired cooling tokens", "count", len(toRefresh), "mode", mode)

	var wg sync.WaitGroup
	for _, token := range toRefresh {
		select {
		case <-ctx.Done():
			return
		case <-s.stopped:
			return
		case s.sem <- struct{}{}:
			tokenSnapshot := token
			wg.Add(1)
			safeGo("token_refresh_token", func() {
				defer wg.Done()
				defer func() { <-s.sem }()
				switch mode {
				case RecoveryModeUpstream:
					s.refreshToken(ctx, tokenSnapshot)
				default: // auto
					s.autoRestoreToken(tokenSnapshot)
				}
			})
		}
	}
	wg.Wait()
}

// refreshToken syncs quota for a single token from upstream API.
func (s *Scheduler) refreshToken(ctx context.Context, token TokenSnapshot) {
	storeToken := s.manager.GetToken(token.ID)
	if storeToken == nil {
		return
	}
	if err := s.manager.SyncQuota(ctx, storeToken, s.baseURL); err != nil {
		slog.Warn("failed to refresh token quota",
			"token_id", token.ID,
			"error", err,
		)
		return
	}

	slog.Debug("refreshed token quota", "token_id", token.ID)
}

// autoRestoreToken restores a single token to configured default quotas.
func (s *Scheduler) autoRestoreToken(t TokenSnapshot) {
	cfg := s.currentConfig()
	chatQ := cfg.DefaultChatQuota
	imageQ := cfg.DefaultImageQuota
	videoQ := cfg.DefaultVideoQuota
	if chatQ <= 0 {
		chatQ = 50
	}
	if imageQ <= 0 {
		imageQ = 20
	}
	if videoQ <= 0 {
		videoQ = 10
	}

	s.manager.RestoreToken(t.ID, chatQ, imageQ, videoQ)
	slog.Debug("auto-restored token quota",
		"token_id", t.ID, "chat", chatQ, "image", imageQ, "video", videoQ)
}

func (s *Scheduler) currentConfig() *config.TokenConfig {
	if s.configFunc != nil {
		return s.configFunc()
	}
	return s.cfg
}
