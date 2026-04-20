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

// RecoveryModeAuto restores configured default quotas when cooldown expires.
const (
	RecoveryModeAuto     = "auto"
	RecoveryModeUpstream = "upstream"
)

// Scheduler periodically scans cooling tokens and restores quotas when the
// cooldown window has expired.
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

// refreshExpiredCooling scans cooling tokens with expired CoolUntil and
// restores them to configured defaults.
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
	slog.Debug("refreshing expired cooling tokens", "count", len(toRefresh))

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
				s.restoreToken(ctx, tokenSnapshot)
			})
		}
	}
	wg.Wait()
}

func (s *Scheduler) restoreToken(ctx context.Context, t TokenSnapshot) {
	cfg := s.currentConfig()
	mode := RecoveryModeAuto
	if cfg != nil && cfg.QuotaRecoveryMode != "" {
		mode = cfg.QuotaRecoveryMode
	}
	if mode == RecoveryModeUpstream {
		s.syncTokenFromUpstream(ctx, t.ID)
		return
	}
	s.autoRestoreToken(t)
}

// autoRestoreToken restores a single token to configured default quotas.
func (s *Scheduler) autoRestoreToken(t TokenSnapshot) {
	cfg := s.currentConfig()
	if cfg == nil {
		cfg = &config.TokenConfig{}
	}
	chatQ := cfg.DefaultChatQuota
	imageQ := cfg.DefaultImageQuota
	videoQ := cfg.DefaultVideoQuota
	grok43Q := cfg.DefaultGrok43Quota
	if chatQ <= 0 {
		chatQ = 50
	}
	if imageQ <= 0 {
		imageQ = 20
	}
	if videoQ <= 0 {
		videoQ = 10
	}
	if grok43Q <= 0 {
		grok43Q = 25
	}

	s.manager.RestoreToken(t.ID, chatQ, imageQ, videoQ, grok43Q)
	slog.Debug("auto-restored token quota",
		"token_id", t.ID, "chat", chatQ, "image", imageQ, "video", videoQ, "grok43", grok43Q)
}

func (s *Scheduler) syncTokenFromUpstream(ctx context.Context, id uint) {
	token := s.manager.GetToken(id)
	if token == nil {
		return
	}
	authToken := token.Token // string copy, safe to use outside lock
	if err := s.manager.SyncQuota(ctx, id, authToken, s.baseURL); err != nil {
		slog.Warn("token: upstream quota sync failed",
			"token_id", id, "error", err)
		return
	}
	slog.Debug("token: upstream quota restored", "token_id", id)
}

func (s *Scheduler) currentConfig() *config.TokenConfig {
	if s.configFunc != nil {
		return s.configFunc()
	}
	return s.cfg
}
