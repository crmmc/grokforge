package token

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
)

func TestScheduler_Start(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	m := NewTokenManager(cfg)

	// Mock server that returns quota
	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		resp := RateLimitsResponse{RemainingQueries: 50}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Add a cooling token with expired CoolUntil
	coolUntil := time.Now().Add(-1 * time.Minute)
	token := &store.Token{
		ID:        1,
		Token:     "test-token",
		Pool:      PoolBasic,
		Status:    string(StatusCooling),
		ChatQuota: 0,
		CoolUntil: &coolUntil,
	}
	m.AddToken(token)

	scheduler := NewScheduler(m, &config.TokenConfig{QuotaRecoveryMode: RecoveryModeUpstream}, server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)

	// Wait for at least one refresh cycle
	time.Sleep(100 * time.Millisecond)

	cancel()
	scheduler.Stop()

	if callCount.Load() < 1 {
		t.Errorf("expected at least 1 API call, got %d", callCount.Load())
	}

	// Token should be restored to active
	if token.Status != string(StatusActive) {
		t.Errorf("expected token status=active, got %s", token.Status)
	}
}

func TestScheduler_OnlyRefreshesExpiredCoolingTokens(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	m := NewTokenManager(cfg)

	var callCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		resp := RateLimitsResponse{RemainingQueries: 50}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Active token - should NOT be refreshed
	activeToken := &store.Token{
		ID:        1,
		Token:     "active-token",
		Pool:      PoolBasic,
		Status:    string(StatusActive),
		ChatQuota: 50,
	}
	m.AddToken(activeToken)

	// Cooling token with future CoolUntil - should NOT be refreshed
	futureCool := time.Now().Add(10 * time.Minute)
	futureCoolingToken := &store.Token{
		ID:        2,
		Token:     "future-cooling",
		Pool:      PoolBasic,
		Status:    string(StatusCooling),
		ChatQuota: 0,
		CoolUntil: &futureCool,
	}
	m.AddToken(futureCoolingToken)

	// Cooling token with expired CoolUntil - SHOULD be refreshed
	pastCool := time.Now().Add(-1 * time.Minute)
	expiredCoolingToken := &store.Token{
		ID:        3,
		Token:     "expired-cooling",
		Pool:      PoolBasic,
		Status:    string(StatusCooling),
		ChatQuota: 0,
		CoolUntil: &pastCool,
	}
	m.AddToken(expiredCoolingToken)

	scheduler := NewScheduler(m, &config.TokenConfig{QuotaRecoveryMode: RecoveryModeUpstream}, server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)

	time.Sleep(100 * time.Millisecond)

	cancel()
	scheduler.Stop()

	// Only the expired cooling token should trigger API call
	if callCount.Load() != 1 {
		t.Errorf("expected exactly 1 API call, got %d", callCount.Load())
	}
}

func TestScheduler_ConcurrencyLimit(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	m := NewTokenManager(cfg)

	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := concurrent.Add(1)
		// Track max concurrent
		for {
			old := maxConcurrent.Load()
			if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond) // Simulate slow API
		concurrent.Add(-1)
		resp := RateLimitsResponse{RemainingQueries: 50}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Add 10 expired cooling tokens
	for i := uint(1); i <= 10; i++ {
		coolUntil := time.Now().Add(-1 * time.Minute)
		token := &store.Token{
			ID:        i,
			Token:     "token-" + string(rune('0'+i)),
			Pool:      PoolBasic,
			Status:    string(StatusCooling),
			ChatQuota: 0,
			CoolUntil: &coolUntil,
		}
		m.AddToken(token)
	}

	scheduler := NewScheduler(m, &config.TokenConfig{QuotaRecoveryMode: RecoveryModeUpstream}, server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)

	time.Sleep(200 * time.Millisecond)

	cancel()
	scheduler.Stop()

	// Max concurrent should not exceed 5 (the semaphore limit)
	if maxConcurrent.Load() > 5 {
		t.Errorf("expected max concurrent <= 5, got %d", maxConcurrent.Load())
	}
}

func TestScheduler_Stop(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	m := NewTokenManager(cfg)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RateLimitsResponse{RemainingQueries: 50}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	scheduler := NewScheduler(m, &config.TokenConfig{QuotaRecoveryMode: RecoveryModeUpstream}, server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)

	time.Sleep(30 * time.Millisecond)

	// Stop should complete without blocking
	done := make(chan struct{})
	go func() {
		cancel()
		scheduler.Stop()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(1 * time.Second):
		t.Error("Stop() blocked for too long")
	}
}
