package token

import (
	"testing"

	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenPool_NameGetAll(t *testing.T) {
	pool := NewTokenPool("extra-pool")
	assert.Equal(t, "extra-pool", pool.Name())
	assert.Nil(t, pool.Get(1))

	tok := &store.Token{ID: 1, Token: "t1", Status: string(StatusActive), Quotas: store.IntMap{"auto": 5}}
	pool.Add(tok)

	require.NotNil(t, pool.Get(1))
	assert.Same(t, tok, pool.Get(1))

	all := pool.All()
	require.Len(t, all, 1)
	assert.Same(t, tok, all[0])
}

func TestTokenPool_SelectAnyExcluding(t *testing.T) {
	t.Run("empty pool returns nil", func(t *testing.T) {
		pool := NewTokenPool(PoolBasic)
		assert.Nil(t, pool.SelectAnyExcluding(AlgoHighQuotaFirst, nil))
	})

	t.Run("skips excluded ids", func(t *testing.T) {
		pool := NewTokenPool(PoolBasic)
		pool.Add(&store.Token{ID: 1, Token: "t1", Status: string(StatusActive), Quotas: store.IntMap{"auto": 5}})
		assert.Nil(t, pool.SelectAnyExcluding(AlgoHighQuotaFirst, map[uint]struct{}{1: {}}))
	})

	t.Run("skips non-active tokens", func(t *testing.T) {
		pool := NewTokenPool(PoolBasic)
		pool.Add(&store.Token{ID: 1, Token: "t1", Status: string(StatusExpired), Quotas: store.IntMap{"auto": 5}})
		assert.Nil(t, pool.SelectAnyExcluding(AlgoHighQuotaFirst, nil))
	})

	t.Run("selects active token ignoring mode quota", func(t *testing.T) {
		pool := NewTokenPool(PoolBasic)
		pool.Add(&store.Token{ID: 1, Token: "t1", Status: string(StatusActive), Quotas: store.IntMap{"auto": 0}})
		selected := pool.SelectAnyExcluding(AlgoHighQuotaFirst, nil)
		require.NotNil(t, selected)
		assert.Equal(t, uint(1), selected.ID)
	})
}

func TestPool_SelectByAlgorithm_EmptyCandidates(t *testing.T) {
	pool := NewTokenPool(PoolBasic)
	for _, algo := range []string{AlgoHighQuotaFirst, AlgoRandom, AlgoRoundRobin, "unknown"} {
		assert.Nil(t, pool.selectByAlgorithm(algo, "auto", nil), "algo %s must handle empty candidates", algo)
	}
	assert.Nil(t, selectHighQuotaFirst(nil, "auto"))
}
