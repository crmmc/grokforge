package token_test

import (
	"context"
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

func TestIntegration_PickAndPersist(t *testing.T) {
	db := setupIntegrationDB(t)
	cfg := &config.TokenConfig{FailThreshold: 3}
	manager := token.NewTokenManager(cfg)

	// Create token in DB
	tok := &store.Token{
		Token:  "integration-token",
		Pool:   token.PoolBasic,
		Status: string(token.StatusActive),
		Quotas: store.IntMap{"auto": 10},
	}
	db.Create(tok)
	manager.AddToken(tok)

	// Start persister
	persister := token.NewPersister(manager, db)
	ctx, cancel := context.WithCancel(context.Background())
	persister.Start(ctx, 50*time.Millisecond)

	t.Run("pick reduces quota and persists", func(t *testing.T) {
		picked, err := manager.Pick(token.PoolBasic, "auto")
		if err != nil {
			t.Fatalf("pick failed: %v", err)
		}
		if picked.Quotas["auto"] != 9 {
			t.Errorf("expected remaining=9, got %d", picked.Quotas["auto"])
		}

		// Wait for periodic flush
		time.Sleep(100 * time.Millisecond)

		// Verify persisted
		var dbTok store.Token
		db.First(&dbTok, tok.ID)
		if dbTok.Quotas["auto"] != 9 {
			t.Errorf("expected DB quota=9, got %d", dbTok.Quotas["auto"])
		}
	})

	t.Run("UpdateModeQuota updates and persists", func(t *testing.T) {
		manager.UpdateModeQuota(tok.ID, "auto", 50, 80)

		// Wait for periodic flush
		time.Sleep(100 * time.Millisecond)

		// Verify persisted
		var dbTok store.Token
		db.First(&dbTok, tok.ID)
		if dbTok.Quotas["auto"] != 50 {
			t.Errorf("expected DB quota=50, got %d", dbTok.Quotas["auto"])
		}
	})

	cancel()
	persister.Stop()
}

func TestIntegration_FullCycle(t *testing.T) {
	db := setupIntegrationDB(t)
	cfg := &config.TokenConfig{FailThreshold: 3}
	manager := token.NewTokenManager(cfg)

	// Create multiple tokens
	for i := 0; i < 5; i++ {
		tok := &store.Token{
			Token:  "token-" + string(rune('a'+i)),
			Pool:   token.PoolBasic,
			Status: string(token.StatusActive),
			Quotas: store.IntMap{"auto": 10},
		}
		db.Create(tok)
		manager.AddToken(tok)
	}

	// Start persister
	persister := token.NewPersister(manager, db)
	ctx, cancel := context.WithCancel(context.Background())
	persister.Start(ctx, 50*time.Millisecond)

	// Pick from all tokens (optimistic deduction)
	for i := uint(1); i <= 5; i++ {
		_, err := manager.Pick(token.PoolBasic, "auto")
		if err != nil {
			t.Errorf("pick token failed: %v", err)
		}
	}

	// Wait for persist
	time.Sleep(100 * time.Millisecond)

	// Verify all persisted
	var tokens []store.Token
	db.Find(&tokens)
	for _, tok := range tokens {
		if tok.Quotas["auto"] != 9 {
			t.Errorf("token %d: expected quota=9, got %d", tok.ID, tok.Quotas["auto"])
		}
	}

	cancel()
	persister.Stop()
}
