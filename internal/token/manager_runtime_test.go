package token

import (
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_SetAndClearResumeAt(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolSuper, Status: string(StatusActive)})

	resumeAt := int(time.Now().Add(time.Hour).Unix())
	mgr.SetResumeAt(1, "auto", resumeAt)
	assert.Equal(t, resumeAt, mgr.GetToken(1).ResumeAts["auto"])

	mgr.ClearResumeAt(1, "auto")
	_, exists := mgr.GetToken(1).ResumeAts["auto"]
	assert.False(t, exists)
}

func TestManager_PickIgnoresResumeAt(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	future := int(time.Now().Add(time.Hour).Unix())
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolSuper, Status: string(StatusActive),
		Quotas: store.IntMap{"auto": 50}, ResumeAts: store.IntMap{"auto": future}})

	tok, err := mgr.Pick(PoolSuper, "auto")
	require.NoError(t, err)
	assert.Equal(t, uint(1), tok.ID, "resume_at should not affect token selection")
}

func TestManager_ScanExhaustedModes(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)
	now := time.Now()

	mgr.AddToken(&store.Token{ID: 1, Token: "s1", Pool: PoolSuper, Status: string(StatusActive),
		Quotas: store.IntMap{"auto": 0, "fast": 1}, ResumeAts: store.IntMap{"auto": int(now.Unix())}})
	mgr.AddToken(&store.Token{ID: 2, Token: "d1", Pool: PoolSuper, Status: string(StatusDisabled),
		Quotas: store.IntMap{"auto": 0}})

	targets := mgr.ScanExhaustedModes(testModeSpecs(), now)
	require.Len(t, targets, 1)
	assert.Equal(t, uint(1), targets[0].TokenID)
	assert.Equal(t, "s1", targets[0].AuthToken)
	assert.Equal(t, PoolSuper, targets[0].Pool)
	assert.Equal(t, "auto", targets[0].Mode)
}

func TestManager_ScanExhaustedModesSkipsUnsupportedAndWaiting(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)
	now := time.Now()

	mgr.AddToken(&store.Token{ID: 1, Token: "b1", Pool: PoolBasic, Status: string(StatusActive),
		Quotas: store.IntMap{"auto": 0, "fast": 0}})
	mgr.AddToken(&store.Token{ID: 2, Token: "s1", Pool: PoolSuper, Status: string(StatusActive),
		Quotas: store.IntMap{"auto": 0, "fast": 1}, ResumeAts: store.IntMap{"auto": int(now.Add(time.Hour).Unix())}})

	targets := mgr.ScanExhaustedModes(testModeSpecs(), now)
	require.Len(t, targets, 1)
	assert.Equal(t, uint(1), targets[0].TokenID)
	assert.Equal(t, "fast", targets[0].Mode)
}

func TestManager_GetActiveTokenInfo(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	mgr.AddToken(&store.Token{ID: 1, Token: "active-token", Pool: PoolSuper, Status: string(StatusActive)})
	mgr.AddToken(&store.Token{ID: 2, Token: "disabled-token", Pool: PoolSuper, Status: string(StatusDisabled)})

	authToken, pool, ok := mgr.GetActiveTokenInfo(1)
	require.True(t, ok)
	assert.Equal(t, "active-token", authToken)
	assert.Equal(t, PoolSuper, pool)

	_, _, ok = mgr.GetActiveTokenInfo(2)
	assert.False(t, ok)
}

func TestManager_AddTokenCopiesInputMaps(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	input := &store.Token{
		ID:          1,
		Token:       "t1",
		Pool:        PoolSuper,
		Status:      string(StatusActive),
		Quotas:      store.IntMap{"auto": 10},
		LimitQuotas: store.IntMap{"auto": 50},
		ResumeAts:   store.IntMap{"auto": 123},
	}
	mgr.AddToken(input)

	input.Quotas["auto"] = 0
	input.LimitQuotas["auto"] = 0
	input.ResumeAts["auto"] = 0

	got := mgr.GetToken(1)
	require.NotNil(t, got)
	assert.Equal(t, 10, got.Quotas["auto"])
	assert.Equal(t, 50, got.LimitQuotas["auto"])
	assert.Equal(t, 123, got.ResumeAts["auto"])
}

func TestManager_ReturnedTokensAreSnapshots(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)
	mgr.AddToken(&store.Token{
		ID:          1,
		Token:       "t1",
		Pool:        PoolSuper,
		Status:      string(StatusActive),
		Quotas:      store.IntMap{"auto": 10},
		LimitQuotas: store.IntMap{"auto": 50},
	})

	got := mgr.GetToken(1)
	got.Quotas["auto"] = 0
	got.LimitQuotas["auto"] = 0

	picked, err := mgr.Pick(PoolSuper, "auto")
	require.NoError(t, err)
	assert.Equal(t, 9, picked.Quotas["auto"])
	picked.Quotas["auto"] = 0

	current := mgr.GetToken(1)
	assert.Equal(t, 9, current.Quotas["auto"])
	assert.Equal(t, 50, current.LimitQuotas["auto"])
}

func TestManager_PickInflightLimit(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3, MaxInflight: 1, SelectionAlgorithm: AlgoHighQuotaFirst}
	mgr := NewTokenManager(cfg)
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 100}})
	mgr.AddToken(&store.Token{ID: 2, Token: "t2", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 50}})

	tok1, err := mgr.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	assert.Equal(t, uint(1), tok1.ID)
	assert.Equal(t, 1, mgr.GetInflight(1))

	tok2, err := mgr.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	assert.Equal(t, uint(2), tok2.ID)
	assert.Equal(t, 1, mgr.GetInflight(2))

	_, err = mgr.Pick(PoolBasic, "auto")
	assert.ErrorIs(t, err, ErrNoTokenAvailable)
}

func TestManager_PickIncrementsInflight(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3, MaxInflight: 8}
	mgr := NewTokenManager(cfg)
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 100}})

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
	mgr.MarkSuccess(1)
	assert.Equal(t, 0, mgr.GetInflight(1))
}

func TestManager_MarkFailedReleasesInflight(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3, MaxInflight: 8}
	mgr := NewTokenManager(cfg)
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 100}})

	_, _ = mgr.Pick(PoolBasic, "auto")
	mgr.MarkFailed(1, "test error")
	assert.Equal(t, 0, mgr.GetInflight(1))
}

func TestManager_ReleaseInflightNoUnderflow(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive)})

	mgr.ReleaseInflightOnly(1)
	assert.Equal(t, 0, mgr.GetInflight(1))

	mgr.AddToken(&store.Token{ID: 2, Token: "t2", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 10}})
	_, _ = mgr.Pick(PoolBasic, "auto")
	mgr.ReleaseInflightOnly(2)
	mgr.ReleaseInflightOnly(2)
	assert.Equal(t, 0, mgr.GetInflight(2))
}

func TestManager_PickRecentUsePenalty(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3, RecentUsePenaltySec: 60, SelectionAlgorithm: AlgoHighQuotaFirst}
	mgr := NewTokenManager(cfg)
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 100}})
	mgr.AddToken(&store.Token{ID: 2, Token: "t2", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 5}})

	first, err := mgr.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	assert.Equal(t, uint(1), first.ID)

	second, err := mgr.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	assert.Equal(t, uint(2), second.ID)
}

func TestManager_PickRecentUsePenaltyFallback(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3, RecentUsePenaltySec: 60, SelectionAlgorithm: AlgoHighQuotaFirst}
	mgr := NewTokenManager(cfg)
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 100}})

	tok1, err := mgr.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	assert.Equal(t, uint(1), tok1.ID)

	tok2, err := mgr.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	assert.Equal(t, uint(1), tok2.ID)
}

func TestManager_PickRecentUsePenaltyDisabled(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3, RecentUsePenaltySec: 0, SelectionAlgorithm: AlgoHighQuotaFirst}
	mgr := NewTokenManager(cfg)
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 100}})
	mgr.AddToken(&store.Token{ID: 2, Token: "t2", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 50}})

	tok1, err := mgr.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	assert.Equal(t, uint(1), tok1.ID)

	tok2, err := mgr.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	assert.Equal(t, uint(1), tok2.ID)
}
