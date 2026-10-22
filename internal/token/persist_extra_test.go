package token

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupRawDB opens an in-memory sqlite database without migrating the token
// table, so token updates fail inside an otherwise working transaction.
func setupRawDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	return db
}

// setupFileDB opens a file-backed sqlite database in a temp dir. Unlike
// ":memory:", a file DB is shared across pool connections, so a writer goroutine
// and a polling reader always observe the same data.
func setupFileDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "tokens.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&store.Token{}))
	return db
}

func TestPersister_Start_ZeroIntervalUsesDefault(t *testing.T) {
	db := setupTestDB(t)
	mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	p := NewPersister(mgr, db)

	p.Start(context.Background(), 0) // interval <= 0 must fall back to default
	p.Stop()                         // loop exits via the stopped channel and performs a final flush
}

func TestPersister_Run_FinalFlushOnContextCancel(t *testing.T) {
	db := setupFileDB(t)
	mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	tok := &store.Token{Token: "t-final", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 100}}
	require.NoError(t, db.Create(tok).Error)
	mgr.AddToken(tok)

	p := NewPersister(mgr, db)
	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx, time.Hour)

	_, err := mgr.Pick(PoolBasic, "auto") // 100 -> 99, marks dirty
	require.NoError(t, err)
	cancel()

	waitUntil(t, 2*time.Second, func() bool {
		var dbToken store.Token
		if err := db.First(&dbToken, tok.ID).Error; err != nil {
			return false
		}
		return dbToken.Quotas["auto"] == 99
	}, "context cancel must trigger a final flush")

	p.Stop()
}

func TestPersister_FlushDirty_DatabaseErrorKeepsDirty(t *testing.T) {
	db := setupTestDB(t)
	mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	tok := &store.Token{Token: "t-err", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 100}}
	require.NoError(t, db.Create(tok).Error)
	mgr.AddToken(tok)
	mgr.ClearDirty([]uint{tok.ID})
	mgr.MarkDirty(tok.ID)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	p := NewPersister(mgr, db)
	count, err := p.FlushDirty(context.Background())
	require.Error(t, err)
	assert.Equal(t, 0, count)
	assert.Len(t, mgr.GetDirtyTokens(), 1, "failed flush must keep tokens dirty")
}

func TestPersister_FlushDirty_UpdateErrorInsideTransaction(t *testing.T) {
	db := setupRawDB(t) // no tokens table: transaction opens, the update fails
	mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 9}})
	mgr.ClearDirty([]uint{1})
	mgr.MarkDirty(1)

	p := NewPersister(mgr, db)
	count, err := p.FlushDirty(context.Background())
	require.Error(t, err)
	assert.Equal(t, 0, count)
	assert.Len(t, mgr.GetDirtyTokens(), 1, "failed flush must keep tokens dirty")
}

func TestPersister_Run_TickerFlushFailureIsLogged(t *testing.T) {
	db := setupRawDB(t)
	mgr := NewTokenManager(&config.TokenConfig{FailThreshold: 3})
	mgr.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 9}})
	mgr.ClearDirty([]uint{1})
	mgr.MarkDirty(1)

	handler := newCaptureHandler("failed to flush dirty tokens")
	orig := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(orig) })

	p := NewPersister(mgr, db)
	ctx, cancel := context.WithCancel(context.Background())
	p.Start(ctx, 5*time.Millisecond)

	select {
	case <-handler.signal:
	case <-time.After(2 * time.Second):
		t.Fatal("ticker flush failure was never logged")
	}

	cancel()
	p.Stop()
	assert.Len(t, mgr.GetDirtyTokens(), 1, "failed flush must keep tokens dirty")
}
