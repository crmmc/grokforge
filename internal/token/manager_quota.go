package token

import (
	"log/slog"

	"github.com/crmmc/grokforge/internal/store"
)

// RefundQuota restores one unit of quota for the given mode.
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
	limit := tokenModeLimit(token, mode)
	token.Quotas[mode] = cappedRefund(token.Quotas[mode], limit)
	m.dirty[id] = struct{}{}
	slog.Debug("token: quota refunded",
		"token_id", token.ID, "pool", token.Pool, "mode", mode,
		"action", "refund", "remaining", token.Quotas[mode], "limit", limit)
}

// ClearModeQuota sets the quota for a specific mode to zero and releases inflight.
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
	m.releaseInflight(id)
	m.dirty[id] = struct{}{}
	slog.Debug("token: mode quota cleared",
		"token_id", token.ID, "pool", token.Pool, "mode", mode,
		"action", "clear_mode_quota", "remaining", 0,
		"limit", tokenModeLimit(token, mode))
}

// SetResumeAt records when an exhausted mode should be checked again.
func (m *TokenManager) SetResumeAt(id uint, mode string, resumeUnix int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	token, ok := m.tokens[id]
	if !ok {
		return
	}
	if token.ResumeAts == nil {
		token.ResumeAts = make(store.IntMap)
	}
	token.ResumeAts[mode] = resumeUnix
	m.dirty[id] = struct{}{}
}

// SetResumeAtIfDue records a recovery timestamp only while the mode is still exhausted and due.
func (m *TokenManager) SetResumeAtIfDue(id uint, mode string, resumeUnix int, nowUnix int) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	token, ok := m.tokens[id]
	if !ok || token.Quotas[mode] != 0 {
		return false
	}
	if token.ResumeAts == nil {
		token.ResumeAts = make(store.IntMap)
	}
	if token.ResumeAts[mode] > nowUnix {
		return false
	}
	token.ResumeAts[mode] = resumeUnix
	m.dirty[id] = struct{}{}
	return true
}

// ClearResumeAt removes the recovery timestamp for a token mode.
func (m *TokenManager) ClearResumeAt(id uint, mode string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	token, ok := m.tokens[id]
	if !ok || token.ResumeAts == nil {
		return
	}
	delete(token.ResumeAts, mode)
	m.dirty[id] = struct{}{}
}

// UpdateModeQuota sets both quota and limit for a specific mode.
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
		"token_id", token.ID, "pool", token.Pool, "mode", mode,
		"action", "update_mode_quota", "remaining", remaining, "limit", limit)
}

func tokenModeLimit(token *store.Token, mode string) int {
	if token.LimitQuotas == nil {
		return 0
	}
	return token.LimitQuotas[mode]
}

func cappedRefund(current int, limit int) int {
	refunded := current + 1
	if limit > 0 && refunded > limit {
		return limit
	}
	return refunded
}
