package token

import (
	"testing"

	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPool_Select_HighQuotaFirst(t *testing.T) {
	pool := NewTokenPool(PoolBasic)

	// Add tokens with different quotas
	pool.Add(&store.Token{ID: 1, Token: "t1", Status: string(StatusActive), ChatQuota: 50})
	pool.Add(&store.Token{ID: 2, Token: "t2", Status: string(StatusActive), ChatQuota: 80})
	pool.Add(&store.Token{ID: 3, Token: "t3", Status: string(StatusActive), ChatQuota: 30})

	selected := pool.Select(AlgoHighQuotaFirst, CategoryChat)
	require.NotNil(t, selected)
	assert.Equal(t, uint(2), selected.ID, "should select token with highest quota")
}

func TestPool_Select_Random(t *testing.T) {
	pool := NewTokenPool(PoolBasic)

	// Add tokens with same quota
	pool.Add(&store.Token{ID: 1, Token: "t1", Status: string(StatusActive), ChatQuota: 50})
	pool.Add(&store.Token{ID: 2, Token: "t2", Status: string(StatusActive), ChatQuota: 50})
	pool.Add(&store.Token{ID: 3, Token: "t3", Status: string(StatusActive), ChatQuota: 50})

	// Run multiple selections and check we don't always get the same one
	selections := make(map[uint]int)
	for i := 0; i < 100; i++ {
		selected := pool.Select(AlgoRandom, CategoryChat)
		require.NotNil(t, selected)
		selections[selected.ID]++
	}

	// With 100 iterations and 3 equal tokens, we should see at least 2 different tokens
	assert.GreaterOrEqual(t, len(selections), 2, "should randomly select among tokens")
}

func TestPool_Select_RoundRobin(t *testing.T) {
	pool := NewTokenPool(PoolBasic)

	pool.Add(&store.Token{ID: 1, Token: "t1", Status: string(StatusActive), ChatQuota: 50})
	pool.Add(&store.Token{ID: 2, Token: "t2", Status: string(StatusActive), ChatQuota: 50})
	pool.Add(&store.Token{ID: 3, Token: "t3", Status: string(StatusActive), ChatQuota: 50})

	// Sequential calls should cycle through all 3 tokens in ID order
	seen := make(map[uint]bool)
	for i := 0; i < 3; i++ {
		selected := pool.Select(AlgoRoundRobin, CategoryChat)
		require.NotNil(t, selected)
		seen[selected.ID] = true
	}
	assert.Len(t, seen, 3, "round-robin should cycle through all 3 tokens")

	// Next 3 calls should repeat the same cycle
	second := make([]uint, 0, 3)
	for i := 0; i < 3; i++ {
		selected := pool.Select(AlgoRoundRobin, CategoryChat)
		require.NotNil(t, selected)
		second = append(second, selected.ID)
	}
	// Verify deterministic ordering by checking all 3 are present
	seen2 := make(map[uint]bool)
	for _, id := range second {
		seen2[id] = true
	}
	assert.Len(t, seen2, 3, "second round-robin cycle should also hit all tokens")
}

func TestPool_Select_PriorityTiers(t *testing.T) {
	pool := NewTokenPool(PoolBasic)

	// pri=3 tokens and pri=1 tokens
	pool.Add(&store.Token{ID: 1, Token: "t1", Status: string(StatusActive), ChatQuota: 50, Priority: 3})
	pool.Add(&store.Token{ID: 2, Token: "t2", Status: string(StatusActive), ChatQuota: 80, Priority: 3})
	pool.Add(&store.Token{ID: 3, Token: "t3", Status: string(StatusActive), ChatQuota: 100, Priority: 1})

	// Should always select from pri=3 first (highest quota in that tier = ID 2)
	selected := pool.Select(AlgoHighQuotaFirst, CategoryChat)
	require.NotNil(t, selected)
	assert.Equal(t, uint(2), selected.ID, "should select highest-quota from highest priority tier")
}

func TestPool_Select_PriorityTiers_FallThrough(t *testing.T) {
	pool := NewTokenPool(PoolBasic)

	// pri=3 has only cooling tokens, pri=0 has active
	pool.Add(&store.Token{ID: 1, Token: "t1", Status: string(StatusCooling), ChatQuota: 100, Priority: 3})
	pool.Add(&store.Token{ID: 2, Token: "t2", Status: string(StatusActive), ChatQuota: 50, Priority: 0})

	selected := pool.Select(AlgoHighQuotaFirst, CategoryChat)
	require.NotNil(t, selected)
	assert.Equal(t, uint(2), selected.ID, "should fall through to pri=0 when pri=3 has no active tokens")
}

func TestPool_Select_EmptyPool(t *testing.T) {
	pool := NewTokenPool(PoolBasic)

	for _, algo := range []string{AlgoHighQuotaFirst, AlgoRandom, AlgoRoundRobin} {
		selected := pool.Select(algo, CategoryChat)
		assert.Nil(t, selected, "should return nil for algorithm %s on empty pool", algo)
	}
}

func TestPool_Select_IgnoresNonActiveTokens(t *testing.T) {
	pool := NewTokenPool(PoolBasic)

	pool.Add(&store.Token{ID: 1, Token: "t1", Status: string(StatusCooling), ChatQuota: 100})
	pool.Add(&store.Token{ID: 2, Token: "t2", Status: string(StatusDisabled), ChatQuota: 100})
	pool.Add(&store.Token{ID: 3, Token: "t3", Status: string(StatusActive), ChatQuota: 50})

	selected := pool.Select(AlgoHighQuotaFirst, CategoryChat)
	require.NotNil(t, selected)
	assert.Equal(t, uint(3), selected.ID, "should only select active tokens")
}

func TestValidAlgorithm(t *testing.T) {
	assert.True(t, ValidAlgorithm("high_quota_first"))
	assert.True(t, ValidAlgorithm("random"))
	assert.True(t, ValidAlgorithm("round_robin"))
	assert.False(t, ValidAlgorithm("invalid"))
	assert.False(t, ValidAlgorithm(""))
}

func TestPool_Count(t *testing.T) {
	pool := NewTokenPool(PoolBasic)

	pool.Add(&store.Token{ID: 1, Token: "t1", Status: string(StatusActive), ChatQuota: 100})
	pool.Add(&store.Token{ID: 2, Token: "t2", Status: string(StatusActive), ChatQuota: 80})
	pool.Add(&store.Token{ID: 3, Token: "t3", Status: string(StatusCooling), ChatQuota: 60})
	pool.Add(&store.Token{ID: 4, Token: "t4", Status: string(StatusDisabled), ChatQuota: 40})

	active, cooling, disabled, _ := pool.Count()
	assert.Equal(t, 2, active, "should count 2 active tokens")
	assert.Equal(t, 1, cooling, "should count 1 cooling token")
	assert.Equal(t, 1, disabled, "should count 1 disabled token")
}

func TestPool_AddAndRemove(t *testing.T) {
	pool := NewTokenPool(PoolBasic)

	token := &store.Token{ID: 1, Token: "t1", Status: string(StatusActive), ChatQuota: 100}
	pool.Add(token)

	active, _, _, _ := pool.Count()
	assert.Equal(t, 1, active)

	pool.Remove(1)
	active, _, _, _ = pool.Count()
	assert.Equal(t, 0, active)
}
