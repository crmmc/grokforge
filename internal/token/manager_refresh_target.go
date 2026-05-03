package token

import (
	"time"

	"github.com/crmmc/grokforge/internal/modelconfig"
	"github.com/crmmc/grokforge/internal/store"
)

// ExhaustedModeTarget identifies a token+mode that needs upstream refresh.
type ExhaustedModeTarget struct {
	TokenID   uint
	AuthToken string
	Pool      string
	Mode      string
}

// ScanExhaustedModes returns active token modes whose quota is due for refresh.
func (m *TokenManager) ScanExhaustedModes(
	modes []modelconfig.ModeSpec,
	now time.Time,
) []ExhaustedModeTarget {
	m.mu.RLock()
	defer m.mu.RUnlock()

	nowUnix := int(now.Unix())
	targets := make([]ExhaustedModeTarget, 0)
	for _, token := range m.tokens {
		if Status(token.Status) != StatusActive {
			continue
		}
		targets = append(targets, scanTokenModes(token, modes, nowUnix)...)
	}
	return targets
}

func scanTokenModes(
	token *store.Token,
	modes []modelconfig.ModeSpec,
	nowUnix int,
) []ExhaustedModeTarget {
	poolKey := PoolToShort(token.Pool)
	targets := make([]ExhaustedModeTarget, 0)
	for _, mode := range modes {
		if !modeNeedsRefresh(token, mode, poolKey, nowUnix) {
			continue
		}
		targets = append(targets, ExhaustedModeTarget{
			TokenID: token.ID, AuthToken: token.Token, Pool: token.Pool, Mode: mode.ID,
		})
	}
	return targets
}

func modeNeedsRefresh(
	token *store.Token,
	mode modelconfig.ModeSpec,
	poolKey string,
	nowUnix int,
) bool {
	return mode.DefaultQuota[poolKey] > 0 &&
		token.Quotas[mode.ID] == 0 &&
		token.ResumeAts[mode.ID] <= nowUnix
}

// GetActiveTokenInfo returns token credentials for an active token.
func (m *TokenManager) GetActiveTokenInfo(id uint) (authToken string, pool string, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	token, exists := m.tokens[id]
	if !exists || Status(token.Status) != StatusActive {
		return "", "", false
	}
	return token.Token, token.Pool, true
}
