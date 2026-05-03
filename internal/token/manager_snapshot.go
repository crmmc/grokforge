package token

import (
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
)

// TokenSnapshot holds a copy of token data for safe persistence.
type TokenSnapshot struct {
	ID           uint
	Status       string
	StatusReason string
	Quotas       store.IntMap
	LimitQuotas  store.IntMap
	FailCount    int
	LastUsed     *time.Time
	ResumeAts    store.IntMap
}

// GetDirtyTokens returns snapshots of tokens that have been modified.
func (m *TokenManager) GetDirtyTokens() []TokenSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]TokenSnapshot, 0, len(m.dirty))
	for id := range m.dirty {
		token, ok := m.tokens[id]
		if !ok {
			continue
		}
		result = append(result, snapshotToken(token))
	}
	return result
}

func snapshotToken(token *store.Token) TokenSnapshot {
	snapshot := TokenSnapshot{
		ID:           token.ID,
		Status:       token.Status,
		StatusReason: token.StatusReason,
		Quotas:       copyIntMap(token.Quotas),
		LimitQuotas:  copyIntMap(token.LimitQuotas),
		FailCount:    token.FailCount,
		ResumeAts:    copyIntMap(token.ResumeAts),
	}
	if token.LastUsed != nil {
		t := *token.LastUsed
		snapshot.LastUsed = &t
	}
	return snapshot
}

func cloneToken(token *store.Token) *store.Token {
	if token == nil {
		return nil
	}
	cloned := *token
	cloned.Quotas = copyIntMap(token.Quotas)
	cloned.LimitQuotas = copyIntMap(token.LimitQuotas)
	cloned.ResumeAts = copyIntMap(token.ResumeAts)
	if token.LastUsed != nil {
		t := *token.LastUsed
		cloned.LastUsed = &t
	}
	return &cloned
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
func (m *TokenManager) ClearDirty(ids []uint) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		delete(m.dirty, id)
	}
}

// UpdateConfig replaces the token config with a copy of the provided config.
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
