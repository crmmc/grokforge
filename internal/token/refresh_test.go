package token

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/modelconfig"
	"github.com/crmmc/grokforge/internal/store"
)

func TestScheduler_Stop(t *testing.T) {
	manager := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	scheduler := NewScheduler(manager, nil, "https://example.com")

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

func TestScheduler_RecordFirstUsed(t *testing.T) {
	manager := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	scheduler := NewScheduler(manager, nil, "https://example.com")

	scheduler.RecordFirstUsed(1, "auto")

	// Record again — should not overwrite
	scheduler.RecordFirstUsed(1, "auto")

	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	if _, ok := scheduler.firstUsedAt[1]; !ok {
		t.Fatal("expected firstUsedAt[1] to exist")
	}
	if _, ok := scheduler.firstUsedAt[1]["auto"]; !ok {
		t.Fatal("expected firstUsedAt[1][auto] to exist")
	}
}

func TestScheduler_SetFirstUsedAt(t *testing.T) {
	manager := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	scheduler := NewScheduler(manager, nil, "https://example.com")

	ts := time.Now().Add(-1 * time.Hour)
	scheduler.SetFirstUsedAt(1, "auto", ts)

	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	got := scheduler.firstUsedAt[1]["auto"]
	if !got.Equal(ts) {
		t.Errorf("expected firstUsedAt = %v, got %v", ts, got)
	}
}

func TestScheduler_RefreshesExpiredMode(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	manager := NewTokenManager(cfg)

	tok := &store.Token{
		ID:     1,
		Token:  "test-token",
		Pool:   PoolBasic,
		Status: string(StatusActive),
		Quotas: store.IntMap{"auto": 0},
	}
	manager.AddToken(tok)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RateLimitsResponse{
			RemainingQueries:  42,
			TotalQueries:      80,
			WindowSizeSeconds: 7200,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	modes := []modelconfig.ModeSpec{
		{
			ID:            "auto",
			UpstreamName:  "auto",
			WindowSeconds: 1, // 1 second window so it expires immediately
			DefaultQuota:  map[string]int{"basic": 50},
		},
	}

	scheduler := NewScheduler(manager, modes, server.URL)

	// Set first_used_at to the past so the window is expired
	scheduler.SetFirstUsedAt(1, "auto", time.Now().Add(-10*time.Second))

	// Directly trigger a scan instead of waiting for the ticker
	scheduler.scan(context.Background())

	// Verify quota was updated
	updated := manager.GetToken(1)
	if updated.Quotas["auto"] != 42 {
		t.Errorf("expected Quotas[auto]=42, got %d", updated.Quotas["auto"])
	}
	if updated.LimitQuotas["auto"] != 80 {
		t.Errorf("expected LimitQuotas[auto]=80, got %d", updated.LimitQuotas["auto"])
	}
}
