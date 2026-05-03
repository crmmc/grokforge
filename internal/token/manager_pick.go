package token

import (
	"log/slog"
	"time"

	"github.com/crmmc/grokforge/internal/store"
)

// PickAnyExcluding selects an active token while ignoring mode quota.
func (m *TokenManager) PickAnyExcluding(poolName string, exclude map[uint]struct{}) (*store.Token, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	pool, ok := m.pools[poolName]
	if !ok {
		return nil, ErrNoTokenAvailable
	}
	algo := m.selectionAlgorithm()
	exclude = m.addInflightExcludes(exclude)
	penaltyExclude := m.addRecentUseExcludes(exclude)

	token := pool.SelectAnyExcluding(algo, penaltyExclude)
	if token == nil && len(penaltyExclude) > len(exclude) {
		token = pool.SelectAnyExcluding(algo, exclude)
	}
	if token == nil {
		return nil, ErrNoTokenAvailable
	}
	m.markPicked(token)
	return cloneToken(token), nil
}

func (m *TokenManager) pick(poolName string, mode string, exclude map[uint]struct{}) (*store.Token, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if mode == "" {
		return nil, ErrNoTokenAvailable
	}
	pool, ok := m.pools[poolName]
	if !ok {
		return nil, ErrNoTokenAvailable
	}
	token := m.selectTrackedToken(pool, mode, exclude)
	if token == nil {
		return nil, ErrNoTokenAvailable
	}
	if token.Quotas == nil {
		token.Quotas = make(store.IntMap)
	}
	token.Quotas[mode]--
	m.markPicked(token)
	slog.Debug("token: quota pre-deducted",
		"token_id", token.ID, "pool", token.Pool, "mode", mode,
		"action", "pre_deduct", "remaining", token.Quotas[mode],
		"limit", token.LimitQuotas[mode])
	return cloneToken(token), nil
}

func (m *TokenManager) selectTrackedToken(
	pool *TokenPool,
	mode string,
	exclude map[uint]struct{},
) *store.Token {
	exclude = m.addInflightExcludes(exclude)
	penaltyExclude := m.addRecentUseExcludes(exclude)
	token := pool.SelectExcluding(m.selectionAlgorithm(), mode, penaltyExclude)
	if token == nil && len(penaltyExclude) > len(exclude) {
		return pool.SelectExcluding(m.selectionAlgorithm(), mode, exclude)
	}
	return token
}

func (m *TokenManager) selectionAlgorithm() string {
	if m.cfg.SelectionAlgorithm == "" {
		return AlgoHighQuotaFirst
	}
	return m.cfg.SelectionAlgorithm
}

func (m *TokenManager) markPicked(token *store.Token) {
	m.inflight[token.ID]++
	now := time.Now()
	token.LastUsed = &now
	m.lastPickedAt[token.ID] = now
	m.dirty[token.ID] = struct{}{}
}
