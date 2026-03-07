package token

import (
	"context"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCooldownTicker_RestoresExpiredCooling(t *testing.T) {
	mgr := setupTestManager()

	// Add a token that should have cooled down already
	pastCoolUntil := time.Now().Add(-1 * time.Minute)
	mgr.AddToken(&store.Token{
		ID:        1,
		Token:     "t1",
		Pool:      PoolBasic,
		Status:    string(StatusCooling),
		ChatQuota: 80,
		CoolUntil: &pastCoolUntil,
	})

	// Create ticker with short interval for testing
	ticker := NewCooldownTicker(mgr, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go ticker.Start(ctx)

	// Wait for at least one tick
	time.Sleep(100 * time.Millisecond)

	token := mgr.GetToken(1)
	require.NotNil(t, token)
	assert.Equal(t, string(StatusActive), token.Status, "expired cooling token should be restored to active")
	assert.Nil(t, token.CoolUntil, "CoolUntil should be cleared")
}

func TestCooldownTicker_IgnoresNotExpiredCooling(t *testing.T) {
	mgr := setupTestManager()

	// Add a token that is still cooling
	futureCoolUntil := time.Now().Add(10 * time.Minute)
	mgr.AddToken(&store.Token{
		ID:        1,
		Token:     "t1",
		Pool:      PoolBasic,
		Status:    string(StatusCooling),
		ChatQuota: 80,
		CoolUntil: &futureCoolUntil,
	})

	ticker := NewCooldownTicker(mgr, 50*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	go ticker.Start(ctx)

	time.Sleep(100 * time.Millisecond)

	token := mgr.GetToken(1)
	require.NotNil(t, token)
	assert.Equal(t, string(StatusCooling), token.Status, "not-expired cooling token should remain cooling")
}

func TestCooldownTicker_StopsOnContextCancel(t *testing.T) {
	mgr := setupTestManager()
	ticker := NewCooldownTicker(mgr, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		ticker.Start(ctx)
		close(done)
	}()

	// Cancel immediately
	cancel()

	select {
	case <-done:
		// Success - ticker stopped
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ticker did not stop on context cancel")
	}
}

func TestCooldownTicker_CheckNow(t *testing.T) {
	mgr := setupTestManager()

	pastCoolUntil := time.Now().Add(-1 * time.Minute)
	mgr.AddToken(&store.Token{
		ID:        1,
		Token:     "t1",
		Pool:      PoolBasic,
		Status:    string(StatusCooling),
		ChatQuota: 80,
		CoolUntil: &pastCoolUntil,
	})

	ticker := NewCooldownTicker(mgr, time.Hour) // Long interval

	// Manually trigger check
	restored := ticker.CheckNow()

	assert.Equal(t, 1, restored, "should restore 1 token")

	token := mgr.GetToken(1)
	require.NotNil(t, token)
	assert.Equal(t, string(StatusActive), token.Status)
}

func TestCooldownTicker_DoesNotRestoreZeroQuota(t *testing.T) {
	mgr := setupTestManager()

	pastCoolUntil := time.Now().Add(-1 * time.Minute)
	mgr.AddToken(&store.Token{
		ID:        1,
		Token:     "t1",
		Pool:      PoolBasic,
		Status:    string(StatusCooling),
		ChatQuota: 0,
		CoolUntil: &pastCoolUntil,
	})

	ticker := NewCooldownTicker(mgr, time.Hour)

	restored := ticker.CheckNow()
	assert.Equal(t, 0, restored, "should not restore zero-quota token")

	token := mgr.GetToken(1)
	require.NotNil(t, token)
	assert.Equal(t, string(StatusCooling), token.Status)
}

func setupTestManager() *TokenManager {
	return NewTokenManager(&config.TokenConfig{FailThreshold: 3})
}
