package token

import (
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waitUntil polls cond using a ticker channel until it holds or the timeout
// elapses. It avoids sleep-based synchronization while keeping tests bounded.
func waitUntil(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	if cond() {
		return
	}
	tick := time.NewTicker(2 * time.Millisecond)
	defer tick.Stop()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		select {
		case <-tick.C:
			if cond() {
				return
			}
		case <-deadline.C:
			t.Fatalf("condition not met within %v: %s", timeout, msg)
		}
	}
}

func TestManager_RemoveToken(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3, MaxInflight: 8}
	mgr := NewTokenManager(cfg)
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 10}})

	_, err := mgr.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	require.Equal(t, 1, mgr.GetInflight(1))

	mgr.RemoveToken(1)

	assert.Nil(t, mgr.GetToken(1))
	assert.Empty(t, mgr.GetTokenPool(1))
	assert.Equal(t, 0, mgr.GetInflight(1))
	assert.Empty(t, mgr.GetDirtyTokens(), "removed token must not stay dirty")

	_, err = mgr.Pick(PoolBasic, "auto")
	assert.ErrorIs(t, err, ErrNoTokenAvailable)

	mgr.RemoveToken(999) // unknown id is a no-op
}

func TestManager_MarkUnknownIDIsNoop(t *testing.T) {
	mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})

	mgr.MarkSuccess(999)
	mgr.MarkFailed(999, "boom")
	mgr.MarkFailedKeepInflight(999, "boom")
	mgr.MarkDisabled(999, "boom")
	mgr.MarkExpired(999, "boom")

	assert.Nil(t, mgr.GetToken(999))
	assert.Empty(t, mgr.GetDirtyTokens())
}

func TestManager_MarkFailedKeepInflight_DisablesAtThreshold(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 1, MaxInflight: 8}
	mgr := NewTokenManager(cfg)
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 10}})

	_, err := mgr.Pick(PoolBasic, "auto")
	require.NoError(t, err)

	mgr.MarkFailedKeepInflight(1, "retry with same token")

	token := mgr.GetToken(1)
	require.NotNil(t, token)
	assert.Equal(t, string(StatusDisabled), token.Status)
	assert.Equal(t, "retry with same token", token.StatusReason)
	assert.Equal(t, 1, mgr.GetInflight(1), "keep-inflight must not release the counter")
}

func TestManager_Pick_EmptyModeReturnsNoToken(t *testing.T) {
	mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 10}})

	_, err := mgr.Pick(PoolBasic, "")
	assert.ErrorIs(t, err, ErrNoTokenAvailable)
}

func TestManager_GetDirtyTokens_SkipsUnknownDirtyID(t *testing.T) {
	mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 10}})
	_ = mgr.GetDirtyTokens() // drain dirty state created by AddToken

	mgr.mu.Lock()
	mgr.dirty[4242] = struct{}{} // stale entry without a backing token
	mgr.mu.Unlock()

	assert.Empty(t, mgr.GetDirtyTokens(), "dirty ids without tokens must be skipped")
}

func TestManager_UpdateConfig_AppliesNewConfig(t *testing.T) {
	mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 10}})

	mgr.UpdateConfig(&config.TokenConfig{FailThreshold: 1})
	mgr.MarkFailed(1, "boom")

	token := mgr.GetToken(1)
	require.NotNil(t, token)
	assert.Equal(t, string(StatusDisabled), token.Status, "new threshold must apply to state transitions")
}
