package token_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/crmmc/grokforge/internal/token"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupIntegrationDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	if err := db.AutoMigrate(&store.Token{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	return db
}

func TestIntegration_QuotaLifecycle(t *testing.T) {
	db := setupIntegrationDB(t)
	cfg := &config.TokenConfig{FailThreshold: 3}
	manager := token.NewTokenManager(cfg)

	// Mock upstream API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := token.RateLimitsResponse{
			RemainingQueries:  50,
			WindowSizeSeconds: 7200,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create token in DB
	tok := &store.Token{
		Token:     "integration-token",
		Pool:      token.PoolBasic,
		Status:    string(token.StatusActive),
		ChatQuota: 10,
	}
	db.Create(tok)
	manager.AddToken(tok)

	// Start persister
	persister := token.NewPersister(manager, db)
	ctx, cancel := context.WithCancel(context.Background())
	persister.Start(ctx, 50*time.Millisecond)

	t.Run("consume reduces quota and persists", func(t *testing.T) {
		remaining, err := manager.Consume(tok.ID, token.CategoryChat, 1)
		if err != nil {
			t.Fatalf("consume failed: %v", err)
		}
		if remaining != 9 {
			t.Errorf("expected remaining=9, got %d", remaining)
		}

		// Wait for periodic flush
		time.Sleep(100 * time.Millisecond)

		// Verify persisted
		var dbTok store.Token
		db.First(&dbTok, tok.ID)
		if dbTok.ChatQuota != 9 {
			t.Errorf("expected DB quota=9, got %d", dbTok.ChatQuota)
		}
	})

	t.Run("sync updates quota from API", func(t *testing.T) {
		err := manager.SyncQuota(ctx, tok, server.URL)
		if err != nil {
			t.Fatalf("sync failed: %v", err)
		}

		// Wait for periodic flush
		time.Sleep(100 * time.Millisecond)

		// Verify persisted - read from DB to avoid race
		var dbTok store.Token
		db.First(&dbTok, tok.ID)
		if dbTok.ChatQuota != 50 {
			t.Errorf("expected DB quota=50, got %d", dbTok.ChatQuota)
		}
	})

	cancel()
	persister.Stop()
}

func TestIntegration_CoolingRecovery(t *testing.T) {
	db := setupIntegrationDB(t)
	cfg := &config.TokenConfig{FailThreshold: 3}
	manager := token.NewTokenManager(cfg)

	// Mock upstream API returns quota
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := token.RateLimitsResponse{
			RemainingQueries:  30,
			WindowSizeSeconds: 7200,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create cooling token with expired CoolUntil
	coolUntil := time.Now().Add(-1 * time.Minute)
	tok := &store.Token{
		Token:     "cooling-token",
		Pool:      token.PoolBasic,
		Status:    string(token.StatusCooling),
		ChatQuota: 0,
		CoolUntil: &coolUntil,
	}
	db.Create(tok)
	manager.AddToken(tok)

	// Start scheduler and persister
	scheduler := token.NewScheduler(manager, &config.TokenConfig{QuotaRecoveryMode: token.RecoveryModeUpstream}, server.URL)
	persister := token.NewPersister(manager, db)

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)
	persister.Start(ctx, 50*time.Millisecond)

	// Wait for refresh and persist
	time.Sleep(200 * time.Millisecond)

	cancel()
	scheduler.Stop()
	persister.Stop()

	// Verify persisted - read from DB to avoid race
	var dbTok store.Token
	db.First(&dbTok, tok.ID)
	if dbTok.Status != string(token.StatusActive) {
		t.Errorf("expected DB status=active, got %s", dbTok.Status)
	}
	if dbTok.ChatQuota != 30 {
		t.Errorf("expected DB quota=30, got %d", dbTok.ChatQuota)
	}
}

func TestIntegration_FullCycle(t *testing.T) {
	db := setupIntegrationDB(t)
	cfg := &config.TokenConfig{FailThreshold: 3}
	manager := token.NewTokenManager(cfg)

	// Mock upstream API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := token.RateLimitsResponse{
			RemainingQueries:  100,
			WindowSizeSeconds: 7200,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create multiple tokens
	for i := 0; i < 5; i++ {
		tok := &store.Token{
			Token:     "token-" + string(rune('a'+i)),
			Pool:      token.PoolBasic,
			Status:    string(token.StatusActive),
			ChatQuota: 10,
		}
		db.Create(tok)
		manager.AddToken(tok)
	}

	// Start components
	scheduler := token.NewScheduler(manager, &config.TokenConfig{QuotaRecoveryMode: token.RecoveryModeUpstream}, server.URL)
	persister := token.NewPersister(manager, db)

	ctx, cancel := context.WithCancel(context.Background())
	scheduler.Start(ctx)
	persister.Start(ctx, 50*time.Millisecond)

	// Consume from all tokens
	for i := uint(1); i <= 5; i++ {
		_, err := manager.Consume(i, token.CategoryChat, 1)
		if err != nil {
			t.Errorf("consume token %d failed: %v", i, err)
		}
	}

	// Wait for persist
	time.Sleep(100 * time.Millisecond)

	// Verify all persisted
	var tokens []store.Token
	db.Find(&tokens)
	for _, tok := range tokens {
		if tok.ChatQuota != 9 {
			t.Errorf("token %d: expected quota=9, got %d", tok.ID, tok.ChatQuota)
		}
	}

	cancel()
	scheduler.Stop()
	persister.Stop()
}
