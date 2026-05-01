package token

import (
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
)

var (
	// ErrNoTokenAvailable is returned when no token is available in the pool.
	ErrNoTokenAvailable = errors.New("no available token")
)

// TokenManager manages token pools and state transitions.
type TokenManager struct {
	cfg          *config.TokenConfig
	pools        map[string]*TokenPool
	tokens       map[uint]*store.Token // all tokens by ID for quick lookup
	dirty        map[uint]struct{}     // tokens that need persistence
	inflight     map[uint]int          // token ID → active request count (memory-only)
	lastPickedAt map[uint]time.Time    // token ID → last pick timestamp (memory-only)
	mu           sync.RWMutex
}

// NewTokenManager creates a new token manager.
func NewTokenManager(cfg *config.TokenConfig) *TokenManager {
	return &TokenManager{
		cfg:          cfg,
		pools:        make(map[string]*TokenPool),
		tokens:       make(map[uint]*store.Token),
		dirty:        make(map[uint]struct{}),
		inflight:     make(map[uint]int),
		lastPickedAt: make(map[uint]time.Time),
	}
}

// AddToken adds a token to the appropriate pool.
func (m *TokenManager) AddToken(token *store.Token) {
	m.mu.Lock()
	defer m.mu.Unlock()

	pool, ok := m.pools[token.Pool]
	if !ok {
		pool = NewTokenPool(token.Pool)
		m.pools[token.Pool] = pool
	}
	pool.Add(token)
	m.tokens[token.ID] = token
}

// RemoveToken removes a token from its pool.
func (m *TokenManager) RemoveToken(id uint) {
	m.mu.Lock()
	defer m.mu.Unlock()

	token, ok := m.tokens[id]
	if !ok {
		return
	}
	if pool, ok := m.pools[token.Pool]; ok {
		pool.Remove(id)
	}
	delete(m.tokens, id)
	delete(m.dirty, id)
	delete(m.inflight, id)
	delete(m.lastPickedAt, id)
}

// GetToken returns a token by ID.
func (m *TokenManager) GetToken(id uint) *store.Token {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tokens[id]
}

// Pick selects a token from the specified pool using the configured selection algorithm.
// Performs optimistic quota deduction on selection.
func (m *TokenManager) Pick(poolName string, mode string) (*store.Token, error) {
	return m.pick(poolName, mode, nil)
}

// PickExcluding selects a token while skipping excluded token IDs.
func (m *TokenManager) PickExcluding(poolName string, mode string, exclude map[uint]struct{}) (*store.Token, error) {
	return m.pick(poolName, mode, exclude)
}

// PickAnyExcluding selects an active token while ignoring mode quota.
// Does not check per-mode cooling (no mode context); image_ws has its own cooldown.
func (m *TokenManager) PickAnyExcluding(poolName string, exclude map[uint]struct{}) (*store.Token, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	pool, ok := m.pools[poolName]
	if !ok {
		return nil, ErrNoTokenAvailable
	}

	algo := m.cfg.SelectionAlgorithm
	if algo == "" {
		algo = AlgoHighQuotaFirst
	}

	// Build inflight-full exclude set (hard exclude).
	exclude = m.addInflightExcludes(exclude)

	// First attempt: apply recent-use penalty (soft exclude, copy semantics).
	penaltyExclude := m.addRecentUseExcludes(exclude)

	token := pool.SelectAnyExcluding(algo, penaltyExclude)
	if token == nil && len(penaltyExclude) > len(exclude) {
		// Fallback: retry without recent-use penalty.
		token = pool.SelectAnyExcluding(algo, exclude)
	}

	if token == nil {
		return nil, ErrNoTokenAvailable
	}

	m.inflight[token.ID]++
	now := time.Now()
	token.LastUsed = &now
	m.lastPickedAt[token.ID] = now
	m.dirty[token.ID] = struct{}{}
	return token, nil
}

func (m *TokenManager) pick(poolName string, mode string, exclude map[uint]struct{}) (*store.Token, error) {
	m.mu.Lock() // 写锁，因为要修改 quota
	defer m.mu.Unlock()

	pool, ok := m.pools[poolName]
	if !ok {
		return nil, ErrNoTokenAvailable
	}

	algo := m.cfg.SelectionAlgorithm
	if algo == "" {
		algo = AlgoHighQuotaFirst
	}
	if mode == "" {
		return nil, ErrNoTokenAvailable
	}

	// Build inflight-full exclude set (hard exclude).
	exclude = m.addInflightExcludes(exclude)

	// First attempt: apply recent-use penalty (soft exclude, copy semantics).
	penaltyExclude := m.addRecentUseExcludes(exclude)

	token := pool.SelectExcluding(algo, mode, penaltyExclude)
	if token == nil && len(penaltyExclude) > len(exclude) {
		// Fallback: retry without recent-use penalty.
		token = pool.SelectExcluding(algo, mode, exclude)
	}

	if token != nil {
		// 乐观预扣：pick 即扣减
		if token.Quotas == nil {
			token.Quotas = make(store.IntMap)
		}
		token.Quotas[mode]--
		m.inflight[token.ID]++
		now := time.Now()
		token.LastUsed = &now
		m.lastPickedAt[token.ID] = now
		m.dirty[token.ID] = struct{}{}
		slog.Debug("token: quota pre-deducted",
			"token_id", token.ID,
			"pool", token.Pool,
			"mode", mode,
			"action", "pre_deduct",
			"remaining", token.Quotas[mode],
			"limit", token.LimitQuotas[mode])
		return token, nil
	}

	return nil, ErrNoTokenAvailable
}

// MarkSuccess transitions a token back to active state.
func (m *TokenManager) MarkSuccess(id uint) {
	m.mu.Lock()
	defer m.mu.Unlock()

	token, ok := m.tokens[id]
	if !ok {
		return
	}

	token.Status = string(StatusActive)
	token.StatusReason = ""
	token.FailCount = 0
	m.dirty[id] = struct{}{}
	m.releaseInflight(id)
}

// MarkFailed increments fail count and disables if threshold reached.
// When FailThreshold <= 0, the token is never auto-disabled (unlimited).
func (m *TokenManager) MarkFailed(id uint, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	token, ok := m.tokens[id]
	if !ok {
		return
	}

	token.FailCount++
	if m.cfg.FailThreshold > 0 && token.FailCount >= m.cfg.FailThreshold {
		token.Status = string(StatusDisabled)
		token.StatusReason = reason
	}
	m.dirty[id] = struct{}{}
	m.releaseInflight(id)
}

// MarkFailedKeepInflight increments fail count without releasing inflight.
// Used when the same request will retry with the same selected token.
func (m *TokenManager) MarkFailedKeepInflight(id uint, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	token, ok := m.tokens[id]
	if !ok {
		return
	}

	token.FailCount++
	if m.cfg.FailThreshold > 0 && token.FailCount >= m.cfg.FailThreshold {
		token.Status = string(StatusDisabled)
		token.StatusReason = reason
	}
	m.dirty[id] = struct{}{}
}

// MarkDisabled transitions a token to disabled state (manual user action).
func (m *TokenManager) MarkDisabled(id uint, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	token, ok := m.tokens[id]
	if !ok {
		return
	}

	token.Status = string(StatusDisabled)
	token.StatusReason = reason
	m.dirty[id] = struct{}{}
}

// MarkExpired transitions a token to expired state (auto-detected invalid, e.g. 401).
func (m *TokenManager) MarkExpired(id uint, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	token, ok := m.tokens[id]
	if !ok {
		return
	}

	token.Status = string(StatusExpired)
	token.StatusReason = reason
	m.dirty[id] = struct{}{}
	m.releaseInflight(id)
}

// RefundQuota restores one unit of quota for the given mode.
// Used when a recoverable error occurs after optimistic deduction.
func (m *TokenManager) RefundQuota(id uint, mode string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	token, ok := m.tokens[id]
	if !ok {
		return
	}

	if token.Quotas == nil {
		token.Quotas = make(store.IntMap)
	}
	limit := 0
	if token.LimitQuotas != nil {
		limit = token.LimitQuotas[mode]
	}
	refunded := token.Quotas[mode] + 1
	if limit > 0 && refunded > limit {
		refunded = limit
	}
	token.Quotas[mode] = refunded
	m.dirty[id] = struct{}{}
	slog.Debug("token: quota refunded",
		"token_id", token.ID,
		"pool", token.Pool,
		"mode", mode,
		"action", "refund",
		"remaining", token.Quotas[mode],
		"limit", limit)
}

// ClearModeQuota sets the quota for a specific mode to zero.
// Used when a 429 rate limit is received for a specific mode.
func (m *TokenManager) ClearModeQuota(id uint, mode string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	token, ok := m.tokens[id]
	if !ok {
		return
	}

	if token.Quotas == nil {
		token.Quotas = make(store.IntMap)
	}
	token.Quotas[mode] = 0
	m.dirty[id] = struct{}{}
	limit := 0
	if token.LimitQuotas != nil {
		limit = token.LimitQuotas[mode]
	}
	slog.Debug("token: mode quota cleared",
		"token_id", token.ID,
		"pool", token.Pool,
		"mode", mode,
		"action", "clear_mode_quota",
		"remaining", 0,
		"limit", limit)
}

// UpdateModeQuota sets both quota and limit for a specific mode.
// Used by the refresh scheduler after fetching upstream rate limits.
func (m *TokenManager) UpdateModeQuota(id uint, mode string, remaining int, limit int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	token, ok := m.tokens[id]
	if !ok {
		return
	}

	if token.Quotas == nil {
		token.Quotas = make(store.IntMap)
	}
	if token.LimitQuotas == nil {
		token.LimitQuotas = make(store.IntMap)
	}
	if limit > 0 && remaining > limit {
		remaining = limit
	}
	token.Quotas[mode] = remaining
	token.LimitQuotas[mode] = limit
	m.dirty[id] = struct{}{}
	slog.Debug("token: mode quota updated",
		"token_id", token.ID,
		"pool", token.Pool,
		"mode", mode,
		"action", "update_mode_quota",
		"remaining", remaining,
		"limit", limit)
}

// TokenSnapshot holds a copy of token data for safe persistence.
type TokenSnapshot struct {
	ID           uint
	Status       string
	StatusReason string
	Quotas       store.IntMap
	LimitQuotas  store.IntMap
	FailCount    int
	LastUsed     *time.Time
	CoolUntils   store.IntMap
}

// GetDirtyTokens returns snapshots of tokens that have been modified.
// Returns copies to avoid race conditions with concurrent modifications.
// Call ClearDirty after successful persistence to avoid data loss on DB failure.
func (m *TokenManager) GetDirtyTokens() []TokenSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]TokenSnapshot, 0, len(m.dirty))
	for id := range m.dirty {
		if token, ok := m.tokens[id]; ok {
			snapshot := TokenSnapshot{
				ID:           token.ID,
				Status:       token.Status,
				StatusReason: token.StatusReason,
				Quotas:       copyIntMap(token.Quotas),
				LimitQuotas:  copyIntMap(token.LimitQuotas),
				FailCount:    token.FailCount,
				CoolUntils:   copyIntMap(token.CoolUntils),
			}
			if token.LastUsed != nil {
				t := *token.LastUsed
				snapshot.LastUsed = &t
			}
			result = append(result, snapshot)
		}
	}
	return result
}

func copyIntMap(m store.IntMap) store.IntMap {
	if m == nil {
		return nil
	}
	cp := make(store.IntMap, len(m))
	for k, v := range m {
		cp[k] = v
	}
	return cp
}

// MarkDirty marks a token snapshot for persistence.
func (m *TokenManager) MarkDirty(id uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tokens[id]; ok {
		m.dirty[id] = struct{}{}
	}
}

// ClearDirty removes the given token IDs from the dirty set.
// Call this only after successful persistence.
func (m *TokenManager) ClearDirty(ids []uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		delete(m.dirty, id)
	}
}

// UpdateConfig replaces the token config with a copy of the provided config.
// Used for hot-reloading config changes from the admin API.
func (m *TokenManager) UpdateConfig(tc *config.TokenConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := *tc
	m.cfg = &copied
}

// GetPool returns a pool by name.
func (m *TokenManager) GetPool(name string) *TokenPool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pools[name]
}

// GetTokenPool returns the pool name for a token by ID.
func (m *TokenManager) GetTokenPool(id uint) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if token, ok := m.tokens[id]; ok {
		return token.Pool
	}
	return ""
}

// ClearModeQuotaAndCool clears mode quota and sets per-mode cooling timestamp.
// Used on 429 rate limit: prevents the token from being selected for this mode
// even after quota refresh, until the cooling period expires.
func (m *TokenManager) ClearModeQuotaAndCool(id uint, mode string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	token, ok := m.tokens[id]
	if !ok {
		return
	}

	// Clear quota for the specific mode.
	if token.Quotas == nil {
		token.Quotas = make(store.IntMap)
	}
	token.Quotas[mode] = 0

	// Set per-mode cooling timestamp (monotonic increase: never shorten).
	coolDur := m.coolDurationForPool(token.Pool)
	until := int(time.Now().Add(coolDur).Unix())
	if token.CoolUntils == nil {
		token.CoolUntils = make(store.IntMap)
	}
	if existing := token.CoolUntils[mode]; until > existing {
		token.CoolUntils[mode] = until
	}

	m.releaseInflight(id)
	m.dirty[id] = struct{}{}

	limit := 0
	if token.LimitQuotas != nil {
		limit = token.LimitQuotas[mode]
	}
	slog.Debug("token: mode quota cleared with cooling",
		"token_id", token.ID,
		"pool", token.Pool,
		"mode", mode,
		"action", "clear_mode_quota_cool",
		"cool_until", token.CoolUntils[mode],
		"cool_duration_sec", int(coolDur.Seconds()),
		"limit", limit)
}

// ReleaseInflightOnly decrements the inflight counter without any state change.
// Used for error paths that don't call a Report/Mark method (e.g. 403, CF challenge).
func (m *TokenManager) ReleaseInflightOnly(id uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.releaseInflight(id)
}

// GetInflight returns the current inflight count for a token.
func (m *TokenManager) GetInflight(id uint) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.inflight[id]
}

// releaseInflight decrements the inflight counter. Caller must hold m.mu write lock.
func (m *TokenManager) releaseInflight(id uint) {
	if m.inflight[id] > 0 {
		m.inflight[id]--
	}
	if m.inflight[id] == 0 {
		delete(m.inflight, id)
	}
}

// addInflightExcludes returns a copy of exclude with tokens at max inflight added.
// Caller must hold m.mu.
func (m *TokenManager) addInflightExcludes(exclude map[uint]struct{}) map[uint]struct{} {
	if m.cfg.MaxInflight <= 0 {
		return exclude
	}
	for id, count := range m.inflight {
		if count >= m.cfg.MaxInflight {
			if exclude == nil {
				exclude = make(map[uint]struct{})
			}
			exclude[id] = struct{}{}
		}
	}
	return exclude
}

// addRecentUseExcludes returns a NEW map with tokens picked within the
// recent-use penalty window added. The input exclude map is NOT modified
// (copy semantics), so the caller can use the original for fallback.
// Caller must hold m.mu.
func (m *TokenManager) addRecentUseExcludes(exclude map[uint]struct{}) map[uint]struct{} {
	window := m.cfg.RecentUsePenaltySec
	if window <= 0 {
		return exclude
	}
	cutoff := time.Now().Add(-time.Duration(window) * time.Second)

	// Collect IDs to penalize, lazily cleaning expired entries.
	var penalized []uint
	for id, pickedAt := range m.lastPickedAt {
		if pickedAt.After(cutoff) {
			penalized = append(penalized, id)
		} else {
			delete(m.lastPickedAt, id)
		}
	}
	if len(penalized) == 0 {
		return exclude
	}

	// Copy the original exclude map, then add penalty entries.
	result := make(map[uint]struct{}, len(exclude)+len(penalized))
	for id := range exclude {
		result[id] = struct{}{}
	}
	for _, id := range penalized {
		result[id] = struct{}{}
	}
	return result
}

// coolDurationForPool returns the cooling duration for the given pool.
func (m *TokenManager) coolDurationForPool(pool string) time.Duration {
	switch pool {
	case string(PoolBasic):
		if m.cfg.CoolDurationBasicSec > 0 {
			return time.Duration(m.cfg.CoolDurationBasicSec) * time.Second
		}
		return 86400 * time.Second
	case string(PoolSuper):
		if m.cfg.CoolDurationSuperSec > 0 {
			return time.Duration(m.cfg.CoolDurationSuperSec) * time.Second
		}
		return 7200 * time.Second
	case string(PoolHeavy):
		if m.cfg.CoolDurationHeavySec > 0 {
			return time.Duration(m.cfg.CoolDurationHeavySec) * time.Second
		}
		return 7200 * time.Second
	default:
		return 7200 * time.Second
	}
}
