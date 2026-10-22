package token

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/modelconfig"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeStoreExtra extends the shared mockTokenStore with error injection.
type fakeStoreExtra struct {
	mockTokenStore
	listErr   error
	getErr    error
	updateErr error
}

func (f *fakeStoreExtra) ListTokens(ctx context.Context) ([]*store.Token, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.mockTokenStore.ListTokens(ctx)
}

func (f *fakeStoreExtra) GetToken(ctx context.Context, id uint) (*store.Token, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.mockTokenStore.GetToken(ctx, id)
}

func (f *fakeStoreExtra) UpdateTokenSnapshots(ctx context.Context, snapshots []store.TokenSnapshotData) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	return f.mockTokenStore.UpdateTokenSnapshots(ctx, snapshots)
}

// fakeRequesterExtra records ForgetToken calls on top of the shared stub.
type fakeRequesterExtra struct {
	refreshRequesterStub
	forgotten []uint
}

func (f *fakeRequesterExtra) ForgetToken(tokenID uint) {
	f.forgotten = append(f.forgotten, tokenID)
}

func TestService_LoadTokens_PropagatesStoreError(t *testing.T) {
	fstore := &fakeStoreExtra{listErr: errors.New("db offline")}
	svc := newTestTokenService(&config.TokenConfig{FailThreshold: 3}, fstore)

	err := svc.LoadTokens(context.Background())
	require.ErrorIs(t, err, fstore.listErr)
}

func TestService_LoadTokens_RejectsInvalidPool(t *testing.T) {
	mockStore := &mockTokenStore{tokens: []*store.Token{
		{ID: 1, Token: "t1", Pool: "weird", Status: string(StatusActive)},
	}}
	svc := newTestTokenService(&config.TokenConfig{FailThreshold: 3}, mockStore)

	err := svc.LoadTokens(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid pool")
}

func TestService_PickAnyExcluding(t *testing.T) {
	svc := newTestTokenService(&config.TokenConfig{FailThreshold: 3}, &mockTokenStore{})
	svc.Manager().AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 10}})

	token, err := svc.PickAnyExcluding(PoolBasic, nil)
	require.NoError(t, err)
	assert.Equal(t, uint(1), token.ID)

	_, err = svc.PickAnyExcluding(PoolSuper, nil)
	assert.ErrorIs(t, err, ErrNoTokenAvailable)
}

func TestService_MarkDisabled(t *testing.T) {
	svc := newTestTokenService(&config.TokenConfig{FailThreshold: 3}, &mockTokenStore{})
	svc.Manager().AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive)})

	svc.MarkDisabled(1, "manual")

	assert.Equal(t, string(StatusDisabled), svc.Manager().GetToken(1).Status)
}

func TestService_MarkExpired(t *testing.T) {
	svc := newTestTokenService(&config.TokenConfig{FailThreshold: 3}, &mockTokenStore{})
	svc.Manager().AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive)})

	svc.MarkExpired(1, "401 unauthorized")

	assert.Equal(t, string(StatusExpired), svc.Manager().GetToken(1).Status)
}

func TestService_RefundQuota(t *testing.T) {
	svc := newTestTokenService(&config.TokenConfig{FailThreshold: 3}, &mockTokenStore{})
	svc.Manager().AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 9}})

	svc.RefundQuota(1, "auto")

	assert.Equal(t, 10, svc.Manager().GetModeQuota(1, "auto"))
}

func TestService_BaseURLAndManager(t *testing.T) {
	svc := newTestTokenService(&config.TokenConfig{FailThreshold: 3}, &mockTokenStore{})

	assert.Equal(t, "https://grok.com", svc.BaseURL())
	assert.NotNil(t, svc.Manager())
}

func TestService_AddToPool(t *testing.T) {
	t.Run("normalizes alias and adds token", func(t *testing.T) {
		svc := newTestTokenService(&config.TokenConfig{FailThreshold: 3}, &mockTokenStore{})

		err := svc.AddToPool(&store.Token{ID: 7, Token: "t7", Pool: "super", Status: string(StatusActive), Quotas: store.IntMap{"auto": 30}})

		require.NoError(t, err)
		assert.Equal(t, PoolSuper, svc.Manager().GetTokenPool(7))
		token, err := svc.Pick(PoolSuper, "auto")
		require.NoError(t, err)
		assert.Equal(t, uint(7), token.ID)
	})

	t.Run("rejects invalid pool", func(t *testing.T) {
		svc := newTestTokenService(&config.TokenConfig{FailThreshold: 3}, &mockTokenStore{})

		err := svc.AddToPool(&store.Token{ID: 8, Token: "t8", Pool: "weird"})

		require.Error(t, err)
		assert.Nil(t, svc.Manager().GetToken(8))
	})
}

func TestService_RemoveFromPool(t *testing.T) {
	svc := newTestTokenService(&config.TokenConfig{FailThreshold: 3}, &mockTokenStore{})
	requester := &fakeRequesterExtra{}
	svc.SetRefreshRequester(requester)
	require.NoError(t, svc.AddToPool(&store.Token{ID: 7, Token: "t7", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 30}}))

	svc.RemoveFromPool(7)

	assert.Nil(t, svc.Manager().GetToken(7))
	assert.Equal(t, []uint{7}, requester.forgotten)
}

func TestService_SyncToken(t *testing.T) {
	t.Run("reloads token from store and forgets scheduler state", func(t *testing.T) {
		mockStore := &mockTokenStore{tokens: []*store.Token{
			{ID: 7, Token: "t7", Pool: "heavy", Status: string(StatusActive), Quotas: store.IntMap{"auto": 30}},
		}}
		svc := newTestTokenService(&config.TokenConfig{FailThreshold: 3}, mockStore)
		requester := &fakeRequesterExtra{}
		svc.SetRefreshRequester(requester)
		// stale in-memory copy with a different pool
		svc.Manager().AddToken(&store.Token{ID: 7, Token: "t7", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 1}})

		require.NoError(t, svc.SyncToken(context.Background(), 7))

		token := svc.Manager().GetToken(7)
		require.NotNil(t, token)
		assert.Equal(t, PoolHeavy, token.Pool)
		assert.Equal(t, []uint{7}, requester.forgotten)
	})

	t.Run("propagates store error", func(t *testing.T) {
		fstore := &fakeStoreExtra{getErr: errors.New("db offline")}
		svc := newTestTokenService(&config.TokenConfig{FailThreshold: 3}, fstore)

		err := svc.SyncToken(context.Background(), 7)

		require.ErrorIs(t, err, fstore.getErr)
	})

	t.Run("rejects invalid pool", func(t *testing.T) {
		mockStore := &mockTokenStore{tokens: []*store.Token{
			{ID: 7, Token: "t7", Pool: "weird", Status: string(StatusActive)},
		}}
		svc := newTestTokenService(&config.TokenConfig{FailThreshold: 3}, mockStore)

		err := svc.SyncToken(context.Background(), 7)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid pool")
	})
}

func TestService_RefreshToken(t *testing.T) {
	ctx := context.Background()

	t.Run("fails when refresh not configured", func(t *testing.T) {
		svc := newTestTokenService(&config.TokenConfig{FailThreshold: 3}, &mockTokenStore{})

		_, err := svc.RefreshToken(ctx, 1)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not configured")
	})

	t.Run("maps ErrTokenNotFound to store.ErrNotFound", func(t *testing.T) {
		svc := newTestTokenService(&config.TokenConfig{FailThreshold: 3}, &mockTokenStore{})
		svc.SetRefreshRequester(&refreshRequesterStub{refreshErr: ErrTokenNotFound})

		_, err := svc.RefreshToken(ctx, 1)

		require.ErrorIs(t, err, store.ErrNotFound)
	})

	t.Run("propagates requester error", func(t *testing.T) {
		svc := newTestTokenService(&config.TokenConfig{FailThreshold: 3}, &mockTokenStore{})
		svc.SetRefreshRequester(&refreshRequesterStub{refreshErr: errors.New("upstream down")})

		_, err := svc.RefreshToken(ctx, 1)

		require.Error(t, err)
	})

	t.Run("flushes dirty tokens and returns snapshot", func(t *testing.T) {
		mockStore := &mockTokenStore{}
		svc := newTestTokenService(&config.TokenConfig{FailThreshold: 3}, mockStore)
		svc.Manager().AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 10}})
		svc.Manager().ClearDirty([]uint{1})
		svc.Manager().MarkDirty(1)
		requester := &refreshRequesterStub{}
		svc.SetRefreshRequester(requester)

		token, err := svc.RefreshToken(ctx, 1)

		require.NoError(t, err)
		require.NotNil(t, token)
		assert.Equal(t, uint(1), token.ID)
		assert.Equal(t, []uint{1}, requester.refreshIDs)
		require.Len(t, mockStore.updated, 1, "dirty snapshots must be persisted")
		assert.Empty(t, svc.Manager().GetDirtyTokens(), "dirty set must be cleared after flush")
	})

	t.Run("propagates flush error", func(t *testing.T) {
		fstore := &fakeStoreExtra{updateErr: errors.New("db offline")}
		svc := NewTokenService(&config.TokenConfig{FailThreshold: 3}, fstore, testModeSpecs(), "https://grok.com")
		svc.Manager().AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 10}})
		svc.Manager().ClearDirty([]uint{1})
		svc.Manager().MarkDirty(1)
		svc.SetRefreshRequester(&refreshRequesterStub{})

		_, err := svc.RefreshToken(ctx, 1)

		require.ErrorIs(t, err, fstore.updateErr)
		assert.Len(t, svc.Manager().GetDirtyTokens(), 1, "failed flush must keep the dirty set")
	})

	t.Run("returns store error for missing token", func(t *testing.T) {
		svc := newTestTokenService(&config.TokenConfig{FailThreshold: 3}, &mockTokenStore{})
		svc.SetRefreshRequester(&refreshRequesterStub{})

		_, err := svc.RefreshToken(ctx, 999)

		require.ErrorIs(t, err, store.ErrNotFound)
	})
}

func TestService_FlushDirty_Branches(t *testing.T) {
	t.Run("empty dirty set succeeds", func(t *testing.T) {
		svc := newTestTokenService(&config.TokenConfig{FailThreshold: 3}, &mockTokenStore{})

		require.NoError(t, svc.FlushDirty(context.Background()))
	})

	t.Run("store error keeps dirty set", func(t *testing.T) {
		fstore := &fakeStoreExtra{updateErr: errors.New("db offline")}
		svc := newTestTokenService(&config.TokenConfig{FailThreshold: 3}, fstore)
		svc.Manager().AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 10}})
		svc.Manager().ClearDirty([]uint{1})
		svc.Manager().MarkDirty(1)

		err := svc.FlushDirty(context.Background())

		require.ErrorIs(t, err, fstore.updateErr)
		assert.Len(t, svc.Manager().GetDirtyTokens(), 1)
	})
}

func TestService_PrimeZeroModeResumeAts(t *testing.T) {
	modes := []modelconfig.ModeSpec{
		{ID: "local_mode", WindowSeconds: 3600, LocalQuota: true, DefaultQuota: map[string]int{"basic": 10}},
		{ID: "remote_mode", WindowSeconds: 7200},
	}
	now := time.Now()
	svc := newTestTokenService(&config.TokenConfig{FailThreshold: 3}, &mockTokenStore{})

	t.Run("no zero modes is noop", func(t *testing.T) {
		token := &store.Token{}
		assert.False(t, svc.primeZeroModeResumeAts(token, nil, modes, now))
		assert.Nil(t, token.ResumeAts)
	})

	t.Run("existing resume is preserved", func(t *testing.T) {
		token := &store.Token{ResumeAts: store.IntMap{"local_mode": 777}}
		assert.False(t, svc.primeZeroModeResumeAts(token, []string{"local_mode"}, modes, now))
		assert.Equal(t, 777, token.ResumeAts["local_mode"])
	})

	t.Run("local mode keeps cooldown window", func(t *testing.T) {
		token := &store.Token{}
		assert.True(t, svc.primeZeroModeResumeAts(token, []string{"local_mode"}, modes, now))
		assert.Greater(t, token.ResumeAts["local_mode"], int(now.Unix()))
	})

	t.Run("upstream mode retries immediately", func(t *testing.T) {
		token := &store.Token{}
		assert.True(t, svc.primeZeroModeResumeAts(token, []string{"remote_mode"}, modes, now))
		assert.InDelta(t, now.Unix(), int64(token.ResumeAts["remote_mode"]), 2)
	})

	t.Run("unknown mode retries immediately", func(t *testing.T) {
		token := &store.Token{}
		assert.True(t, svc.primeZeroModeResumeAts(token, []string{"mystery"}, modes, now))
		assert.InDelta(t, now.Unix(), int64(token.ResumeAts["mystery"]), 2)
	})
}

func TestNormalizeTokenPool(t *testing.T) {
	assert.NoError(t, normalizeTokenPool(nil))

	token := &store.Token{ID: 5, Pool: "super"}
	require.NoError(t, normalizeTokenPool(token))
	assert.Equal(t, PoolSuper, token.Pool)

	bad := &store.Token{ID: 6, Pool: "weird"}
	err := normalizeTokenPool(bad)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "token 6")
}
