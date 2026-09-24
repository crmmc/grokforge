package token

import (
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_RefundQuota_Branches(t *testing.T) {
	t.Run("unknown token is noop", func(t *testing.T) {
		mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
		mgr.RefundQuota(999, "auto")
		assert.Empty(t, mgr.GetDirtyTokens())
	})

	t.Run("initializes nil quota map", func(t *testing.T) {
		mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
		mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive)})

		mgr.RefundQuota(1, "auto")

		token := mgr.GetToken(1)
		require.NotNil(t, token)
		assert.Equal(t, 1, token.Quotas["auto"], "refund with nil map and no limit starts at 1")
	})

	t.Run("caps refund at limit", func(t *testing.T) {
		mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
		mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive),
			Quotas: store.IntMap{"auto": 10}, LimitQuotas: store.IntMap{"auto": 10}})

		mgr.RefundQuota(1, "auto")

		token := mgr.GetToken(1)
		require.NotNil(t, token)
		assert.Equal(t, 10, token.Quotas["auto"], "refund must not exceed the mode limit")
	})
}

func TestManager_ClearModeQuota_Branches(t *testing.T) {
	t.Run("unknown token is noop", func(t *testing.T) {
		mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
		mgr.ClearModeQuota(999, "auto")
		assert.Empty(t, mgr.GetDirtyTokens())
	})

	t.Run("initializes nil quota map", func(t *testing.T) {
		mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
		mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive)})

		mgr.ClearModeQuota(1, "auto")

		token := mgr.GetToken(1)
		require.NotNil(t, token)
		assert.Equal(t, 0, token.Quotas["auto"])
	})
}

func TestManager_SetResumeAt_UnknownTokenIsNoop(t *testing.T) {
	mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	mgr.SetResumeAt(999, "auto", 123)
	assert.Empty(t, mgr.GetDirtyTokens())
}

func TestManager_SetResumeAtIfDue(t *testing.T) {
	now := int(time.Now().Unix())
	future := int(time.Now().Add(time.Hour).Unix())

	t.Run("unknown token returns false", func(t *testing.T) {
		mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
		assert.False(t, mgr.SetResumeAtIfDue(999, "auto", future, now))
	})

	t.Run("non-exhausted mode returns false", func(t *testing.T) {
		mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
		mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 5}})
		assert.False(t, mgr.SetResumeAtIfDue(1, "auto", future, now))
	})

	t.Run("future resume returns false", func(t *testing.T) {
		mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
		mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive),
			Quotas: store.IntMap{"auto": 0}, ResumeAts: store.IntMap{"auto": future}})
		assert.False(t, mgr.SetResumeAtIfDue(1, "auto", future+1, now))
	})

	t.Run("sets when exhausted and due", func(t *testing.T) {
		mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
		mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 0}})
		assert.True(t, mgr.SetResumeAtIfDue(1, "auto", future, now))
		assert.Equal(t, future, mgr.GetResumeAt(1, "auto"))
	})
}

func TestManager_UpdateModeQuota_Branches(t *testing.T) {
	t.Run("unknown token is noop", func(t *testing.T) {
		mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
		mgr.UpdateModeQuota(999, "auto", 10, 20)
		assert.Empty(t, mgr.GetDirtyTokens())
	})

	t.Run("clamps remaining above limit", func(t *testing.T) {
		mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
		mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive)})

		mgr.UpdateModeQuota(1, "auto", 100, 50)

		token := mgr.GetToken(1)
		require.NotNil(t, token)
		assert.Equal(t, 50, token.Quotas["auto"])
		assert.Equal(t, 50, token.LimitQuotas["auto"])
	})
}

func TestManager_GetModeQuota_Branches(t *testing.T) {
	mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 7}})
	mgr.AddToken(&store.Token{ID: 2, Token: "t2", Pool: PoolBasic, Status: string(StatusActive)})

	assert.Equal(t, 7, mgr.GetModeQuota(1, "auto"))
	assert.Equal(t, 0, mgr.GetModeQuota(2, "auto"), "nil quota map reads as zero")
	assert.Equal(t, 0, mgr.GetModeQuota(999, "auto"), "unknown token reads as zero")
}
