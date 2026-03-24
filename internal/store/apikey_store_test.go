package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupAPIKeyTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))
	return db
}

func TestAPIKeyStore_Create(t *testing.T) {
	db := setupAPIKeyTestDB(t)
	s := NewAPIKeyStore(db)
	ctx := context.Background()

	ak := &APIKey{Name: "test-key"}
	err := s.Create(ctx, ak)
	require.NoError(t, err)
	assert.NotZero(t, ak.ID)
	assert.True(t, strings.HasPrefix(ak.Key, "gf-"), "key should have gf- prefix")
	assert.Equal(t, 51, len(ak.Key), "key should be 51 chars total (gf- + 48 hex)")

	// Verify stored in DB
	var found APIKey
	require.NoError(t, db.First(&found, ak.ID).Error)
	assert.Equal(t, ak.Key, found.Key)
	assert.Equal(t, "test-key", found.Name)
}

func TestAPIKeyStore_List(t *testing.T) {
	db := setupAPIKeyTestDB(t)
	s := NewAPIKeyStore(db)
	ctx := context.Background()

	// Create 5 keys with different statuses
	for i := 0; i < 3; i++ {
		ak := &APIKey{Name: "active-key", Status: "active"}
		require.NoError(t, s.Create(ctx, ak))
	}
	for i := 0; i < 2; i++ {
		ak := &APIKey{Name: "inactive-key"}
		require.NoError(t, s.Create(ctx, ak))
		ak.Status = "inactive"
		require.NoError(t, s.Update(ctx, ak))
	}

	// List all (page 1, size 20)
	keys, total, err := s.List(ctx, 1, 20, "")
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, keys, 5)

	// List with status filter
	keys, total, err = s.List(ctx, 1, 20, "active")
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	assert.Len(t, keys, 3)

	// List with pagination
	keys, total, err = s.List(ctx, 1, 2, "")
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, keys, 2)
}

func TestAPIKeyStore_GetByKey(t *testing.T) {
	db := setupAPIKeyTestDB(t)
	s := NewAPIKeyStore(db)
	ctx := context.Background()

	ak := &APIKey{Name: "lookup-key"}
	require.NoError(t, s.Create(ctx, ak))

	// Find by full key string
	found, err := s.GetByKey(ctx, ak.Key)
	require.NoError(t, err)
	assert.Equal(t, ak.ID, found.ID)
	assert.Equal(t, ak.Key, found.Key)

	// Not found
	found, err = s.GetByKey(ctx, "gf-nonexistent")
	assert.Error(t, err)
	assert.Nil(t, found)
}

func TestAPIKeyStore_Update(t *testing.T) {
	db := setupAPIKeyTestDB(t)
	s := NewAPIKeyStore(db)
	ctx := context.Background()

	ak := &APIKey{Name: "original"}
	require.NoError(t, s.Create(ctx, ak))

	ak.Name = "updated"
	ak.RateLimit = 120
	require.NoError(t, s.Update(ctx, ak))

	found, err := s.GetByID(ctx, ak.ID)
	require.NoError(t, err)
	assert.Equal(t, "updated", found.Name)
	assert.Equal(t, 120, found.RateLimit)
}

func TestAPIKeyStore_Delete(t *testing.T) {
	db := setupAPIKeyTestDB(t)
	s := NewAPIKeyStore(db)
	ctx := context.Background()

	ak := &APIKey{Name: "delete-me"}
	require.NoError(t, s.Create(ctx, ak))

	require.NoError(t, s.Delete(ctx, ak.ID))

	// Should not be found after soft delete
	_, err := s.GetByID(ctx, ak.ID)
	assert.Error(t, err)
}

func TestAPIKeyStore_Regenerate(t *testing.T) {
	db := setupAPIKeyTestDB(t)
	s := NewAPIKeyStore(db)
	ctx := context.Background()

	ak := &APIKey{Name: "regen-key"}
	require.NoError(t, s.Create(ctx, ak))
	oldKey := ak.Key

	newKey, err := s.Regenerate(ctx, ak.ID)
	require.NoError(t, err)
	assert.NotEqual(t, oldKey, newKey)
	assert.True(t, strings.HasPrefix(newKey, "gf-"))
	assert.Equal(t, 51, len(newKey))

	// Old key should no longer work
	_, err = s.GetByKey(ctx, oldKey)
	assert.Error(t, err)

	// New key should work
	found, err := s.GetByKey(ctx, newKey)
	require.NoError(t, err)
	assert.Equal(t, ak.ID, found.ID)
}

func TestAPIKeyStore_CountByStatus(t *testing.T) {
	db := setupAPIKeyTestDB(t)
	s := NewAPIKeyStore(db)
	ctx := context.Background()

	// Create keys with various statuses
	statuses := []string{"active", "active", "active", "inactive", "expired", "rate_limited"}
	for _, status := range statuses {
		ak := &APIKey{Name: "key-" + status}
		require.NoError(t, s.Create(ctx, ak))
		ak.Status = status
		require.NoError(t, s.Update(ctx, ak))
	}

	total, active, inactive, expired, rateLimited, err := s.CountByStatus(ctx)
	require.NoError(t, err)
	assert.Equal(t, 6, total)
	assert.Equal(t, 3, active)
	assert.Equal(t, 1, inactive)
	assert.Equal(t, 1, expired)
	assert.Equal(t, 1, rateLimited)
}

func TestAPIKeyStore_IncrementUsage(t *testing.T) {
	db := setupAPIKeyTestDB(t)
	s := NewAPIKeyStore(db)
	ctx := context.Background()

	ak := &APIKey{Name: "usage-key"}
	require.NoError(t, s.Create(ctx, ak))

	// Increment twice
	require.NoError(t, s.IncrementUsage(ctx, ak.ID))
	require.NoError(t, s.IncrementUsage(ctx, ak.ID))

	found, err := s.GetByID(ctx, ak.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, found.DailyUsed)
	assert.Equal(t, 2, found.TotalUsed)
	assert.NotNil(t, found.LastUsedAt)
}

func TestAPIKeyStore_ResetDailyUsage(t *testing.T) {
	db := setupAPIKeyTestDB(t)
	s := NewAPIKeyStore(db)
	ctx := context.Background()

	ak := &APIKey{Name: "reset-key"}
	require.NoError(t, s.Create(ctx, ak))

	// Build up some usage
	require.NoError(t, s.IncrementUsage(ctx, ak.ID))
	require.NoError(t, s.IncrementUsage(ctx, ak.ID))
	require.NoError(t, s.IncrementUsage(ctx, ak.ID))

	// Verify usage before reset
	found, err := s.GetByID(ctx, ak.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, found.DailyUsed)
	assert.Equal(t, 3, found.TotalUsed)

	// Reset daily
	require.NoError(t, s.ResetDailyUsage(ctx))

	// daily_used should be 0, total_used preserved
	found, err = s.GetByID(ctx, ak.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, found.DailyUsed)
	assert.Equal(t, 3, found.TotalUsed, "total_used should be preserved after daily reset")
}

// Ensure time import is used
var _ = time.Now
