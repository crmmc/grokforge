package token

import (
	"testing"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
)

func TestManager_GetPool(t *testing.T) {
	mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 10}})

	assert.NotNil(t, mgr.GetPool(PoolBasic))
	assert.Nil(t, mgr.GetPool("missing"))
}

func TestManager_GetTokenPool(t *testing.T) {
	mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 10}})

	assert.Equal(t, PoolBasic, mgr.GetTokenPool(1))
	assert.Empty(t, mgr.GetTokenPool(999))
}
