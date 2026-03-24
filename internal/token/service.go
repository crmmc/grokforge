package token

import (
	"context"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
)

// TokenStore defines the interface for token persistence.
type TokenStore interface {
	ListTokens(ctx context.Context) ([]*store.Token, error)
	GetToken(ctx context.Context, id uint) (*store.Token, error)
	UpdateTokenSnapshots(ctx context.Context, snapshots []store.TokenSnapshotData) error
}

// PoolStats holds statistics for a token pool.
type PoolStats struct {
	Active   int
	Cooling  int
	Disabled int
}

// TokenService provides the high-level API for token management.
type TokenService struct {
	cfg     *config.TokenConfig
	store   TokenStore
	manager *TokenManager
	ticker  *CooldownTicker
	baseURL string
}

// NewTokenService creates a new token service.
func NewTokenService(cfg *config.TokenConfig, store TokenStore, baseURL string) *TokenService {
	mgr := NewTokenManager(cfg)

	interval := time.Duration(cfg.CoolCheckIntervalSec) * time.Second
	if interval == 0 {
		interval = 30 * time.Second // default
	}

	return &TokenService{
		cfg:     cfg,
		store:   store,
		manager: mgr,
		ticker:  NewCooldownTicker(mgr, interval),
		baseURL: baseURL,
	}
}

// LoadTokens loads all tokens from the store into the manager.
func (s *TokenService) LoadTokens(ctx context.Context) error {
	tokens, err := s.store.ListTokens(ctx)
	if err != nil {
		return err
	}

	for _, t := range tokens {
		s.manager.AddToken(t)
	}

	// Clear dirty set after initial load
	dirty := s.manager.GetDirtyTokens()
	ids := make([]uint, len(dirty))
	for i, d := range dirty {
		ids[i] = d.ID
	}
	s.manager.ClearDirty(ids)

	return nil
}

// Pick selects a token from the specified pool.
func (s *TokenService) Pick(pool string, cat QuotaCategory) (*store.Token, error) {
	return s.manager.Pick(pool, cat)
}

// PickExcluding selects a token from the specified pool while skipping excluded IDs.
func (s *TokenService) PickExcluding(pool string, cat QuotaCategory, exclude map[uint]struct{}) (*store.Token, error) {
	return s.manager.PickExcluding(pool, cat, exclude)
}

// Consume deducts quota from the token for the given category.
func (s *TokenService) Consume(tokenID uint, cat QuotaCategory, cost int) (int, error) {
	return s.manager.Consume(tokenID, cat, cost)
}

// RefreshToken refreshes a token's quota by syncing with upstream API.
func (s *TokenService) RefreshToken(ctx context.Context, id uint) (*store.Token, error) {
	token := s.manager.GetToken(id)
	if token == nil {
		return nil, ErrTokenNotFound
	}
	if err := s.manager.SyncQuota(ctx, token, s.baseURL); err != nil {
		return nil, err
	}
	return token, nil
}

// ReportSuccess marks a token as successfully used.
func (s *TokenService) ReportSuccess(id uint) {
	s.manager.MarkSuccess(id)
}

// ReportRateLimit marks a token as rate limited (cooling).
// Uses per-group cooldown duration based on the token's pool.
func (s *TokenService) ReportRateLimit(id uint, reason string) {
	pool := s.manager.GetTokenPool(id)
	var durationMin int
	switch pool {
	case PoolSuper:
		durationMin = s.cfg.SuperCoolDurationMin
	default:
		durationMin = s.cfg.BasicCoolDurationMin
	}
	duration := time.Duration(durationMin) * time.Minute
	if duration == 0 {
		duration = DefaultCoolDuration
	}
	s.manager.MarkCooling(id, duration, reason)
}

// ReportError marks a token as having an error.
func (s *TokenService) ReportError(id uint, reason string) {
	s.manager.MarkFailed(id, reason)
}

// MarkDisabled immediately disables a token (manual user action).
func (s *TokenService) MarkDisabled(id uint, reason string) {
	s.manager.MarkDisabled(id, reason)
}

// MarkExpired marks a token as expired (auto-detected invalid, e.g. 401).
func (s *TokenService) MarkExpired(id uint, reason string) {
	s.manager.MarkExpired(id, reason)
}

// FlushDirty persists all dirty tokens to the store.
func (s *TokenService) FlushDirty(ctx context.Context) error {
	dirty := s.manager.GetDirtyTokens()
	if len(dirty) == 0 {
		return nil
	}
	// Convert to store.TokenSnapshotData
	snapshots := make([]store.TokenSnapshotData, len(dirty))
	ids := make([]uint, len(dirty))
	for i, d := range dirty {
		ids[i] = d.ID
		snapshots[i] = store.TokenSnapshotData{
			ID:                d.ID,
			Status:            d.Status,
			StatusReason:      d.StatusReason,
			ChatQuota:         d.ChatQuota,
			InitialChatQuota:  d.InitialChatQuota,
			ImageQuota:        d.ImageQuota,
			InitialImageQuota: d.InitialImageQuota,
			VideoQuota:        d.VideoQuota,
			InitialVideoQuota: d.InitialVideoQuota,
			FailCount:         d.FailCount,
			CoolUntil:         d.CoolUntil,
			LastUsed:          d.LastUsed,
		}
	}
	if err := s.store.UpdateTokenSnapshots(ctx, snapshots); err != nil {
		return err
	}
	// Clear dirty set only after successful persistence
	s.manager.ClearDirty(ids)
	return nil
}

// Stats returns statistics for all pools.
func (s *TokenService) Stats() map[string]PoolStats {
	result := make(map[string]PoolStats)

	s.manager.mu.RLock()
	defer s.manager.mu.RUnlock()

	for name, pool := range s.manager.pools {
		active, cooling, disabled, expired := pool.Count()
		result[name] = PoolStats{
			Active:   active,
			Cooling:  cooling,
			Disabled: disabled + expired,
		}
	}

	return result
}

// StartTicker starts the cooldown ticker in a goroutine.
func (s *TokenService) StartTicker(ctx context.Context) {
	safeGo("token_cooldown_ticker", func() {
		s.ticker.Start(ctx)
	})
}

// Manager returns the underlying token manager.
func (s *TokenService) Manager() *TokenManager {
	return s.manager
}

// AddToPool adds a token to the in-memory pool (called after admin import).
func (s *TokenService) AddToPool(token *store.Token) {
	s.manager.AddToken(token)
}

// RemoveFromPool removes a token from the in-memory pool (called after admin delete).
func (s *TokenService) RemoveFromPool(id uint) {
	s.manager.RemoveToken(id)
}

// SyncToken reloads a single token from DB into memory (called after admin update).
func (s *TokenService) SyncToken(ctx context.Context, id uint) error {
	dbToken, err := s.store.GetToken(ctx, id)
	if err != nil {
		return err
	}
	s.manager.RemoveToken(id)
	s.manager.AddToken(dbToken)
	return nil
}
