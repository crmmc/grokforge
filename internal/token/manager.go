package token

import (
	"errors"
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

	owned := cloneToken(token)
	pool, ok := m.pools[owned.Pool]
	if !ok {
		pool = NewTokenPool(owned.Pool)
		m.pools[owned.Pool] = pool
	}
	pool.Add(owned)
	m.tokens[owned.ID] = owned
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

// GetToken returns a token snapshot by ID.
func (m *TokenManager) GetToken(id uint) *store.Token {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return cloneToken(m.tokens[id])
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
