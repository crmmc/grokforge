package token

import (
	"testing"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_Pick_ReturnsTokenFromCorrectPool(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	mgr.AddToken(&store.Token{ID: 1, Token: "basic1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 80}})
	mgr.AddToken(&store.Token{ID: 2, Token: "super1", Pool: PoolSuper, Status: string(StatusActive), Quotas: store.IntMap{"auto": 140}})

	token, err := mgr.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	require.NotNil(t, token)
	assert.Equal(t, uint(1), token.ID)

	token, err = mgr.Pick(PoolSuper, "auto")
	require.NoError(t, err)
	require.NotNil(t, token)
	assert.Equal(t, uint(2), token.ID)
}

func TestManager_Pick_NoDisabledFallback(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3, SelectionAlgorithm: AlgoHighQuotaFirst}
	mgr := NewTokenManager(cfg)

	mgr.AddToken(&store.Token{ID: 1, Token: "disabled1", Pool: PoolBasic, Status: string(StatusDisabled), Quotas: store.IntMap{"auto": 80}})

	token, err := mgr.Pick(PoolBasic, "auto")
	assert.ErrorIs(t, err, ErrNoTokenAvailable, "should return ErrNoTokenAvailable when only disabled tokens")
	assert.Nil(t, token)
}

func TestManager_Pick_UsesAlgorithm(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3, SelectionAlgorithm: AlgoRoundRobin}
	mgr := NewTokenManager(cfg)

	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 50}})
	mgr.AddToken(&store.Token{ID: 2, Token: "t2", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 50}})
	mgr.AddToken(&store.Token{ID: 3, Token: "t3", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 50}})

	// With round-robin, 3 calls should cycle through all 3 tokens
	seen := make(map[uint]bool)
	for i := 0; i < 3; i++ {
		token, err := mgr.Pick(PoolBasic, "auto")
		require.NoError(t, err)
		require.NotNil(t, token)
		seen[token.ID] = true
	}
	assert.Len(t, seen, 3, "round-robin should have visited all 3 tokens")
}

func TestManager_PickExcluding_SkipsExcluded(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3, SelectionAlgorithm: AlgoHighQuotaFirst}
	mgr := NewTokenManager(cfg)

	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 100}})
	mgr.AddToken(&store.Token{ID: 2, Token: "t2", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 90}})

	token, err := mgr.PickExcluding(PoolBasic, "auto", map[uint]struct{}{1: {}})
	require.NoError(t, err)
	require.NotNil(t, token)
	assert.Equal(t, uint(2), token.ID)
}

func TestManager_Pick_ReturnsErrorWhenNoTokens(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	token, err := mgr.Pick(PoolBasic, "auto")
	assert.Error(t, err)
	assert.Nil(t, token)
}

func TestManager_Pick_OptimisticDeduction(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 10}})

	token, err := mgr.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	require.NotNil(t, token)
	assert.Equal(t, 9, token.Quotas["auto"], "Pick should deduct 1 from quota")
}

func TestManager_Pick_SkipsZeroQuota(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 0}})
	mgr.AddToken(&store.Token{ID: 2, Token: "t2", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 5}})

	token, err := mgr.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	require.NotNil(t, token)
	assert.Equal(t, uint(2), token.ID, "should skip token with zero quota")
}

func TestManager_MarkSuccess_RestoresActive(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusDisabled), Quotas: store.IntMap{"auto": 80}, FailCount: 2})

	mgr.MarkSuccess(1)

	token := mgr.GetToken(1)
	require.NotNil(t, token)
	assert.Equal(t, string(StatusActive), token.Status)
	assert.Equal(t, 0, token.FailCount)
}

func TestManager_MarkFailed_IncrementsFailCount(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 80}, FailCount: 0})

	mgr.MarkFailed(1, "test error")
	token := mgr.GetToken(1)
	assert.Equal(t, 1, token.FailCount)

	mgr.MarkFailed(1, "test error")
	token = mgr.GetToken(1)
	assert.Equal(t, 2, token.FailCount)
}

func TestManager_MarkFailed_DisablesAtThreshold(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 80}, FailCount: 2})

	mgr.MarkFailed(1, "test error") // 3rd failure

	token := mgr.GetToken(1)
	require.NotNil(t, token)
	assert.Equal(t, string(StatusDisabled), token.Status)
}

func TestManager_MarkFailed_ZeroThresholdNeverDisables(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 0}
	mgr := NewTokenManager(cfg)

	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 80}, FailCount: 0})

	// Even after many failures, token should stay active
	for i := 0; i < 100; i++ {
		mgr.MarkFailed(1, "test error")
	}

	token := mgr.GetToken(1)
	require.NotNil(t, token)
	assert.Equal(t, string(StatusActive), token.Status)
	assert.Equal(t, 100, token.FailCount)
}

func TestManager_DirtyTracking(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 80}})
	mgr.AddToken(&store.Token{ID: 2, Token: "t2", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 80}})

	// Clear any dirty from Add
	_ = mgr.GetDirtyTokens()
	mgr.ClearDirty([]uint{1, 2})

	mgr.RefundQuota(1, "auto")
	mgr.MarkFailed(2, "test error")

	dirty := mgr.GetDirtyTokens()
	assert.Len(t, dirty, 2, "should have 2 dirty tokens")

	// ClearDirty should remove from dirty set
	mgr.ClearDirty([]uint{1, 2})
	dirty = mgr.GetDirtyTokens()
	assert.Len(t, dirty, 0, "dirty set should be cleared after ClearDirty")
}

func TestManager_RefundQuota(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 9}})

	mgr.RefundQuota(1, "auto")

	token := mgr.GetToken(1)
	require.NotNil(t, token)
	assert.Equal(t, 10, token.Quotas["auto"], "RefundQuota should increment quota by 1")
}

func TestManager_ClearModeQuota(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 50}})

	mgr.ClearModeQuota(1, "auto")

	token := mgr.GetToken(1)
	require.NotNil(t, token)
	assert.Equal(t, 0, token.Quotas["auto"], "ClearModeQuota should set quota to 0")
}

func TestManager_UpdateModeQuota(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 5}, LimitQuotas: store.IntMap{"auto": 10}})

	mgr.UpdateModeQuota(1, "auto", 42, 80)

	token := mgr.GetToken(1)
	require.NotNil(t, token)
	assert.Equal(t, 42, token.Quotas["auto"], "UpdateModeQuota should set remaining")
	assert.Equal(t, 80, token.LimitQuotas["auto"], "UpdateModeQuota should set limit")
}

func TestManager_UpdateModeQuota_InitializesNilMaps(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive)})

	mgr.UpdateModeQuota(1, "fast", 30, 50)

	token := mgr.GetToken(1)
	require.NotNil(t, token)
	assert.Equal(t, 30, token.Quotas["fast"])
	assert.Equal(t, 50, token.LimitQuotas["fast"])
}

func TestManager_MarkExpired(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 80}})

	mgr.MarkExpired(1, "401 unauthorized")

	token := mgr.GetToken(1)
	require.NotNil(t, token)
	assert.Equal(t, string(StatusExpired), token.Status)
	assert.Equal(t, "401 unauthorized", token.StatusReason)
}

func TestManager_MarkDisabled(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 80}})

	mgr.MarkDisabled(1, "manual disable")

	token := mgr.GetToken(1)
	require.NotNil(t, token)
	assert.Equal(t, string(StatusDisabled), token.Status)
	assert.Equal(t, "manual disable", token.StatusReason)
}
