package token

import (
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestManager_AddRecentUseExcludes_NoRecentPicksKeepsExclude(t *testing.T) {
	mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3, RecentUsePenaltySec: 60})

	mgr.mu.Lock()
	got := mgr.addRecentUseExcludes(nil)
	mgr.mu.Unlock()

	assert.Nil(t, got, "no recent picks must keep the original exclude set")
}

func TestManager_RecentlyPickedIDs_PrunesStaleEntries(t *testing.T) {
	mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})

	cutoff := time.Now()
	mgr.mu.Lock()
	mgr.lastPickedAt[1] = cutoff.Add(-2 * time.Minute)
	mgr.lastPickedAt[2] = cutoff.Add(time.Minute)
	penalized := mgr.recentlyPickedIDs(cutoff)
	mgr.mu.Unlock()

	assert.Equal(t, []uint{2}, penalized)

	mgr.mu.RLock()
	_, staleKept := mgr.lastPickedAt[1]
	mgr.mu.RUnlock()
	assert.False(t, staleKept, "stale entries must be pruned")
}
