package token

import (
	"context"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestDB(t *testing.T) *gorm.DB {
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

func TestPersister_FlushDirty(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.TokenConfig{FailThreshold: 3}
	m := NewTokenManager(cfg)

	// Create token in DB first
	token := &store.Token{
		Token:     "test-token",
		Pool:      PoolBasic,
		Status:    string(StatusActive),
		ChatQuota: 100,
	}
	db.Create(token)

	// Add to manager and modify
	m.AddToken(token)
	m.Consume(token.ID, CategoryChat, 1) // quota -> 99, marks dirty

	persister := NewPersister(m, db)

	t.Run("flushes dirty tokens to database", func(t *testing.T) {
		count, err := persister.FlushDirty(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 1 {
			t.Errorf("expected count=1, got %d", count)
		}

		// Verify in DB
		var dbToken store.Token
		db.First(&dbToken, token.ID)
		if dbToken.ChatQuota != 99 {
			t.Errorf("expected DB ChatQuota=99, got %d", dbToken.ChatQuota)
		}
	})

	t.Run("clears dirty set after flush", func(t *testing.T) {
		// No more dirty tokens
		count, err := persister.FlushDirty(context.Background())
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if count != 0 {
			t.Errorf("expected count=0 after flush, got %d", count)
		}
	})
}

func TestPersister_PeriodicFlush(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.TokenConfig{FailThreshold: 3}
	m := NewTokenManager(cfg)

	token := &store.Token{
		Token:     "test-token",
		Pool:      PoolBasic,
		Status:    string(StatusActive),
		ChatQuota: 100,
	}
	db.Create(token)
	m.AddToken(token)

	persister := NewPersister(m, db)

	ctx, cancel := context.WithCancel(context.Background())
	persister.Start(ctx, 50*time.Millisecond)

	// Modify token
	m.Consume(token.ID, CategoryChat, 1)

	// Wait for periodic flush
	time.Sleep(100 * time.Millisecond)

	cancel()
	persister.Stop()

	// Verify persisted
	var dbToken store.Token
	db.First(&dbToken, token.ID)
	if dbToken.ChatQuota != 99 {
		t.Errorf("expected DB ChatQuota=99, got %d", dbToken.ChatQuota)
	}
}

func TestPersister_BatchUpdate(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.TokenConfig{FailThreshold: 3}
	m := NewTokenManager(cfg)

	// Create multiple tokens
	for i := 0; i < 10; i++ {
		token := &store.Token{
			Token:     "token-" + string(rune('a'+i)),
			Pool:      PoolBasic,
			Status:    string(StatusActive),
			ChatQuota: 100,
		}
		db.Create(token)
		m.AddToken(token)
		m.Consume(token.ID, CategoryChat, 1) // mark dirty
	}

	persister := NewPersister(m, db)

	count, err := persister.FlushDirty(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 10 {
		t.Errorf("expected count=10, got %d", count)
	}

	// Verify all persisted
	var tokens []store.Token
	db.Find(&tokens)
	for _, tok := range tokens {
		if tok.ChatQuota != 99 {
			t.Errorf("token %d: expected ChatQuota=99, got %d", tok.ID, tok.ChatQuota)
		}
	}
}

func TestPersister_Stop(t *testing.T) {
	db := setupTestDB(t)
	cfg := &config.TokenConfig{FailThreshold: 3}
	m := NewTokenManager(cfg)

	persister := NewPersister(m, db)

	ctx, cancel := context.WithCancel(context.Background())
	persister.Start(ctx, 10*time.Millisecond)

	time.Sleep(30 * time.Millisecond)

	// Stop should complete without blocking
	done := make(chan struct{})
	go func() {
		cancel()
		persister.Stop()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(1 * time.Second):
		t.Error("Stop() blocked for too long")
	}
}
