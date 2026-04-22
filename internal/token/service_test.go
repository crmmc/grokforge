package token

import (
	"context"
	"testing"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTokenStore implements store.TokenStore for testing
type mockTokenStore struct {
	tokens  []*store.Token
	updated []store.TokenSnapshotData
}

func (m *mockTokenStore) ListTokens(ctx context.Context) ([]*store.Token, error) {
	return m.tokens, nil
}

func (m *mockTokenStore) GetToken(ctx context.Context, id uint) (*store.Token, error) {
	for _, t := range m.tokens {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, store.ErrNotFound
}

func (m *mockTokenStore) UpdateTokenSnapshots(ctx context.Context, snapshots []store.TokenSnapshotData) error {
	m.updated = append(m.updated, snapshots...)
	return nil
}

func TestService_LoadTokens(t *testing.T) {
	mockStore := &mockTokenStore{
		tokens: []*store.Token{
			{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 80}},
			{ID: 2, Token: "t2", Pool: PoolSuper, Status: string(StatusActive), Quotas: store.IntMap{"auto": 140}},
		},
	}

	cfg := &config.TokenConfig{FailThreshold: 3}
	svc := NewTokenService(cfg, mockStore, "https://grok.com")

	err := svc.LoadTokens(context.Background())
	require.NoError(t, err)

	// Verify tokens are loaded into manager
	token, err := svc.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	assert.Equal(t, uint(1), token.ID)

	token, err = svc.Pick(PoolSuper, "auto")
	require.NoError(t, err)
	assert.Equal(t, uint(2), token.ID)
}

func TestService_LoadTokens_NormalizesPoolAlias(t *testing.T) {
	mockStore := &mockTokenStore{
		tokens: []*store.Token{
			{ID: 1, Token: "t1", Pool: "heavy", Status: string(StatusActive), Quotas: store.IntMap{"auto": 80}},
		},
	}

	cfg := &config.TokenConfig{FailThreshold: 3}
	svc := NewTokenService(cfg, mockStore, "https://grok.com")

	err := svc.LoadTokens(context.Background())
	require.NoError(t, err)

	token, err := svc.Pick(PoolHeavy, "auto")
	require.NoError(t, err)
	assert.Equal(t, PoolHeavy, token.Pool)
}

func TestService_Pick(t *testing.T) {
	mockStore := &mockTokenStore{}
	cfg := &config.TokenConfig{FailThreshold: 3}
	svc := NewTokenService(cfg, mockStore, "https://grok.com")

	// Manually add token
	svc.manager.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 80}})

	token, err := svc.Pick(PoolBasic, "auto")
	require.NoError(t, err)
	assert.Equal(t, uint(1), token.ID)
}

func TestService_PickExcluding(t *testing.T) {
	mockStore := &mockTokenStore{}
	cfg := &config.TokenConfig{FailThreshold: 3}
	svc := NewTokenService(cfg, mockStore, "https://grok.com")

	svc.manager.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 100}})
	svc.manager.AddToken(&store.Token{ID: 2, Token: "t2", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 90}})

	token, err := svc.PickExcluding(PoolBasic, "auto", map[uint]struct{}{1: {}})
	require.NoError(t, err)
	assert.Equal(t, uint(2), token.ID)
}

func TestService_ReportSuccess(t *testing.T) {
	mockStore := &mockTokenStore{}
	cfg := &config.TokenConfig{FailThreshold: 3}
	svc := NewTokenService(cfg, mockStore, "https://grok.com")

	svc.manager.AddToken(&store.Token{
		ID: 1, Token: "t1", Pool: PoolBasic,
		Status: string(StatusActive), Quotas: store.IntMap{"auto": 80},
		FailCount: 2,
	})

	svc.ReportSuccess(1)

	token := svc.manager.GetToken(1)
	assert.Equal(t, string(StatusActive), token.Status)
	assert.Equal(t, 0, token.FailCount)
}

func TestService_ReportRateLimit(t *testing.T) {
	mockStore := &mockTokenStore{}
	cfg := &config.TokenConfig{FailThreshold: 3}
	svc := NewTokenService(cfg, mockStore, "https://grok.com")

	svc.manager.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 80}})

	svc.ReportRateLimit(1, "auto", "rate limited")

	token := svc.manager.GetToken(1)
	assert.Equal(t, 0, token.Quotas["auto"], "ReportRateLimit should clear mode quota to 0")
}

func TestService_ReportError_Recoverable(t *testing.T) {
	mockStore := &mockTokenStore{}
	cfg := &config.TokenConfig{FailThreshold: 3}
	svc := NewTokenService(cfg, mockStore, "https://grok.com")

	svc.manager.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 9}, FailCount: 0})

	svc.ReportError(1, "auto", true, "test error")
	token := svc.manager.GetToken(1)
	assert.Equal(t, 10, token.Quotas["auto"], "recoverable error should refund quota")
	assert.Equal(t, 1, token.FailCount)
}

func TestService_ReportError_NonRecoverable(t *testing.T) {
	mockStore := &mockTokenStore{}
	cfg := &config.TokenConfig{FailThreshold: 3}
	svc := NewTokenService(cfg, mockStore, "https://grok.com")

	svc.manager.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 9}, FailCount: 0})

	svc.ReportError(1, "auto", false, "test error")
	token := svc.manager.GetToken(1)
	assert.Equal(t, 9, token.Quotas["auto"], "non-recoverable error should NOT refund quota")
	assert.Equal(t, 1, token.FailCount)
}

func TestService_FlushDirty(t *testing.T) {
	mockStore := &mockTokenStore{}
	cfg := &config.TokenConfig{FailThreshold: 3}
	svc := NewTokenService(cfg, mockStore, "https://grok.com")

	svc.manager.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 80}})
	svc.manager.AddToken(&store.Token{ID: 2, Token: "t2", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 80}})

	// Clear initial dirty
	dirty := svc.manager.GetDirtyTokens()
	ids := make([]uint, len(dirty))
	for i, d := range dirty {
		ids[i] = d.ID
	}
	svc.manager.ClearDirty(ids)

	// Make changes
	svc.ReportRateLimit(1, "auto", "rate limited")
	svc.ReportError(2, "auto", false, "test error")

	err := svc.FlushDirty(context.Background())
	require.NoError(t, err)

	assert.Len(t, mockStore.updated, 2, "should persist 2 dirty tokens")
}

func TestService_RefreshToken_NotFound(t *testing.T) {
	mockStore := &mockTokenStore{}
	cfg := &config.TokenConfig{FailThreshold: 3}
	svc := NewTokenService(cfg, mockStore, "https://grok.com")

	// Token 999 doesn't exist
	token := svc.manager.GetToken(999)
	assert.Nil(t, token, "non-existent token should return nil")
}

func TestService_Stats(t *testing.T) {
	mockStore := &mockTokenStore{}
	cfg := &config.TokenConfig{FailThreshold: 3}
	svc := NewTokenService(cfg, mockStore, "https://grok.com")

	svc.manager.AddToken(&store.Token{ID: 1, Token: "t1", Pool: PoolBasic, Status: string(StatusActive), Quotas: store.IntMap{"auto": 80}})
	svc.manager.AddToken(&store.Token{ID: 2, Token: "t2", Pool: PoolBasic, Status: string(StatusDisabled), Quotas: store.IntMap{"auto": 60}})
	svc.manager.AddToken(&store.Token{ID: 3, Token: "t3", Pool: PoolSuper, Status: string(StatusActive), Quotas: store.IntMap{"auto": 140}})

	stats := svc.Stats()

	assert.Equal(t, 1, stats[PoolBasic].Active)
	assert.Equal(t, 1, stats[PoolBasic].Disabled)
	assert.Equal(t, 0, stats[PoolBasic].Expired)
	assert.Equal(t, 1, stats[PoolSuper].Active)
}
