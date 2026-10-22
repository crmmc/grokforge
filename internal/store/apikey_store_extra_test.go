package store

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyStore_List_FindErrorAfterCount(t *testing.T) {
	db := setupAPIKeyTestDB(t)
	s := NewAPIKeyStore(db)
	ctx := context.Background()

	require.NoError(t, s.Create(ctx, &APIKey{Name: "one"}))
	failOnNthQuery(t, db, 2) // Count succeeds, Find fails

	keys, total, err := s.List(ctx, 1, 10, "")
	assert.ErrorIs(t, err, errInjected)
	assert.Zero(t, total)
	assert.Nil(t, keys)
}

func TestAPIKeyStore_QueryErrorPaths(t *testing.T) {
	tests := []struct {
		name string
		act  func(ctx context.Context, s *APIKeyStore) error
	}{
		{
			name: "list count query fails",
			act: func(ctx context.Context, s *APIKeyStore) error {
				_, _, err := s.List(ctx, 1, 10, "")
				return err
			},
		},
		{
			name: "get by id query fails",
			act: func(ctx context.Context, s *APIKeyStore) error {
				_, err := s.GetByID(ctx, 1)
				return err
			},
		},
		{
			name: "get by key query fails",
			act: func(ctx context.Context, s *APIKeyStore) error {
				_, err := s.GetByKey(ctx, "gf-x")
				return err
			},
		},
		{
			name: "create query fails",
			act: func(ctx context.Context, s *APIKeyStore) error {
				return s.Create(ctx, &APIKey{Name: "boom"})
			},
		},
		{
			name: "update query fails",
			act: func(ctx context.Context, s *APIKeyStore) error {
				return s.Update(ctx, &APIKey{Name: "boom"})
			},
		},
		{
			name: "delete query fails",
			act: func(ctx context.Context, s *APIKeyStore) error {
				return s.Delete(ctx, 1)
			},
		},
		{
			name: "regenerate query fails",
			act: func(ctx context.Context, s *APIKeyStore) error {
				_, err := s.Regenerate(ctx, 1)
				return err
			},
		},
		{
			name: "count by status query fails",
			act: func(ctx context.Context, s *APIKeyStore) error {
				_, _, _, _, _, err := s.CountByStatus(ctx)
				return err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewAPIKeyStore(closedDB(t))
			err := tc.act(context.Background(), s)
			require.Error(t, err)
			assert.NotErrorIs(t, err, ErrNotFound, "query failures must not be misreported as ErrNotFound")
		})
	}
}

func TestAPIKeyStore_NotFound(t *testing.T) {
	tests := []struct {
		name string
		act  func(ctx context.Context, s *APIKeyStore) error
	}{
		{
			name: "get by id missing",
			act: func(ctx context.Context, s *APIKeyStore) error {
				_, err := s.GetByID(ctx, 99999)
				return err
			},
		},
		{
			name: "get by key missing",
			act: func(ctx context.Context, s *APIKeyStore) error {
				_, err := s.GetByKey(ctx, "gf-missing")
				return err
			},
		},
		{
			name: "delete missing id",
			act: func(ctx context.Context, s *APIKeyStore) error {
				return s.Delete(ctx, 99999)
			},
		},
		{
			name: "regenerate missing id",
			act: func(ctx context.Context, s *APIKeyStore) error {
				_, err := s.Regenerate(ctx, 99999)
				return err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewAPIKeyStore(setupAPIKeyTestDB(t))
			err := tc.act(context.Background(), s)
			assert.ErrorIs(t, err, ErrNotFound)
		})
	}
}

func TestGenerateAPIKey_Format(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 16; i++ {
		key, err := generateAPIKey()
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(key, "gf-"))
		assert.Len(t, key, 51, "gf- prefix plus 48 hex chars")
		assert.False(t, seen[key], "keys must be unique")
		seen[key] = true
	}
}

// TestAPIKeyStore_Update_MissingIDDotUpsertsViaSaveFallback pins gorm's Save
// behavior: when the UPDATE affects no rows, Save falls back to an upsert
// create, so "updating" a missing primary key materializes the record instead
// of surfacing ErrNotFound.
func TestAPIKeyStore_Update_MissingIDDotUpsertsViaSaveFallback(t *testing.T) {
	s := NewAPIKeyStore(setupAPIKeyTestDB(t))
	ctx := context.Background()

	require.NoError(t, s.Update(ctx, &APIKey{Name: "ghost", ID: 99999}))

	found, err := s.GetByID(ctx, 99999)
	require.NoError(t, err)
	assert.Equal(t, "ghost", found.Name)
}

func TestAPIKeyStore_List_StatusFilterAndOrdering(t *testing.T) {
	db := setupAPIKeyTestDB(t)
	s := NewAPIKeyStore(db)
	ctx := context.Background()

	for _, name := range []string{"a", "b", "c"} {
		ak := &APIKey{Name: name, Status: "active"}
		require.NoError(t, s.Create(ctx, ak))
	}

	keys, total, err := s.List(ctx, 1, 2, "active")
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	require.Len(t, keys, 2)
	assert.Equal(t, "active", keys[0].Status)

	// Page 2 returns the remaining key.
	keys, total, err = s.List(ctx, 2, 2, "active")
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	require.Len(t, keys, 1)
}
