package token

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
)

func TestScheduler_StartRestoresExpiredCoolingTokens(t *testing.T) {
	cfg := &config.TokenConfig{
		FailThreshold:     3,
		DefaultChatQuota:  50,
		DefaultImageQuota: 20,
		DefaultVideoQuota: 10,
	}
	manager := NewTokenManager(cfg)

	coolUntil := time.Now().Add(-1 * time.Minute)
	token := &store.Token{
		ID:         1,
		Token:      "test-token",
		Pool:       PoolBasic,
		Status:     string(StatusCooling),
		ChatQuota:  0,
		ImageQuota: 0,
		VideoQuota: 0,
		CoolUntil:  &coolUntil,
	}
	manager.AddToken(token)

	scheduler := NewScheduler(manager, cfg, "https://example.com")
	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	cancel()
	scheduler.Stop()

	if token.Status != string(StatusActive) {
		t.Fatalf("expected token status=active, got %s", token.Status)
	}
	if token.ChatQuota != 50 || token.ImageQuota != 20 || token.VideoQuota != 10 {
		t.Fatalf("unexpected restored quotas: chat=%d image=%d video=%d", token.ChatQuota, token.ImageQuota, token.VideoQuota)
	}
}

func TestScheduler_OnlyRestoresExpiredCoolingTokens(t *testing.T) {
	cfg := &config.TokenConfig{
		FailThreshold:     3,
		DefaultChatQuota:  50,
		DefaultImageQuota: 20,
		DefaultVideoQuota: 10,
	}
	manager := NewTokenManager(cfg)

	active := &store.Token{
		ID:        1,
		Token:     "active-token",
		Pool:      PoolBasic,
		Status:    string(StatusActive),
		ChatQuota: 7,
	}
	manager.AddToken(active)

	futureCoolUntil := time.Now().Add(10 * time.Minute)
	futureCooling := &store.Token{
		ID:        2,
		Token:     "future-cooling",
		Pool:      PoolBasic,
		Status:    string(StatusCooling),
		ChatQuota: 0,
		CoolUntil: &futureCoolUntil,
	}
	manager.AddToken(futureCooling)

	expiredCoolUntil := time.Now().Add(-1 * time.Minute)
	expiredCooling := &store.Token{
		ID:        3,
		Token:     "expired-cooling",
		Pool:      PoolBasic,
		Status:    string(StatusCooling),
		ChatQuota: 0,
		CoolUntil: &expiredCoolUntil,
	}
	manager.AddToken(expiredCooling)

	scheduler := NewScheduler(manager, cfg, "https://example.com")
	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	cancel()
	scheduler.Stop()

	if active.Status != string(StatusActive) || active.ChatQuota != 7 {
		t.Fatalf("active token should remain unchanged, got status=%s quota=%d", active.Status, active.ChatQuota)
	}
	if futureCooling.Status != string(StatusCooling) || futureCooling.ChatQuota != 0 {
		t.Fatalf("future cooling token should remain cooling, got status=%s quota=%d", futureCooling.Status, futureCooling.ChatQuota)
	}
	if expiredCooling.Status != string(StatusActive) || expiredCooling.ChatQuota != 50 {
		t.Fatalf("expired cooling token should be restored, got status=%s quota=%d", expiredCooling.Status, expiredCooling.ChatQuota)
	}
}

func TestScheduler_Stop(t *testing.T) {
	manager := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	scheduler := NewScheduler(manager, &config.TokenConfig{FailThreshold: 3}, "https://example.com")

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)
	time.Sleep(30 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		cancel()
		scheduler.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Error("Stop() blocked for too long")
	}
}

func TestScheduler_StartSyncsExpiredCoolingTokensInUpstreamMode(t *testing.T) {
	cfg := &config.TokenConfig{
		FailThreshold:     3,
		QuotaRecoveryMode: RecoveryModeUpstream,
		DefaultImageQuota: 20,
		DefaultVideoQuota: 10,
		DefaultChatQuota:  50,
	}
	manager := NewTokenManager(cfg)

	coolUntil := time.Now().Add(-1 * time.Minute)
	token := &store.Token{
		ID:         1,
		Token:      "test-token",
		Pool:       PoolBasic,
		Status:     string(StatusCooling),
		ChatQuota:  0,
		ImageQuota: 0,
		VideoQuota: 0,
		CoolUntil:  &coolUntil,
	}
	manager.AddToken(token)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RateLimitsResponse{
			RemainingQueries:  42,
			WindowSizeSeconds: 7200,
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Fatalf("encode response: %v", err)
		}
	}))
	defer server.Close()

	scheduler := NewScheduler(manager, cfg, server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	cancel()
	scheduler.Stop()

	if token.Status != string(StatusActive) {
		t.Fatalf("expected token status=active, got %s", token.Status)
	}
	if token.ChatQuota != 42 {
		t.Fatalf("expected upstream chat quota 42, got %d", token.ChatQuota)
	}
	if token.ImageQuota != 20 || token.VideoQuota != 10 {
		t.Fatalf("unexpected restored media quotas: image=%d video=%d", token.ImageQuota, token.VideoQuota)
	}
}
