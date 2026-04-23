package token

import (
	"context"
	"time"
)

// FirstUseTracker tracks first-used timestamps for token+mode refresh windows.
type FirstUseTracker interface {
	RecordFirstUsed(tokenID uint, mode string)
	SetFirstUsedAt(tokenID uint, mode string, t time.Time)
}

// ManualQuotaRefresher performs an immediate quota refresh for a token.
type ManualQuotaRefresher interface {
	RefreshToken(ctx context.Context, id uint) error
}
