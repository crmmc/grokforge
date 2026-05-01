package token

import (
	"testing"
	"time"

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

// ── Cooling tests ──────────────────────────────────────────────

func TestManager_ClearModeQuotaAndCool(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3, MaxInflight: 8, CoolDurationSuperSec: 7200}
	mgr := NewTokenManager(cfg)
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolSuper, Status: string(StatusActive), Quotas: store.IntMap{"auto": 50}})

	// Pick to increment inflight
	tok, err := mgr.Pick(PoolSuper, "auto")
	require.NoError(t, err)
	assert.Equal(t, 1, mgr.GetInflight(tok.ID))

	mgr.ClearModeQuotaAndCool(1, "auto")

	token := mgr.GetToken(1)
	assert.Equal(t, 0, token.Quotas["auto"], "quota should be cleared")
	assert.Greater(t, token.CoolUntils["auto"], 0, "cool_until should be set")
	assert.Equal(t, 0, mgr.GetInflight(1), "inflight should be released")
}

func TestManager_CoolMonotonicIncrease(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3, CoolDurationSuperSec: 100}
	mgr := NewTokenManager(cfg)
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolSuper, Status: string(StatusActive), Quotas: store.IntMap{"auto": 50}})

	mgr.ClearModeQuotaAndCool(1, "auto")
	first := mgr.GetToken(1).CoolUntils["auto"]

	// Set a much longer cooling manually
	mgr.GetToken(1).CoolUntils["auto"] = first + 99999

	// ClearModeQuotaAndCool again — should NOT shorten
	mgr.ClearModeQuotaAndCool(1, "auto")
	assert.Equal(t, first+99999, mgr.GetToken(1).CoolUntils["auto"], "cooling should not be shortened")
}

func TestManager_CoolPerPoolDuration(t *testing.T) {
	cfg := &config.TokenConfig{
		FailThreshold:        3,
		CoolDurationBasicSec: 1000,
		CoolDurationSuperSec: 2000,
		CoolDurationHeavySec: 3000,
	}
	mgr := NewTokenManager(cfg)
	mgr.AddToken(&store.Token{ID: 1, Token: "b1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"fast": 50}})
	mgr.AddToken(&store.Token{ID: 2, Token: "s1", Pool: PoolSuper, Status: string(StatusActive), Quotas: store.IntMap{"auto": 50}})
	mgr.AddToken(&store.Token{ID: 3, Token: "h1", Pool: PoolHeavy, Status: string(StatusActive), Quotas: store.IntMap{"heavy": 50}})

	now := int(time.Now().Unix())
	mgr.ClearModeQuotaAndCool(1, "fast")
	mgr.ClearModeQuotaAndCool(2, "auto")
	mgr.ClearModeQuotaAndCool(3, "heavy")

	// Each pool should have different cooling duration (within 5s tolerance)
	assert.InDelta(t, now+1000, mgr.GetToken(1).CoolUntils["fast"], 5)
	assert.InDelta(t, now+2000, mgr.GetToken(2).CoolUntils["auto"], 5)
	assert.InDelta(t, now+3000, mgr.GetToken(3).CoolUntils["heavy"], 5)
}

func TestManager_PickSkipsCoolingToken(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	future := int(time.Now().Add(1 * time.Hour).Unix())
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolSuper, Status: string(StatusActive),
		Quotas: store.IntMap{"auto": 50}, CoolUntils: store.IntMap{"auto": future}})
	mgr.AddToken(&store.Token{ID: 2, Token: "t2", Pool: PoolSuper, Status: string(StatusActive),
		Quotas: store.IntMap{"auto": 50}})

	tok, err := mgr.Pick(PoolSuper, "auto")
	require.NoError(t, err)
	assert.Equal(t, uint(2), tok.ID, "should skip cooling token and pick token 2")
}

func TestManager_PickCoolingExpired(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	past := int(time.Now().Add(-1 * time.Hour).Unix())
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolSuper, Status: string(StatusActive),
		Quotas: store.IntMap{"auto": 50}, CoolUntils: store.IntMap{"auto": past}})

	tok, err := mgr.Pick(PoolSuper, "auto")
	require.NoError(t, err)
	assert.Equal(t, uint(1), tok.ID, "expired cooling token should be selectable")
}

// ── Inflight tests ─────────────────────────────────────────────

func TestManager_PickInflightLimit(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3, MaxInflight: 1, SelectionAlgorithm: AlgoHighQuotaFirst}
	mgr := NewTokenManager(cfg)
	// Token 1 has higher quota → will be picked first by high_quota_first.
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 100}})
	mgr.AddToken(&store.Token{ID: 2, Token: "t2", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 50}})

	// First pick: token 1 (highest quota), inflight becomes 1 (= max).
	tok1, err := mgr.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	assert.Equal(t, uint(1), tok1.ID, "first pick should select highest-quota token")
	assert.Equal(t, 1, mgr.GetInflight(1))

	// Second pick: token 1 is at max inflight, must select token 2.
	tok2, err := mgr.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	assert.Equal(t, uint(2), tok2.ID, "should skip full-inflight token and pick token 2")
	assert.Equal(t, 1, mgr.GetInflight(2))

	// Both tokens at max inflight, next pick should fail.
	_, err = mgr.Pick(PoolBasic, "auto")
	assert.ErrorIs(t, err, ErrNoTokenAvailable, "should fail when all tokens at max inflight")
}

func TestManager_PickIncrementsInflight(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3, MaxInflight: 8}
	mgr := NewTokenManager(cfg)
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 100}})

	assert.Equal(t, 0, mgr.GetInflight(1))

	_, err := mgr.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	assert.Equal(t, 1, mgr.GetInflight(1))

	_, err = mgr.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	assert.Equal(t, 2, mgr.GetInflight(1))
}

func TestManager_MarkSuccessReleasesInflight(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3, MaxInflight: 8}
	mgr := NewTokenManager(cfg)
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 100}})

	_, _ = mgr.Pick(PoolBasic, "auto")
	assert.Equal(t, 1, mgr.GetInflight(1))

	mgr.MarkSuccess(1)
	assert.Equal(t, 0, mgr.GetInflight(1))
}

func TestManager_MarkFailedReleasesInflight(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3, MaxInflight: 8}
	mgr := NewTokenManager(cfg)
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 100}})

	_, _ = mgr.Pick(PoolBasic, "auto")
	assert.Equal(t, 1, mgr.GetInflight(1))

	mgr.MarkFailed(1, "test error")
	assert.Equal(t, 0, mgr.GetInflight(1))
}

func TestManager_ReleaseInflightNoUnderflow(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive)})

	// Release without prior Pick — should not go negative
	mgr.ReleaseInflightOnly(1)
	assert.Equal(t, 0, mgr.GetInflight(1))

	// Double release after single Pick
	mgr.AddToken(&store.Token{ID: 2, Token: "t2", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 10}})
	_, _ = mgr.Pick(PoolBasic, "auto")
	mgr.ReleaseInflightOnly(2)
	mgr.ReleaseInflightOnly(2)
	assert.Equal(t, 0, mgr.GetInflight(2))
}

func TestManager_PickRecentUsePenalty(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3, RecentUsePenaltySec: 60, SelectionAlgorithm: AlgoHighQuotaFirst}
	mgr := NewTokenManager(cfg)

	// Token 1 has much higher quota. Without penalty, high_quota_first would always pick it.
	// With penalty, second pick should select token 2 despite lower quota.
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 100}})
	mgr.AddToken(&store.Token{ID: 2, Token: "t2", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 5}})

	first, err := mgr.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	assert.Equal(t, uint(1), first.ID, "first pick should select highest quota token")

	// Second pick: recent-use penalty should exclude token 1, forcing token 2.
	second, err := mgr.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	assert.Equal(t, uint(2), second.ID, "second pick should select token 2 due to recent-use penalty on token 1")
}

func TestManager_PickRecentUsePenaltyFallback(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3, RecentUsePenaltySec: 60, SelectionAlgorithm: AlgoHighQuotaFirst}
	mgr := NewTokenManager(cfg)

	// Only one token: penalty should not block selection (fallback).
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 100}})

	tok1, err := mgr.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	assert.Equal(t, uint(1), tok1.ID)

	// Second pick: only token is penalized, but fallback allows selection.
	tok2, err := mgr.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	assert.Equal(t, uint(1), tok2.ID, "should still pick the only token via fallback")
}

func TestManager_PickRecentUsePenaltyDisabled(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3, RecentUsePenaltySec: 0, SelectionAlgorithm: AlgoHighQuotaFirst}
	mgr := NewTokenManager(cfg)

	// With penalty disabled, highest quota always wins.
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 100}})
	mgr.AddToken(&store.Token{ID: 2, Token: "t2", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 50}})

	tok1, err := mgr.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	assert.Equal(t, uint(1), tok1.ID, "highest quota token should be picked")

	// Second pick: penalty is disabled, so token 1 still has the highest quota (99 after deduction).
	tok2, err := mgr.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	assert.Equal(t, uint(1), tok2.ID, "same token should be picked again when penalty is disabled")
}
