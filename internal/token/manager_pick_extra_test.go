package token

import (
	"testing"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_PickAnyExcluding(t *testing.T) {
	t.Run("unknown pool returns no token", func(t *testing.T) {
		mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
		token, err := mgr.PickAnyExcluding(PoolBasic, nil)
		assert.ErrorIs(t, err, ErrNoTokenAvailable)
		assert.Nil(t, token)
	})

	t.Run("selects active token ignoring quota", func(t *testing.T) {
		mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
		mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 0}})

		token, err := mgr.PickAnyExcluding(PoolBasic, nil)
		require.NoError(t, err)
		require.NotNil(t, token)
		assert.Equal(t, uint(1), token.ID)
		assert.Equal(t, 0, token.Quotas["auto"], "PickAnyExcluding must not pre-deduct quota")
	})

	t.Run("skips tokens at max inflight", func(t *testing.T) {
		mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3, MaxInflight: 1})
		mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 10}})
		mgr.AddToken(&store.Token{ID: 2, Token: "t2", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 10}})

		first, err := mgr.PickAnyExcluding(PoolBasic, nil)
		require.NoError(t, err)
		second, err := mgr.PickAnyExcluding(PoolBasic, nil)
		require.NoError(t, err)
		assert.NotEqual(t, first.ID, second.ID)

		third, err := mgr.PickAnyExcluding(PoolBasic, nil)
		assert.ErrorIs(t, err, ErrNoTokenAvailable)
		assert.Nil(t, third)
	})

	t.Run("falls back to penalized token when all penalized", func(t *testing.T) {
		mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3, RecentUsePenaltySec: 60})
		mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 10}})

		first, err := mgr.PickAnyExcluding(PoolBasic, nil)
		require.NoError(t, err)
		second, err := mgr.PickAnyExcluding(PoolBasic, nil)
		require.NoError(t, err)
		assert.Equal(t, first.ID, second.ID, "penalty must fall back to the same token when no alternative exists")
	})

	t.Run("skips non-active tokens", func(t *testing.T) {
		mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
		mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusDisabled), Quotas: store.IntMap{"auto": 10}})

		token, err := mgr.PickAnyExcluding(PoolBasic, nil)
		assert.ErrorIs(t, err, ErrNoTokenAvailable)
		assert.Nil(t, token)
	})

	t.Run("honors explicit excludes", func(t *testing.T) {
		mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
		mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 10}})

		token, err := mgr.PickAnyExcluding(PoolBasic, map[uint]struct{}{1: {}})
		assert.ErrorIs(t, err, ErrNoTokenAvailable)
		assert.Nil(t, token)
	})
}

func TestManager_PickExcluding_CombinesExplicitExcludeWithRecentUsePenalty(t *testing.T) {
	mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3, RecentUsePenaltySec: 60, SelectionAlgorithm: AlgoHighQuotaFirst})
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 100}})
	mgr.AddToken(&store.Token{ID: 2, Token: "t2", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 90}})
	mgr.AddToken(&store.Token{ID: 3, Token: "t3", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 80}})

	first, err := mgr.Pick(PoolBasic, "auto") // picks t1 and marks it recently used
	require.NoError(t, err)
	require.Equal(t, uint(1), first.ID)

	second, err := mgr.PickExcluding(PoolBasic, "auto", map[uint]struct{}{3: {}})
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, uint(2), second.ID, "explicit exclude and recent-use penalty must combine")
}
