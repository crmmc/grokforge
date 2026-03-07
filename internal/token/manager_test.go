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

	mgr.AddToken(&store.Token{ID: 1, Token: "basic1", Pool: PoolBasic, Status: string(StatusActive), ChatQuota: 80})
	mgr.AddToken(&store.Token{ID: 2, Token: "super1", Pool: PoolSuper, Status: string(StatusActive), ChatQuota: 140})

	token, err := mgr.Pick(PoolBasic, CategoryChat)
	require.NoError(t, err)
	require.NotNil(t, token)
	assert.Equal(t, uint(1), token.ID)

	token, err = mgr.Pick(PoolSuper, CategoryChat)
	require.NoError(t, err)
	require.NotNil(t, token)
	assert.Equal(t, uint(2), token.ID)
}

func TestManager_Pick_NoCoolingFallback(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3, SelectionAlgorithm: AlgoHighQuotaFirst}
	mgr := NewTokenManager(cfg)

	coolUntil := time.Now().Add(5 * time.Minute)
	mgr.AddToken(&store.Token{ID: 1, Token: "cooling1", Pool: PoolBasic, Status: string(StatusCooling), ChatQuota: 80, CoolUntil: &coolUntil})

	token, err := mgr.Pick(PoolBasic, CategoryChat)
	assert.ErrorIs(t, err, ErrNoTokenAvailable, "should return ErrNoTokenAvailable when only cooling tokens")
	assert.Nil(t, token)
}

func TestManager_Pick_UsesAlgorithm(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3, SelectionAlgorithm: AlgoRoundRobin}
	mgr := NewTokenManager(cfg)

	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), ChatQuota: 50})
	mgr.AddToken(&store.Token{ID: 2, Token: "t2", Pool: PoolBasic, Status: string(StatusActive), ChatQuota: 50})
	mgr.AddToken(&store.Token{ID: 3, Token: "t3", Pool: PoolBasic, Status: string(StatusActive), ChatQuota: 50})

	// With round-robin, 3 calls should cycle through all 3 tokens
	seen := make(map[uint]bool)
	for i := 0; i < 3; i++ {
		token, err := mgr.Pick(PoolBasic, CategoryChat)
		require.NoError(t, err)
		require.NotNil(t, token)
		seen[token.ID] = true
	}
	assert.Len(t, seen, 3, "round-robin should have visited all 3 tokens")
}

func TestManager_PickExcluding_SkipsExcluded(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3, SelectionAlgorithm: AlgoHighQuotaFirst}
	mgr := NewTokenManager(cfg)

	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), ChatQuota: 100})
	mgr.AddToken(&store.Token{ID: 2, Token: "t2", Pool: PoolBasic, Status: string(StatusActive), ChatQuota: 90})

	token, err := mgr.PickExcluding(PoolBasic, CategoryChat, map[uint]struct{}{1: {}})
	require.NoError(t, err)
	require.NotNil(t, token)
	assert.Equal(t, uint(2), token.ID)
}

func TestManager_Pick_ReturnsErrorWhenNoTokens(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	token, err := mgr.Pick(PoolBasic, CategoryChat)
	assert.Error(t, err)
	assert.Nil(t, token)
}

func TestManager_MarkCooling(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), ChatQuota: 80})

	mgr.MarkCooling(1, 5*time.Minute, "rate limited")

	token := mgr.GetToken(1)
	require.NotNil(t, token)
	assert.Equal(t, string(StatusCooling), token.Status)
	assert.NotNil(t, token.CoolUntil)
	assert.True(t, token.CoolUntil.After(time.Now()))
}

func TestManager_MarkSuccess_RestoresActive(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	coolUntil := time.Now().Add(5 * time.Minute)
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusCooling), ChatQuota: 80, CoolUntil: &coolUntil, FailCount: 2})

	mgr.MarkSuccess(1)

	token := mgr.GetToken(1)
	require.NotNil(t, token)
	assert.Equal(t, string(StatusActive), token.Status)
	assert.Nil(t, token.CoolUntil)
	assert.Equal(t, 0, token.FailCount)
}

func TestManager_MarkFailed_IncrementsFailCount(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), ChatQuota: 80, FailCount: 0})

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

	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), ChatQuota: 80, FailCount: 2})

	mgr.MarkFailed(1, "test error") // 3rd failure

	token := mgr.GetToken(1)
	require.NotNil(t, token)
	assert.Equal(t, string(StatusDisabled), token.Status)
}

func TestManager_MarkFailed_ZeroThresholdNeverDisables(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 0}
	mgr := NewTokenManager(cfg)

	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), ChatQuota: 80, FailCount: 0})

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

	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), ChatQuota: 80})
	mgr.AddToken(&store.Token{ID: 2, Token: "t2", Pool: PoolBasic, Status: string(StatusActive), ChatQuota: 80})

	// Clear any dirty from Add
	_ = mgr.GetDirtyTokens()
	mgr.ClearDirty([]uint{1, 2})

	mgr.MarkCooling(1, 5*time.Minute, "rate limited")
	mgr.MarkFailed(2, "test error")

	dirty := mgr.GetDirtyTokens()
	assert.Len(t, dirty, 2, "should have 2 dirty tokens")

	// ClearDirty should remove from dirty set
	mgr.ClearDirty([]uint{1, 2})
	dirty = mgr.GetDirtyTokens()
	assert.Len(t, dirty, 0, "dirty set should be cleared after ClearDirty")
}

func TestRestoreToken_RestoresCoolingToken(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	mgr := NewTokenManager(cfg)

	coolUntil := time.Now().Add(5 * time.Minute)
	mgr.AddToken(&store.Token{
		ID: 1, Token: "cooling1", Pool: PoolBasic,
		Status: string(StatusCooling), CoolUntil: &coolUntil,
		ChatQuota: 0, ImageQuota: 0, VideoQuota: 0,
	})
	mgr.AddToken(&store.Token{
		ID: 2, Token: "active1", Pool: PoolBasic,
		Status:    string(StatusActive),
		ChatQuota: 5, ImageQuota: 3, VideoQuota: 1,
	})

	mgr.RestoreToken(1, 80, 10, 5)

	// Cooling token should be restored to active
	tok1 := mgr.GetToken(1)
	require.NotNil(t, tok1)
	assert.Equal(t, string(StatusActive), tok1.Status)
	assert.Nil(t, tok1.CoolUntil)
	assert.Equal(t, 80, tok1.ChatQuota)
	assert.Equal(t, 80, tok1.InitialChatQuota)
	assert.Equal(t, 10, tok1.ImageQuota)
	assert.Equal(t, 10, tok1.InitialImageQuota)
	assert.Equal(t, 5, tok1.VideoQuota)
	assert.Equal(t, 5, tok1.InitialVideoQuota)

	// Active token should be unchanged (RestoreToken only affects the specified ID)
	tok2 := mgr.GetToken(2)
	require.NotNil(t, tok2)
	assert.Equal(t, string(StatusActive), tok2.Status)
	assert.Equal(t, 5, tok2.ChatQuota)
}
