package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/modelconfig"
	"github.com/crmmc/grokforge/internal/registry"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errTokenStore wraps mockTokenStore with per-method error injection.
type errTokenStore struct {
	*mockTokenStore

	listErr          error
	listFilteredErr  error
	listIDsErr       error
	getErr           error
	createErr        error
	updateErr        error
	deleteErr        error
	batchErr         error
	onListTokens     func()
	onListFiltered   func(filter store.TokenFilter)
	onListTokenIDs   func(filter store.TokenFilter)
	onGetToken       func(id uint)
	onCreateToken    func(t *store.Token)
	onUpdateToken    func(t *store.Token)
	onDeleteToken    func(id uint)
	onBatchUpdate    func(req store.BatchUpdateRequest)
	batchUpdateCount int
}

func (e *errTokenStore) ListTokens(ctx context.Context) ([]*store.Token, error) {
	if e.onListTokens != nil {
		e.onListTokens()
	}
	if e.listErr != nil {
		return nil, e.listErr
	}
	return e.mockTokenStore.ListTokens(ctx)
}

func (e *errTokenStore) ListTokensFiltered(ctx context.Context, filter store.TokenFilter) ([]*store.Token, error) {
	if e.onListFiltered != nil {
		e.onListFiltered(filter)
	}
	if e.listFilteredErr != nil {
		return nil, e.listFilteredErr
	}
	return e.mockTokenStore.ListTokensFiltered(ctx, filter)
}

func (e *errTokenStore) ListTokenIDs(ctx context.Context, filter store.TokenFilter) ([]uint, error) {
	if e.onListTokenIDs != nil {
		e.onListTokenIDs(filter)
	}
	if e.listIDsErr != nil {
		return nil, e.listIDsErr
	}
	return e.mockTokenStore.ListTokenIDs(ctx, filter)
}

func (e *errTokenStore) GetToken(ctx context.Context, id uint) (*store.Token, error) {
	if e.onGetToken != nil {
		e.onGetToken(id)
	}
	if e.getErr != nil {
		return nil, e.getErr
	}
	return e.mockTokenStore.GetToken(ctx, id)
}

func (e *errTokenStore) CreateToken(ctx context.Context, token *store.Token) error {
	if e.onCreateToken != nil {
		e.onCreateToken(token)
	}
	if e.createErr != nil {
		return e.createErr
	}
	return e.mockTokenStore.CreateToken(ctx, token)
}

func (e *errTokenStore) UpdateToken(ctx context.Context, token *store.Token) error {
	if e.onUpdateToken != nil {
		e.onUpdateToken(token)
	}
	if e.updateErr != nil {
		return e.updateErr
	}
	return e.mockTokenStore.UpdateToken(ctx, token)
}

func (e *errTokenStore) DeleteToken(ctx context.Context, id uint) error {
	if e.onDeleteToken != nil {
		e.onDeleteToken(id)
	}
	if e.deleteErr != nil {
		return e.deleteErr
	}
	return e.mockTokenStore.DeleteToken(ctx, id)
}

func (e *errTokenStore) BatchUpdateTokens(ctx context.Context, req store.BatchUpdateRequest) (int, error) {
	e.batchUpdateCount++
	if e.onBatchUpdate != nil {
		e.onBatchUpdate(req)
	}
	if e.batchErr != nil {
		return 0, e.batchErr
	}
	return e.mockTokenStore.BatchUpdateTokens(ctx, req)
}

// fakePoolSyncer records pool sync calls with optional error injection.
type fakePoolSyncer struct {
	added    []*store.Token
	removed  []uint
	synced   []uint
	addErr   error
	syncErrs map[uint]error
}

func (f *fakePoolSyncer) AddToPool(token *store.Token) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.added = append(f.added, token)
	return nil
}

func (f *fakePoolSyncer) RemoveFromPool(id uint) {
	f.removed = append(f.removed, id)
}

func (f *fakePoolSyncer) SyncToken(_ context.Context, id uint) error {
	f.synced = append(f.synced, id)
	if f.syncErrs != nil {
		if err, ok := f.syncErrs[id]; ok {
			return err
		}
	}
	return nil
}

// fakeInflight returns configurable inflight counts.
type fakeInflight struct {
	counts map[uint]int
}

func (f *fakeInflight) GetInflight(id uint) int { return f.counts[id] }

// errRefresher wraps a fixed result for TokenRefresher.
type errRefresher struct {
	token *store.Token
	err   error
	calls []uint
}

func (r *errRefresher) RefreshToken(_ context.Context, id uint) (*store.Token, error) {
	r.calls = append(r.calls, id)
	if r.err != nil {
		return nil, r.err
	}
	return r.token, nil
}

func withID(t *testing.T, r *http.Request, id string) *http.Request {
	return withIDParam(t, r, id)
}

func longToken(prefix string) string {
	return prefix + "abcdefghijklmnopqrstuvwxyz0123456789"
}

func TestHandleListTokens_Extra(t *testing.T) {
	t.Run("status exhausted filters derived display status", func(t *testing.T) {
		ts := newMockTokenStore()
		require.NoError(t, ts.CreateToken(context.Background(), &store.Token{
			ID:     1,
			Token:  longToken("t1_"),
			Pool:   "ssoBasic",
			Status: store.TokenStatusActive,
			Quotas: store.IntMap{"auto": 0},
		}))
		require.NoError(t, ts.CreateToken(context.Background(), &store.Token{
			ID:     2,
			Token:  longToken("t2_"),
			Pool:   "ssoBasic",
			Status: store.TokenStatusActive,
			Quotas: store.IntMap{"auto": 5},
		}))

		w := httptest.NewRecorder()
		handleListTokens(ts, nil, &fakeInflight{})(w, httptest.NewRequest(http.MethodGet, "/admin/tokens?status=exhausted", nil))

		require.Equal(t, http.StatusOK, w.Code)
		var resp PaginatedTokenResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, 1, resp.Total)
		require.Len(t, resp.Data, 1)
		assert.Equal(t, uint(1), resp.Data[0].ID)
		assert.Equal(t, "exhausted", resp.Data[0].DisplayStatus)
	})

	t.Run("nsfw filter valid and invalid", func(t *testing.T) {
		ts := newMockTokenStore()
		require.NoError(t, ts.CreateToken(context.Background(), &store.Token{Token: longToken("n1_"), Pool: "ssoBasic", Status: store.TokenStatusActive, NsfwEnabled: true}))

		w := httptest.NewRecorder()
		handleListTokens(ts, nil, nil)(w, httptest.NewRequest(http.MethodGet, "/admin/tokens?nsfw=true", nil))
		require.Equal(t, http.StatusOK, w.Code)
		var resp PaginatedTokenResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, 1, resp.Total)

		w = httptest.NewRecorder()
		handleListTokens(ts, nil, nil)(w, httptest.NewRequest(http.MethodGet, "/admin/tokens?nsfw=maybe", nil))
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("page beyond total returns empty data", func(t *testing.T) {
		ts := newMockTokenStore()
		require.NoError(t, ts.CreateToken(context.Background(), &store.Token{Token: longToken("p1_"), Pool: "ssoBasic", Status: store.TokenStatusActive}))

		w := httptest.NewRecorder()
		handleListTokens(ts, nil, nil)(w, httptest.NewRequest(http.MethodGet, "/admin/tokens?page=5&page_size=10", nil))
		require.Equal(t, http.StatusOK, w.Code)
		var resp PaginatedTokenResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, 1, resp.Total)
		assert.Empty(t, resp.Data)
	})

	t.Run("list error returns 500", func(t *testing.T) {
		ts := &errTokenStore{mockTokenStore: newMockTokenStore(), listErr: errors.New("db")}
		w := httptest.NewRecorder()
		handleListTokens(ts, nil, nil)(w, httptest.NewRequest(http.MethodGet, "/admin/tokens", nil))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("filtered list error returns 500", func(t *testing.T) {
		ts := &errTokenStore{mockTokenStore: newMockTokenStore(), listFilteredErr: errors.New("db")}
		w := httptest.NewRecorder()
		handleListTokens(ts, nil, nil)(w, httptest.NewRequest(http.MethodGet, "/admin/tokens?status=active", nil))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("pagination params and inflight included", func(t *testing.T) {
		ts := newMockTokenStore()
		require.NoError(t, ts.CreateToken(context.Background(), &store.Token{ID: 3, Token: longToken("i1_"), Pool: "ssoBasic", Status: store.TokenStatusActive}))

		w := httptest.NewRecorder()
		ip := &fakeInflight{counts: map[uint]int{1: 4}}
		handleListTokens(ts, nil, ip)(w, httptest.NewRequest(http.MethodGet, "/admin/tokens?page=1&page_size=1&status=active", nil))
		require.Equal(t, http.StatusOK, w.Code)
		var resp PaginatedTokenResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		require.Len(t, resp.Data, 1)
		assert.Equal(t, 4, resp.Data[0].Inflight)
	})
}

func TestHandleGetToken_Extra(t *testing.T) {
	ts := newMockTokenStore()
	require.NoError(t, ts.CreateToken(context.Background(), &store.Token{ID: 1, Token: longToken("g1_"), Pool: "ssoBasic", Status: store.TokenStatusActive}))

	tests := []struct {
		name       string
		id         string
		getErr     error
		wantStatus int
	}{
		{name: "invalid id", id: "abc", wantStatus: http.StatusBadRequest},
		{name: "not found", id: "42", wantStatus: http.StatusNotFound},
		{name: "store error", id: "1", getErr: errors.New("boom"), wantStatus: http.StatusInternalServerError},
		{name: "success", id: "1", wantStatus: http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ets := &errTokenStore{mockTokenStore: ts, getErr: tc.getErr}
			w := httptest.NewRecorder()
			handleGetToken(ets, nil, &fakeInflight{})(w, withID(t, httptest.NewRequest(http.MethodGet, "/admin/tokens/1", nil), tc.id))
			require.Equal(t, tc.wantStatus, w.Code)
		})
	}
}

func TestHandleUpdateToken_Extra(t *testing.T) {
	baseToken := func() *store.Token {
		return &store.Token{ID: 1, Token: longToken("u1_"), Pool: "ssoBasic", Status: store.TokenStatusActive}
	}

	tests := []struct {
		name       string
		id         string
		body       string
		getErr     error
		updateErr  error
		wantStatus int
	}{
		{name: "invalid id", id: "abc", body: "{}", wantStatus: http.StatusBadRequest},
		{name: "not found", id: "99", body: "{}", wantStatus: http.StatusNotFound},
		{name: "get error", id: "1", body: "{}", getErr: errors.New("boom"), wantStatus: http.StatusInternalServerError},
		{name: "invalid json", id: "1", body: "{bad", wantStatus: http.StatusBadRequest},
		{name: "remark too long", id: "1", body: `{"remark":"` + strings.Repeat("x", 501) + `"}`, wantStatus: http.StatusBadRequest},
		{name: "invalid status", id: "1", body: `{"status":"bogus"}`, wantStatus: http.StatusBadRequest},
		{name: "unknown mode without registry", id: "1", body: `{"quotas":{"nosuch":1}}`, wantStatus: http.StatusBadRequest},
		{name: "negative quota", id: "1", body: `{"quotas":{"auto":-1}}`, wantStatus: http.StatusBadRequest},
		{name: "update error", id: "1", body: "{}", updateErr: errors.New("boom"), wantStatus: http.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := newMockTokenStore()
			require.NoError(t, ts.CreateToken(context.Background(), baseToken()))
			ets := &errTokenStore{mockTokenStore: ts, getErr: tc.getErr, updateErr: tc.updateErr}

			w := httptest.NewRecorder()
			handleUpdateToken(ets, &fakePoolSyncer{}, nil, &fakeInflight{})(w, withID(t, httptest.NewRequest(http.MethodPut, "/admin/tokens/1", strings.NewReader(tc.body)), tc.id))
			require.Equal(t, tc.wantStatus, w.Code)
		})
	}

	t.Run("unknown mode with registry", func(t *testing.T) {
		ts := newMockTokenStore()
		require.NoError(t, ts.CreateToken(context.Background(), baseToken()))
		w := httptest.NewRecorder()
		handleUpdateToken(ts, nil, adminTestRegistry(), nil)(w, withID(t, httptest.NewRequest(http.MethodPut, "/admin/tokens/1", strings.NewReader(`{"quotas":{"legacy":1}}`)), "1"))
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("quota exceeds limit without registry", func(t *testing.T) {
		ts := newMockTokenStore()
		tok := baseToken()
		tok.LimitQuotas = store.IntMap{"auto": 5}
		require.NoError(t, ts.CreateToken(context.Background(), tok))
		w := httptest.NewRecorder()
		handleUpdateToken(ts, nil, nil, nil)(w, withID(t, httptest.NewRequest(http.MethodPut, "/admin/tokens/1", strings.NewReader(`{"quotas":{"auto":6}}`)), "1"))
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("full update applies fields and syncs pool", func(t *testing.T) {
		ts := newMockTokenStore()
		tok := baseToken()
		tok.LimitQuotas = store.IntMap{"auto": 100}
		require.NoError(t, ts.CreateToken(context.Background(), tok))

		syncer := &fakePoolSyncer{}
		body := `{"status":"disabled","quotas":{"auto":50},"remark":"note","nsfw_enabled":true,"priority":9}`
		w := httptest.NewRecorder()
		handleUpdateToken(ts, syncer, nil, &fakeInflight{})(w, withID(t, httptest.NewRequest(http.MethodPut, "/admin/tokens/1", strings.NewReader(body)), "1"))

		require.Equal(t, http.StatusOK, w.Code)
		updated := ts.tokens[1]
		assert.Equal(t, store.TokenStatusDisabled, updated.Status)
		assert.Equal(t, "manual disable", updated.StatusReason)
		assert.Equal(t, 50, updated.Quotas["auto"])
		assert.Equal(t, "note", updated.Remark)
		assert.True(t, updated.NsfwEnabled)
		assert.Equal(t, 9, updated.Priority)
		assert.Equal(t, []uint{1}, syncer.synced)

		var resp TokenResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "disabled", resp.DisplayStatus)
	})

	t.Run("active status clears reason and nil quotas map is created", func(t *testing.T) {
		ts := newMockTokenStore()
		tok := baseToken()
		tok.Status = store.TokenStatusDisabled
		tok.StatusReason = "manual disable"
		tok.LimitQuotas = store.IntMap{"auto": 100}
		require.NoError(t, ts.CreateToken(context.Background(), tok))

		body := `{"status":"active","quotas":{"auto":7}}`
		w := httptest.NewRecorder()
		handleUpdateToken(ts, nil, nil, nil)(w, withID(t, httptest.NewRequest(http.MethodPut, "/admin/tokens/1", strings.NewReader(body)), "1"))

		require.Equal(t, http.StatusOK, w.Code)
		updated := ts.tokens[1]
		assert.Equal(t, "", updated.StatusReason)
		assert.Equal(t, 7, updated.Quotas["auto"])
	})

	t.Run("pool sync error still returns 200", func(t *testing.T) {
		ts := newMockTokenStore()
		require.NoError(t, ts.CreateToken(context.Background(), baseToken()))
		syncer := &fakePoolSyncer{syncErrs: map[uint]error{1: errors.New("sync fail")}}
		w := httptest.NewRecorder()
		handleUpdateToken(ts, syncer, nil, nil)(w, withID(t, httptest.NewRequest(http.MethodPut, "/admin/tokens/1", strings.NewReader(`{}`)), "1"))
		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestHandleListTokenIDs_Extra(t *testing.T) {
	t.Run("no filter lists all ids", func(t *testing.T) {
		ts := newMockTokenStore()
		require.NoError(t, ts.CreateToken(context.Background(), &store.Token{ID: 5, Token: longToken("a_"), Status: store.TokenStatusActive}))
		w := httptest.NewRecorder()
		handleListTokenIDs(ts, nil)(w, httptest.NewRequest(http.MethodGet, "/admin/tokens/ids", nil))
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string][]uint
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, []uint{1}, resp["ids"])
	})

	t.Run("plain status filter", func(t *testing.T) {
		ts := &errTokenStore{mockTokenStore: newMockTokenStore()}
		ts.onListTokenIDs = func(filter store.TokenFilter) {
			require.NotNil(t, filter.Status)
			assert.Equal(t, "disabled", *filter.Status)
		}
		w := httptest.NewRecorder()
		handleListTokenIDs(ts, nil)(w, httptest.NewRequest(http.MethodGet, "/admin/tokens/ids?status=disabled", nil))
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("exhausted status filters by display status", func(t *testing.T) {
		ts := newMockTokenStore()
		require.NoError(t, ts.CreateToken(context.Background(), &store.Token{ID: 1, Token: longToken("e_"), Status: store.TokenStatusActive, Quotas: store.IntMap{"auto": 0}}))
		require.NoError(t, ts.CreateToken(context.Background(), &store.Token{ID: 2, Token: longToken("f_"), Status: store.TokenStatusActive, Quotas: store.IntMap{"auto": 3}}))
		w := httptest.NewRecorder()
		handleListTokenIDs(ts, nil)(w, httptest.NewRequest(http.MethodGet, "/admin/tokens/ids?status=exhausted", nil))
		require.Equal(t, http.StatusOK, w.Code)
		var resp map[string][]uint
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, []uint{1}, resp["ids"])
	})

	t.Run("exhausted list error returns 500", func(t *testing.T) {
		ts := &errTokenStore{mockTokenStore: newMockTokenStore(), listFilteredErr: errors.New("db")}
		w := httptest.NewRecorder()
		handleListTokenIDs(ts, nil)(w, httptest.NewRequest(http.MethodGet, "/admin/tokens/ids?status=exhausted", nil))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("ids list error returns 500", func(t *testing.T) {
		ts := &errTokenStore{mockTokenStore: newMockTokenStore(), listIDsErr: errors.New("db")}
		w := httptest.NewRecorder()
		handleListTokenIDs(ts, nil)(w, httptest.NewRequest(http.MethodGet, "/admin/tokens/ids", nil))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandleDeleteToken_Extra(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		deleteErr  error
		wantStatus int
	}{
		{name: "invalid id", id: "abc", wantStatus: http.StatusBadRequest},
		{name: "not found", id: "42", wantStatus: http.StatusNotFound},
		{name: "store error", id: "1", deleteErr: errors.New("boom"), wantStatus: http.StatusInternalServerError},
		{name: "success", id: "1", wantStatus: http.StatusNoContent},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ts := newMockTokenStore()
			require.NoError(t, ts.CreateToken(context.Background(), &store.Token{ID: 1, Token: longToken("d_"), Status: store.TokenStatusActive}))
			ets := &errTokenStore{mockTokenStore: ts, deleteErr: tc.deleteErr}

			syncer := &fakePoolSyncer{}
			w := httptest.NewRecorder()
			handleDeleteToken(ets, syncer)(w, withID(t, httptest.NewRequest(http.MethodDelete, "/admin/tokens/1", nil), tc.id))
			require.Equal(t, tc.wantStatus, w.Code)
			if tc.wantStatus == http.StatusNoContent {
				assert.Equal(t, []uint{1}, syncer.removed)
			}
		})
	}
}

func TestHandleRefreshToken_Extra(t *testing.T) {
	t.Run("nil refresher returns 501", func(t *testing.T) {
		w := httptest.NewRecorder()
		handleRefreshToken(newMockTokenStore(), nil, nil, nil)(w, withID(t, httptest.NewRequest(http.MethodPost, "/admin/tokens/1/refresh", nil), "1"))
		assert.Equal(t, http.StatusNotImplemented, w.Code)
	})

	t.Run("invalid id returns 400", func(t *testing.T) {
		w := httptest.NewRecorder()
		handleRefreshToken(newMockTokenStore(), &errRefresher{}, nil, nil)(w, withID(t, httptest.NewRequest(http.MethodPost, "/admin/tokens/1/refresh", nil), "abc"))
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("not found returns 404", func(t *testing.T) {
		ref := &errRefresher{err: store.ErrNotFound}
		w := httptest.NewRecorder()
		handleRefreshToken(newMockTokenStore(), ref, nil, nil)(w, withID(t, httptest.NewRequest(http.MethodPost, "/admin/tokens/1/refresh", nil), "1"))
		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("refresh failure returns 502 without leaking error", func(t *testing.T) {
		ref := &errRefresher{err: errors.New("upstream secret")}
		w := httptest.NewRecorder()
		handleRefreshToken(newMockTokenStore(), ref, nil, nil)(w, withID(t, httptest.NewRequest(http.MethodPost, "/admin/tokens/1/refresh", nil), "1"))
		require.Equal(t, http.StatusBadGateway, w.Code)
		assert.Contains(t, w.Body.String(), tokenRefreshFailedMessage)
		assert.NotContains(t, w.Body.String(), "upstream secret")
	})

	t.Run("nil token falls back to store", func(t *testing.T) {
		ts := newMockTokenStore()
		require.NoError(t, ts.CreateToken(context.Background(), &store.Token{ID: 1, Token: longToken("r_"), Pool: "ssoBasic", Status: store.TokenStatusActive, Quotas: store.IntMap{"auto": 5}}))
		ref := &errRefresher{token: nil}
		w := httptest.NewRecorder()
		handleRefreshToken(ts, ref, nil, &fakeInflight{})(w, withID(t, httptest.NewRequest(http.MethodPost, "/admin/tokens/1/refresh", nil), "1"))
		require.Equal(t, http.StatusOK, w.Code)
		var resp TokenResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, uint(1), resp.ID)
	})

	t.Run("fallback store error returns 500", func(t *testing.T) {
		ts := &errTokenStore{mockTokenStore: newMockTokenStore(), getErr: errors.New("db")}
		ref := &errRefresher{token: nil}
		w := httptest.NewRecorder()
		handleRefreshToken(ts, ref, nil, nil)(w, withID(t, httptest.NewRequest(http.MethodPost, "/admin/tokens/1/refresh", nil), "1"))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("refreshed token returned directly", func(t *testing.T) {
		ref := &errRefresher{token: &store.Token{ID: 9, Token: longToken("z_"), Pool: "ssoBasic", Status: store.TokenStatusActive}}
		w := httptest.NewRecorder()
		handleRefreshToken(newMockTokenStore(), ref, nil, nil)(w, withID(t, httptest.NewRequest(http.MethodPost, "/admin/tokens/1/refresh", nil), "9"))
		require.Equal(t, http.StatusOK, w.Code)
		var resp TokenResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, uint(9), resp.ID)
	})
}

func TestDeriveDisplayStatus_Extra(t *testing.T) {
	reg := registry.NewTestRegistry(
		[]modelconfig.ModelSpec{
			{ID: "grok-4.20", Type: modelconfig.TypeChat, Enabled: true, PoolFloor: modelconfig.PoolBasic, Mode: "auto", PublicType: "chat"},
		},
		[]modelconfig.ModeSpec{
			{ID: "auto", UpstreamName: "auto", WindowSeconds: 7200, DefaultQuota: map[string]int{"basic": 20}},
		},
	)

	tests := []struct {
		name string
		tok  *store.Token
		reg  *registry.ModelRegistry
		want string
	}{
		{name: "disabled wins", tok: &store.Token{Status: store.TokenStatusDisabled, Quotas: store.IntMap{"auto": 5}}, want: "disabled"},
		{name: "expired wins", tok: &store.Token{Status: store.TokenStatusExpired}, want: "expired"},
		{name: "nil registry with remaining quota", tok: &store.Token{Status: store.TokenStatusActive, Quotas: store.IntMap{"auto": 5}}, want: "active"},
		{name: "nil registry all zero quotas", tok: &store.Token{Status: store.TokenStatusActive, Quotas: store.IntMap{"auto": 0}}, want: "exhausted"},
		{name: "nil registry empty quotas", tok: &store.Token{Status: store.TokenStatusActive}, want: "active"},
		{name: "registry with no supported modes for pool", tok: &store.Token{Status: store.TokenStatusActive, Pool: "unknownPool"}, reg: reg, want: "active"},
		{name: "registry supported mode remaining", tok: &store.Token{Status: store.TokenStatusActive, Pool: "ssoBasic", Quotas: store.IntMap{"auto": 1}}, reg: reg, want: "active"},
		{name: "registry supported mode exhausted", tok: &store.Token{Status: store.TokenStatusActive, Pool: "ssoBasic", Quotas: store.IntMap{"auto": 0}}, reg: reg, want: "exhausted"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, deriveDisplayStatus(tc.tok, tc.reg))
		})
	}
}

func TestSupportedModesAndFilterHelpers(t *testing.T) {
	reg := adminTestRegistry()

	t.Run("supportedModesForPool nil registry", func(t *testing.T) {
		assert.Nil(t, supportedModesForPool(nil, "ssoBasic"))
	})

	t.Run("supportedModesForPool maps mode ids", func(t *testing.T) {
		modes := supportedModesForPool(reg, "ssoBasic")
		assert.Equal(t, []string{"auto"}, modes)
	})

	t.Run("knownModeLimits nil registry returns limits as-is", func(t *testing.T) {
		tok := &store.Token{LimitQuotas: store.IntMap{"auto": 5, "legacy": 1}}
		assert.Equal(t, tok.LimitQuotas, knownModeLimits(tok, nil))
	})

	t.Run("knownModeLimits filters with registry", func(t *testing.T) {
		tok := &store.Token{LimitQuotas: store.IntMap{"auto": 5, "legacy": 1}}
		filtered := knownModeLimits(tok, reg)
		assert.Equal(t, store.IntMap{"auto": 5}, filtered)
	})

	t.Run("filterModeMap nil source or nil registry returns src", func(t *testing.T) {
		src := store.IntMap{"auto": 1, "legacy": 2}
		assert.Nil(t, filterModeMap(nil, reg))
		assert.Equal(t, src, filterModeMap(src, nil))
	})
}

func TestTokenToResponse_MaskingAndInflight(t *testing.T) {
	tok := &store.Token{
		ID:       3,
		Token:    longToken("m_"),
		Pool:     "ssoBasic",
		Status:   store.TokenStatusActive,
		Quotas:   store.IntMap{"auto": 1},
		Remark:   "r",
		Priority: 2,
	}
	resp := tokenToResponse(tok, nil, 6)
	assert.Equal(t, maskSecret(tok.Token), resp.Token)
	assert.Equal(t, 6, resp.Inflight)
	assert.Equal(t, "r", resp.Remark)
	assert.Equal(t, 2, resp.Priority)
	assert.NotEqual(t, tok.Token, resp.Token)
}

func TestSafeGetInflight_NilProvider(t *testing.T) {
	assert.Equal(t, 0, safeGetInflight(nil, 1))
}

func TestFilterModeMap_SkipsUnknownModes(t *testing.T) {
	reg := adminTestRegistry()
	// src contains only "legacy"; the registry's "auto" mode is absent from src
	// so the loop takes the skip branch.
	assert.Empty(t, filterModeMap(store.IntMap{"legacy": 1}, reg))
}
