package token

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/modelconfig"
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
	Disabled int
	Expired  int
}

// TokenService provides the high-level API for token management.
type TokenService struct {
	cfg              *config.TokenConfig
	store            TokenStore
	manager          *TokenManager
	modes            []modelconfig.ModeSpec
	baseURL          string
	refreshRequester RefreshRequester
}

// NewTokenService creates a new token service.
func NewTokenService(
	cfg *config.TokenConfig,
	store TokenStore,
	modes []modelconfig.ModeSpec,
	baseURL string,
) *TokenService {
	return &TokenService{
		cfg:     cfg,
		store:   store,
		manager: NewTokenManager(cfg),
		modes:   append([]modelconfig.ModeSpec(nil), modes...),
		baseURL: baseURL,
	}
}

// SetRefreshRequester wires upstream quota refresh requests into the service.
func (s *TokenService) SetRefreshRequester(requester RefreshRequester) {
	s.refreshRequester = requester
}

// LoadTokens loads all tokens from the store into the manager.
func (s *TokenService) LoadTokens(ctx context.Context) error {
	tokens, err := s.store.ListTokens(ctx)
	if err != nil {
		return err
	}

	for _, t := range tokens {
		if err := normalizeTokenPool(t); err != nil {
			return err
		}
		normalized := normalizeTokenQuotas(t, s.modes)
		primeChanged := s.primeZeroModeResumeAts(t, normalized.zeroModes, time.Now())
		s.manager.AddToken(t)
		if normalized.changed || primeChanged {
			s.manager.MarkDirty(t.ID)
		}
	}

	return nil
}

// Pick selects a token from the specified pool.
func (s *TokenService) Pick(pool string, mode string) (*store.Token, error) {
	return s.manager.Pick(pool, mode)
}

// PickExcluding selects a token from the specified pool while skipping excluded IDs.
func (s *TokenService) PickExcluding(pool string, mode string, exclude map[uint]struct{}) (*store.Token, error) {
	return s.manager.PickExcluding(pool, mode, exclude)
}

// PickAnyExcluding selects an active token without checking mode quota.
func (s *TokenService) PickAnyExcluding(pool string, exclude map[uint]struct{}) (*store.Token, error) {
	return s.manager.PickAnyExcluding(pool, exclude)
}

// ReportSuccess marks a token as successfully used.
func (s *TokenService) ReportSuccess(id uint) {
	s.manager.MarkSuccess(id)
}

// ReportRateLimit clears local quota and asks the scheduler for authoritative state.
func (s *TokenService) ReportRateLimit(id uint, mode string, reason string) {
	s.manager.ClearModeQuota(id, mode)
	s.manager.ClearResumeAt(id, mode)
	if s.refreshRequester != nil && mode != "" {
		s.refreshRequester.RequestRefresh(id, mode)
	}
}

// ReportError handles an error for a token.
// If recoverable is true, refunds the pre-deducted quota for the mode.
func (s *TokenService) ReportError(id uint, mode string, recoverable bool, reason string) {
	if recoverable {
		s.manager.RefundQuota(id, mode)
	}
	s.manager.MarkFailed(id, reason)
}

// ReportErrorKeepInflight records an intermediate retry error without
// releasing inflight or refunding quota. The final token exit path decides
// whether the pre-deducted quota should be refunded.
func (s *TokenService) ReportErrorKeepInflight(id uint, mode string, recoverable bool, reason string) {
	s.manager.MarkFailedKeepInflight(id, reason)
}

// MarkDisabled immediately disables a token (manual user action).
func (s *TokenService) MarkDisabled(id uint, reason string) {
	s.manager.MarkDisabled(id, reason)
}

// MarkExpired marks a token as expired (auto-detected invalid, e.g. 401).
func (s *TokenService) MarkExpired(id uint, reason string) {
	s.manager.MarkExpired(id, reason)
}

// RefundQuota restores one unit of quota for the given mode.
func (s *TokenService) RefundQuota(id uint, mode string) {
	s.manager.RefundQuota(id, mode)
}

// ReleaseToken decrements the inflight counter without any state change.
// Used for error paths that don't call a Report/Mark method (e.g. 403, CF challenge).
func (s *TokenService) ReleaseToken(id uint) {
	s.manager.ReleaseInflightOnly(id)
}

// FlushDirty persists all dirty tokens to the store.
func (s *TokenService) FlushDirty(ctx context.Context) error {
	dirty := s.manager.GetDirtyTokens()
	if len(dirty) == 0 {
		return nil
	}
	snapshots := make([]store.TokenSnapshotData, len(dirty))
	ids := make([]uint, len(dirty))
	for i, d := range dirty {
		ids[i] = d.ID
		snapshots[i] = store.TokenSnapshotData{
			ID:           d.ID,
			Status:       d.Status,
			StatusReason: d.StatusReason,
			Quotas:       d.Quotas,
			LimitQuotas:  d.LimitQuotas,
			FailCount:    d.FailCount,
			LastUsed:     d.LastUsed,
			ResumeAts:    d.ResumeAts,
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
		active, disabled, expired := pool.Count()
		result[name] = PoolStats{
			Active:   active,
			Disabled: disabled,
			Expired:  expired,
		}
	}

	return result
}

// BaseURL returns the configured upstream base URL.
func (s *TokenService) BaseURL() string {
	return s.baseURL
}

// Manager returns the underlying token manager.
func (s *TokenService) Manager() *TokenManager {
	return s.manager
}

// AddToPool adds a token to the in-memory pool (called after admin import).
func (s *TokenService) AddToPool(token *store.Token) error {
	if err := normalizeTokenPool(token); err != nil {
		return err
	}
	s.manager.AddToken(token)
	return nil
}

// RemoveFromPool removes a token from the in-memory pool (called after admin delete).
func (s *TokenService) RemoveFromPool(id uint) {
	s.manager.RemoveToken(id)
	if s.refreshRequester != nil {
		s.refreshRequester.ForgetToken(id)
	}
}

// SyncToken reloads a single token from DB into memory (called after admin update).
func (s *TokenService) SyncToken(ctx context.Context, id uint) error {
	dbToken, err := s.store.GetToken(ctx, id)
	if err != nil {
		return err
	}
	if err := normalizeTokenPool(dbToken); err != nil {
		return err
	}
	s.manager.RemoveToken(id)
	if s.refreshRequester != nil {
		s.refreshRequester.ForgetToken(id)
	}
	s.manager.AddToken(dbToken)
	return nil
}

// RefreshToken forces an immediate upstream quota refresh and persists the result.
func (s *TokenService) RefreshToken(ctx context.Context, id uint) (*store.Token, error) {
	if s.refreshRequester == nil {
		return nil, fmt.Errorf("token refresh not configured")
	}
	if err := s.refreshRequester.RefreshToken(ctx, id); err != nil {
		if errors.Is(err, ErrTokenNotFound) {
			return nil, store.ErrNotFound
		}
		return nil, err
	}
	if err := s.FlushDirty(ctx); err != nil {
		return nil, err
	}
	token := s.manager.GetToken(id)
	if token == nil {
		return nil, store.ErrNotFound
	}
	return token, nil
}

func (s *TokenService) primeZeroModeResumeAts(
	token *store.Token,
	zeroModes []string,
	now time.Time,
) bool {
	if len(zeroModes) == 0 {
		return false
	}
	if token.ResumeAts == nil {
		token.ResumeAts = make(store.IntMap)
	}
	changed := false
	for _, mode := range zeroModes {
		if _, exists := token.ResumeAts[mode]; exists {
			continue
		}
		token.ResumeAts[mode] = int(now.Unix())
		changed = true
	}
	return changed
}

func normalizeTokenPool(token *store.Token) error {
	if token == nil {
		return nil
	}
	pool, err := NormalizePoolName(token.Pool)
	if err != nil {
		return fmt.Errorf("token %d has invalid pool %q: %w", token.ID, token.Pool, err)
	}
	token.Pool = pool
	return nil
}
