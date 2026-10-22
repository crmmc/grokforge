package token

import (
	"testing"

	"github.com/crmmc/grokforge/internal/modelconfig"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
)

func TestNormalizeTokenQuotas(t *testing.T) {
	modes := []modelconfig.ModeSpec{
		{ID: "auto", DefaultQuota: map[string]int{"basic": 0, "super": 50}},
		{ID: "fast", DefaultQuota: map[string]int{"basic": 30, "super": 140}},
	}

	t.Run("nil token is noop", func(t *testing.T) {
		res := normalizeTokenQuotas(nil, modes)
		assert.False(t, res.changed)
		assert.Empty(t, res.zeroModes)
	})

	t.Run("initializes nil maps from defaults", func(t *testing.T) {
		token := &store.Token{ID: 1, Pool: PoolSuper}
		res := normalizeTokenQuotas(token, modes)
		assert.True(t, res.changed)
		assert.Equal(t, 50, token.Quotas["auto"])
		assert.Equal(t, 50, token.LimitQuotas["auto"])
		assert.Equal(t, 140, token.Quotas["fast"])
		assert.Equal(t, 140, token.LimitQuotas["fast"])
		assert.Empty(t, res.zeroModes)
	})

	t.Run("removes unknown mode keys", func(t *testing.T) {
		token := &store.Token{
			ID:          1,
			Pool:        PoolSuper,
			Quotas:      store.IntMap{"auto": 10, "legacy": 3},
			LimitQuotas: store.IntMap{"auto": 50, "legacy": 3},
		}
		res := normalizeTokenQuotas(token, modes)
		assert.True(t, res.changed)
		_, hasLegacyQuota := token.Quotas["legacy"]
		_, hasLegacyLimit := token.LimitQuotas["legacy"]
		assert.False(t, hasLegacyQuota)
		assert.False(t, hasLegacyLimit)
		assert.Equal(t, 10, token.Quotas["auto"])
	})

	t.Run("skips modes without pool default", func(t *testing.T) {
		token := &store.Token{ID: 1, Pool: PoolBasic, Quotas: store.IntMap{}, LimitQuotas: store.IntMap{}}
		res := normalizeTokenQuotas(token, modes)
		assert.True(t, res.changed)
		_, hasAutoLimit := token.LimitQuotas["auto"]
		assert.False(t, hasAutoLimit, "auto has no basic default and must be skipped")
		assert.Equal(t, 30, token.Quotas["fast"])
	})

	t.Run("clamps quota above limit", func(t *testing.T) {
		token := &store.Token{
			ID:          1,
			Pool:        PoolBasic,
			Quotas:      store.IntMap{"fast": 500},
			LimitQuotas: store.IntMap{"fast": 100},
		}
		res := normalizeTokenQuotas(token, modes)
		assert.True(t, res.changed)
		assert.Equal(t, 100, token.Quotas["fast"])
	})

	t.Run("reports zero modes sorted", func(t *testing.T) {
		token := &store.Token{
			ID:     1,
			Pool:   PoolSuper,
			Quotas: store.IntMap{"auto": 0, "fast": 0},
		}
		res := normalizeTokenQuotas(token, modes)
		assert.True(t, res.changed)
		assert.Equal(t, []string{"auto", "fast"}, res.zeroModes)
	})
}
