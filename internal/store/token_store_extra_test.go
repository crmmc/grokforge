package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedTokenStoreTokens(t *testing.T, s *TokenStore) []*Token {
	t.Helper()
	ctx := context.Background()
	tokens := []*Token{
		{Token: "extra-tok-1", Pool: "ssoBasic", Status: TokenStatusActive, Priority: 1},
		{Token: "extra-tok-2", Pool: "ssoBasic", Status: TokenStatusDisabled},
		{Token: "extra-tok-3", Pool: "ssoSuper", Status: TokenStatusActive, NsfwEnabled: true},
	}
	for _, tok := range tokens {
		require.NoError(t, s.CreateToken(ctx, tok))
	}
	return tokens
}

func TestTokenStore_ListTokens(t *testing.T) {
	s := NewTokenStore(setupTokenTestDB(t))
	ctx := context.Background()

	tokens, err := s.ListTokens(ctx)
	require.NoError(t, err)
	assert.Empty(t, tokens)

	seedTokenStoreTokens(t, s)
	tokens, err = s.ListTokens(ctx)
	require.NoError(t, err)
	assert.Len(t, tokens, 3)
}

func TestTokenStore_ListTokens_QueryError(t *testing.T) {
	s := NewTokenStore(closedDB(t))
	tokens, err := s.ListTokens(context.Background())
	assert.Error(t, err)
	assert.Empty(t, tokens)
}

func TestTokenStore_ListTokenIDs(t *testing.T) {
	s := NewTokenStore(setupTokenTestDB(t))
	ctx := context.Background()
	seeded := seedTokenStoreTokens(t, s)
	activeStatus := TokenStatusActive
	nsfwTrue := true

	tests := []struct {
		name    string
		filter  TokenFilter
		wantIDs []uint
	}{
		{name: "no filter returns all", filter: TokenFilter{}, wantIDs: []uint{seeded[0].ID, seeded[1].ID, seeded[2].ID}},
		{name: "status filter", filter: TokenFilter{Status: &activeStatus}, wantIDs: []uint{seeded[0].ID, seeded[2].ID}},
		{name: "nsfw filter", filter: TokenFilter{NsfwEnabled: &nsfwTrue}, wantIDs: []uint{seeded[2].ID}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ids, err := s.ListTokenIDs(ctx, tc.filter)
			require.NoError(t, err)
			assert.ElementsMatch(t, tc.wantIDs, ids)
		})
	}
}

func TestTokenStore_ListTokenIDs_QueryError(t *testing.T) {
	s := NewTokenStore(closedDB(t))
	ids, err := s.ListTokenIDs(context.Background(), TokenFilter{})
	assert.Error(t, err)
	assert.Empty(t, ids)
}

func TestTokenStore_GetToken(t *testing.T) {
	s := NewTokenStore(setupTokenTestDB(t))
	ctx := context.Background()
	seeded := seedTokenStoreTokens(t, s)

	found, err := s.GetToken(ctx, seeded[0].ID)
	require.NoError(t, err)
	assert.Equal(t, "extra-tok-1", found.Token)

	_, err = s.GetToken(ctx, 99999)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestTokenStore_GetToken_QueryError(t *testing.T) {
	s := NewTokenStore(closedDB(t))
	found, err := s.GetToken(context.Background(), 1)
	assert.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotFound)
	assert.NotNil(t, found, "query errors return the zero record, not ErrNotFound")
}

func TestTokenStore_UpdateToken(t *testing.T) {
	s := NewTokenStore(setupTokenTestDB(t))
	ctx := context.Background()
	seeded := seedTokenStoreTokens(t, s)

	seeded[0].Status = TokenStatusExpired
	seeded[0].Remark = "updated"
	require.NoError(t, s.UpdateToken(ctx, seeded[0]))

	found, err := s.GetToken(ctx, seeded[0].ID)
	require.NoError(t, err)
	assert.Equal(t, TokenStatusExpired, found.Status)
	assert.Equal(t, "updated", found.Remark)

	// gorm's Save() falls back to an upsert-style create when the UPDATE
	// affects no rows, so "updating" a missing primary key materializes the
	// record instead of surfacing ErrNotFound.
	ghost := &Token{Token: "ghost", ID: 99999}
	require.NoError(t, s.UpdateToken(ctx, ghost))
	found, err = s.GetToken(ctx, 99999)
	require.NoError(t, err)
	assert.Equal(t, "ghost", found.Token)
}

func TestTokenStore_UpdateToken_QueryError(t *testing.T) {
	s := NewTokenStore(closedDB(t))
	err := s.UpdateToken(context.Background(), &Token{Token: "boom"})
	assert.Error(t, err)
}

func TestTokenStore_DeleteToken(t *testing.T) {
	s := NewTokenStore(setupTokenTestDB(t))
	ctx := context.Background()
	seeded := seedTokenStoreTokens(t, s)

	require.NoError(t, s.DeleteToken(ctx, seeded[0].ID))
	_, err := s.GetToken(ctx, seeded[0].ID)
	assert.ErrorIs(t, err, ErrNotFound, "soft-deleted token must disappear")

	assert.ErrorIs(t, s.DeleteToken(ctx, seeded[0].ID), ErrNotFound)
	assert.ErrorIs(t, s.DeleteToken(ctx, 99999), ErrNotFound)
}

func TestTokenStore_DeleteToken_QueryError(t *testing.T) {
	s := NewTokenStore(closedDB(t))
	assert.Error(t, s.DeleteToken(context.Background(), 1))
}

func TestTokenStore_UpdateTokenSnapshots(t *testing.T) {
	s := NewTokenStore(setupTokenTestDB(t))
	ctx := context.Background()
	seeded := seedTokenStoreTokens(t, s)

	lastUsed := time.Unix(1700000100, 0).UTC()
	snapshots := []TokenSnapshotData{
		{
			ID:           seeded[0].ID,
			Status:       TokenStatusExpired,
			StatusReason: "refresh failed",
			Quotas:       IntMap{"auto": 3},
			LimitQuotas:  IntMap{"auto": 50},
			FailCount:    2,
			LastUsed:     &lastUsed,
			ResumeAts:    IntMap{"auto": 1700000200},
		},
		{
			ID:     seeded[2].ID,
			Status: TokenStatusDisabled,
			Quotas: IntMap{"heavy": 0},
		},
	}
	require.NoError(t, s.UpdateTokenSnapshots(ctx, snapshots))

	first, err := s.GetToken(ctx, seeded[0].ID)
	require.NoError(t, err)
	assert.Equal(t, TokenStatusExpired, first.Status)
	assert.Equal(t, "refresh failed", first.StatusReason)
	assert.Equal(t, IntMap{"auto": 3}, first.Quotas)
	assert.Equal(t, IntMap{"auto": 50}, first.LimitQuotas)
	assert.Equal(t, 2, first.FailCount)
	require.NotNil(t, first.LastUsed)
	assert.True(t, lastUsed.Equal(*first.LastUsed))
	assert.Equal(t, IntMap{"auto": 1700000200}, first.ResumeAts)

	third, err := s.GetToken(ctx, seeded[2].ID)
	require.NoError(t, err)
	assert.Equal(t, TokenStatusDisabled, third.Status)
	assert.Equal(t, IntMap{"heavy": 0}, third.Quotas)
	assert.Empty(t, third.StatusReason)
}

func TestTokenStore_UpdateTokenSnapshots_QueryError(t *testing.T) {
	s := NewTokenStore(closedDB(t))
	err := s.UpdateTokenSnapshots(context.Background(), []TokenSnapshotData{{ID: 1, Status: TokenStatusActive}})
	assert.Error(t, err)
}

func TestTokenStore_BatchUpdateTokens_TransactionError(t *testing.T) {
	db := setupTokenTestDB(t)
	s := NewTokenStore(db)
	ctx := context.Background()
	seeded := seedTokenStoreTokens(t, s)
	failOnNthProcessor(t, db, "update", 1) // the UPDATE inside the tx fails

	status := TokenStatusDisabled
	count, err := s.BatchUpdateTokens(ctx, BatchUpdateRequest{
		IDs:    []uint{seeded[0].ID},
		Status: &status,
	})
	assert.ErrorIs(t, err, errInjected)
	assert.Zero(t, count)

	found, err := s.GetToken(ctx, seeded[0].ID)
	require.NoError(t, err)
	assert.Equal(t, TokenStatusActive, found.Status, "failed transaction must roll back the status change")
}

func TestTokenStore_UpdateTokenSnapshots_TransactionError(t *testing.T) {
	db := setupTokenTestDB(t)
	s := NewTokenStore(db)
	ctx := context.Background()
	seeded := seedTokenStoreTokens(t, s)
	failOnNthProcessor(t, db, "update", 1) // first snapshot UPDATE inside the tx fails

	err := s.UpdateTokenSnapshots(ctx, []TokenSnapshotData{
		{ID: seeded[0].ID, Status: TokenStatusExpired, Quotas: IntMap{"auto": 1}},
		{ID: seeded[1].ID, Status: TokenStatusActive, Quotas: IntMap{"auto": 2}},
	})
	assert.ErrorIs(t, err, errInjected)

	found, err := s.GetToken(ctx, seeded[0].ID)
	require.NoError(t, err)
	assert.Equal(t, TokenStatusActive, found.Status, "failed transaction must roll back snapshot updates")
}

func TestTokenStore_BatchUpdateTokens_StatusReason(t *testing.T) {
	s := NewTokenStore(setupTokenTestDB(t))
	ctx := context.Background()
	seeded := seedTokenStoreTokens(t, s)

	reason := "quota exhausted"
	count, err := s.BatchUpdateTokens(ctx, BatchUpdateRequest{
		IDs:          []uint{seeded[0].ID, seeded[1].ID},
		StatusReason: &reason,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	for _, id := range []uint{seeded[0].ID, seeded[1].ID} {
		found, err := s.GetToken(ctx, id)
		require.NoError(t, err)
		assert.Equal(t, reason, found.StatusReason)
	}
}

func TestTokenStore_BatchUpdateTokens_EmptyRequest(t *testing.T) {
	s := NewTokenStore(setupTokenTestDB(t))
	ctx := context.Background()
	seeded := seedTokenStoreTokens(t, s)

	// No update fields set: nothing happens, no error.
	count, err := s.BatchUpdateTokens(ctx, BatchUpdateRequest{IDs: []uint{seeded[0].ID}})
	require.NoError(t, err)
	assert.Zero(t, count)

	found, err := s.GetToken(ctx, seeded[0].ID)
	require.NoError(t, err)
	assert.Equal(t, TokenStatusActive, found.Status, "token must be untouched by empty batch request")
}

func TestTokenStore_BatchUpdateTokens_QueryError(t *testing.T) {
	s := NewTokenStore(closedDB(t))
	status := TokenStatusDisabled
	count, err := s.BatchUpdateTokens(context.Background(), BatchUpdateRequest{
		IDs:    []uint{1},
		Status: &status,
	})
	assert.Error(t, err)
	assert.Zero(t, count)
}
