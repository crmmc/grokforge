package token

import (
	"context"
	"time"
)

// CooldownTicker periodically checks and restores expired cooling tokens.
type CooldownTicker struct {
	mgr      *TokenManager
	interval time.Duration
}

// NewCooldownTicker creates a new cooldown ticker.
func NewCooldownTicker(mgr *TokenManager, interval time.Duration) *CooldownTicker {
	return &CooldownTicker{
		mgr:      mgr,
		interval: interval,
	}
}

// Start begins the ticker loop. Blocks until context is cancelled.
func (t *CooldownTicker) Start(ctx context.Context) {
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.CheckNow()
		}
	}
}

// CheckNow immediately checks all cooling tokens and restores expired ones.
// Returns the number of tokens restored.
func (t *CooldownTicker) CheckNow() int {
	t.mgr.mu.Lock()
	defer t.mgr.mu.Unlock()

	now := time.Now()
	restored := 0

	for _, token := range t.mgr.tokens {
		if Status(token.Status) != StatusCooling {
			continue
		}
		if token.CoolUntil == nil {
			continue
		}
		if now.After(*token.CoolUntil) {
			// Restore if any category has quota remaining
			if token.ChatQuota <= 0 && token.ImageQuota <= 0 && token.VideoQuota <= 0 {
				continue
			}
			token.Status = string(StatusActive)
			token.StatusReason = ""
			token.CoolUntil = nil
			t.mgr.dirty[token.ID] = struct{}{}
			restored++
		}
	}

	return restored
}
