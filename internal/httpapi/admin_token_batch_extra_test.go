package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/modelconfig"
	"github.com/crmmc/grokforge/internal/registry"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingNsfwEnabler records upstream enable calls on a channel for deterministic sync.
// (fakeNsfwEnabler already exists in admin_token_test.go with a different shape.)
type recordingNsfwEnabler struct {
	calls chan string
}

func (f *recordingNsfwEnabler) EnableNsfwUpstream(_ context.Context, tokenString string) error {
	f.calls <- tokenString
	return nil
}

func TestHandleBatchImport_Extra(t *testing.T) {
	t.Run("invalid pool fails whole batch", func(t *testing.T) {
		ts := newMockTokenStore()
		resp := handleBatchImport(context.Background(), ts, nil, BatchTokenRequest{
			Operation: BatchOpImport,
			Tokens:    []string{longToken("a_")},
			Pool:      "bogus",
		}, nil)
		assert.Equal(t, 1, resp.Failed)
		require.Len(t, resp.Errors, 1)
		assert.Equal(t, "invalid pool", resp.Errors[0].Message)
	})

	t.Run("registry quota overrides unknown mode", func(t *testing.T) {
		reg := adminTestRegistry()
		ts := newMockTokenStore()
		resp := handleBatchImport(context.Background(), ts, nil, BatchTokenRequest{
			Operation: BatchOpImport,
			Tokens:    []string{longToken("a_")},
			Pool:      "basic",
			Quotas:    store.IntMap{"legacy": 5},
		}, reg)
		assert.Equal(t, 1, resp.Failed)
		assert.Contains(t, resp.Errors[0].Message, "unknown quota mode")
	})

	t.Run("registry quota negative override", func(t *testing.T) {
		reg := adminTestRegistry()
		ts := newMockTokenStore()
		resp := handleBatchImport(context.Background(), ts, nil, BatchTokenRequest{
			Operation: BatchOpImport,
			Tokens:    []string{longToken("a_")},
			Pool:      "basic",
			Quotas:    store.IntMap{"auto": -1},
		}, reg)
		assert.Equal(t, 1, resp.Failed)
		assert.Contains(t, resp.Errors[0].Message, "must be >= 0")
	})

	t.Run("empty and short token strings rejected", func(t *testing.T) {
		ts := newMockTokenStore()
		resp := handleBatchImport(context.Background(), ts, nil, BatchTokenRequest{
			Operation: BatchOpImport,
			Tokens:    []string{"", "short"},
			Pool:      "basic",
		}, nil)
		assert.Equal(t, 2, resp.Failed)
		require.Len(t, resp.Errors, 2)
		assert.Equal(t, "empty token string", resp.Errors[0].Message)
		assert.Contains(t, resp.Errors[1].Message, "token too short")
		assert.Empty(t, ts.tokens)
	})

	t.Run("create error records failure", func(t *testing.T) {
		ets := &errTokenStore{mockTokenStore: newMockTokenStore(), createErr: errors.New("db")}
		resp := handleBatchImport(context.Background(), ets, nil, BatchTokenRequest{
			Operation: BatchOpImport,
			Tokens:    []string{longToken("a_")},
			Pool:      "basic",
		}, nil)
		assert.Equal(t, 1, resp.Failed)
		assert.Contains(t, resp.Errors[0].Message, "failed to create token")
	})

	t.Run("pool sync failure rolls back token", func(t *testing.T) {
		ts := newMockTokenStore()
		syncer := &fakePoolSyncer{addErr: errors.New("pool full")}
		resp := handleBatchImport(context.Background(), ts, syncer, BatchTokenRequest{
			Operation: BatchOpImport,
			Tokens:    []string{longToken("a_")},
			Pool:      "basic",
		}, nil)
		assert.Equal(t, 1, resp.Failed)
		assert.Contains(t, resp.Errors[0].Message, "failed to sync token to pool")
		assert.Empty(t, ts.tokens, "token should be rolled back")
	})

	t.Run("import status and defaults applied", func(t *testing.T) {
		ts := newMockTokenStore()
		syncer := &fakePoolSyncer{}
		nsfw := true
		resp := handleBatchImport(context.Background(), ts, syncer, BatchTokenRequest{
			Operation:   BatchOpImport,
			Tokens:      []string{longToken("a_")},
			Pool:        "super",
			Priority:    4,
			Status:      store.TokenStatusDisabled,
			Remark:      "batch",
			NsfwEnabled: &nsfw,
		}, nil)
		assert.Equal(t, 1, resp.Success)
		require.Len(t, ts.tokens, 1)
		for _, tok := range ts.tokens {
			assert.Equal(t, "ssoSuper", tok.Pool)
			assert.Equal(t, store.TokenStatusDisabled, tok.Status)
			assert.Equal(t, 4, tok.Priority)
			assert.Equal(t, "batch", tok.Remark)
			assert.True(t, tok.NsfwEnabled)
		}
		assert.Len(t, syncer.added, 1)
	})
}

func TestBuildImportQuotaMaps(t *testing.T) {
	reg := registry.NewTestRegistry(
		[]modelconfig.ModelSpec{
			{ID: "m", Type: modelconfig.TypeChat, Enabled: true, PoolFloor: modelconfig.PoolBasic, Mode: "auto", PublicType: "chat"},
		},
		[]modelconfig.ModeSpec{
			{ID: "auto", UpstreamName: "auto", WindowSeconds: 7200, DefaultQuota: map[string]int{"basic": 20, "free": 0}},
		},
	)

	t.Run("nil registry copies overrides", func(t *testing.T) {
		src := store.IntMap{"auto": 3}
		quotas, limits, err := buildImportQuotaMaps("basic", src, nil)
		require.NoError(t, err)
		assert.Equal(t, src, quotas)
		assert.Equal(t, src, limits)
		assert.NotSame(t, &src, &quotas, "must copy, not alias")
	})

	t.Run("registry defaults with zero-limit mode skipped", func(t *testing.T) {
		quotas, limits, err := buildImportQuotaMaps("basic", nil, reg)
		require.NoError(t, err)
		assert.Equal(t, store.IntMap{"auto": 20}, quotas)
		assert.Equal(t, store.IntMap{"auto": 20}, limits)
	})

	t.Run("override within limit", func(t *testing.T) {
		quotas, limits, err := buildImportQuotaMaps("basic", store.IntMap{"auto": 10}, reg)
		require.NoError(t, err)
		assert.Equal(t, store.IntMap{"auto": 10}, quotas)
		assert.Equal(t, store.IntMap{"auto": 20}, limits)
	})

	t.Run("override above limit rejected", func(t *testing.T) {
		_, _, err := buildImportQuotaMaps("basic", store.IntMap{"auto": 999}, reg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds limit")
	})
}

func TestHandleBatchExport_Extra(t *testing.T) {
	t.Run("list error adds batch error", func(t *testing.T) {
		ets := &errTokenStore{mockTokenStore: newMockTokenStore(), listErr: errors.New("db")}
		resp := handleBatchExport(context.Background(), ets, nil, false)
		assert.Equal(t, 0, resp.Success)
		require.Len(t, resp.Errors, 1)
		assert.Equal(t, "failed to list tokens", resp.Errors[0].Message)
	})

	t.Run("missing ids are skipped", func(t *testing.T) {
		ts := newMockTokenStore()
		require.NoError(t, ts.CreateToken(context.Background(), &store.Token{Token: longToken("x_"), Pool: "basic", Status: store.TokenStatusActive}))
		resp := handleBatchExport(context.Background(), ts, []uint{1, 77}, false)
		assert.Equal(t, 1, resp.Success)
		require.Len(t, resp.Tokens, 1)
	})
}

func TestHandleBatchDelete_Extra(t *testing.T) {
	t.Run("delete failure counted per id", func(t *testing.T) {
		ts := newMockTokenStore()
		require.NoError(t, ts.CreateToken(context.Background(), &store.Token{Token: longToken("x_"), Pool: "basic", Status: store.TokenStatusActive}))
		ets := &errTokenStore{mockTokenStore: ts}
		ets.onDeleteToken = func(id uint) {
			if id == 2 {
				// simulate failure for missing id (id 2 doesn't exist anyway)
			}
		}
		syncer := &fakePoolSyncer{}
		resp := handleBatchDelete(context.Background(), ets, syncer, BatchTokenRequest{Operation: BatchOpDelete, IDs: []uint{1, 2}})
		assert.Equal(t, 1, resp.Success)
		assert.Equal(t, 1, resp.Failed)
		require.Len(t, resp.Errors, 1)
		assert.Equal(t, uint(2), resp.Errors[0].ID)
		assert.Equal(t, []uint{1}, syncer.removed)
	})
}

func TestHandleBatchUpdate_Extra(t *testing.T) {
	t.Run("enable and disable statuses", func(t *testing.T) {
		ts := newMockTokenStore()
		require.NoError(t, ts.CreateToken(context.Background(), &store.Token{Token: longToken("x_"), Pool: "basic", Status: store.TokenStatusDisabled}))

		syncer := &fakePoolSyncer{}
		resp := handleBatchUpdate(context.Background(), ts, syncer, BatchTokenRequest{Operation: BatchOpEnable, IDs: []uint{1}})
		assert.Equal(t, 1, resp.Success)
		assert.Equal(t, store.TokenStatusActive, ts.tokens[1].Status)
		assert.Equal(t, []uint{1}, syncer.synced)

		resp = handleBatchUpdate(context.Background(), ts, syncer, BatchTokenRequest{Operation: BatchOpDisable, IDs: []uint{1}})
		assert.Equal(t, 1, resp.Success)
		assert.Equal(t, store.TokenStatusDisabled, ts.tokens[1].Status)
	})

	t.Run("batch update error surfaces message", func(t *testing.T) {
		ets := &errTokenStore{mockTokenStore: newMockTokenStore(), batchErr: errors.New("db")}
		resp := handleBatchUpdate(context.Background(), ets, nil, BatchTokenRequest{Operation: BatchOpEnable, IDs: []uint{1}})
		assert.Equal(t, 0, resp.Success)
		require.Len(t, resp.Errors, 1)
		assert.Contains(t, resp.Errors[0].Message, "batch update failed")
	})

	t.Run("sync error does not fail request", func(t *testing.T) {
		ts := newMockTokenStore()
		require.NoError(t, ts.CreateToken(context.Background(), &store.Token{Token: longToken("x_"), Pool: "basic", Status: store.TokenStatusDisabled}))
		syncer := &fakePoolSyncer{syncErrs: map[uint]error{1: errors.New("sync fail")}}
		resp := handleBatchUpdate(context.Background(), ts, syncer, BatchTokenRequest{Operation: BatchOpEnable, IDs: []uint{1}})
		assert.Equal(t, 1, resp.Success)
	})
}

func TestHandleBatchEnableNsfw_Extra(t *testing.T) {
	t.Run("batch update error surfaces message", func(t *testing.T) {
		ets := &errTokenStore{mockTokenStore: newMockTokenStore(), batchErr: errors.New("db")}
		resp := handleBatchEnableNsfw(context.Background(), ets, nil, BatchTokenRequest{Operation: BatchOpEnableNsfw, IDs: []uint{1}}, nil)
		assert.Equal(t, 0, resp.Success)
		require.Len(t, resp.Errors, 1)
		assert.Contains(t, resp.Errors[0].Message, "batch update failed")
	})

	t.Run("sync error does not fail request", func(t *testing.T) {
		ts := newMockTokenStore()
		require.NoError(t, ts.CreateToken(context.Background(), &store.Token{Token: longToken("x_"), Pool: "basic", Status: store.TokenStatusActive}))
		syncer := &fakePoolSyncer{syncErrs: map[uint]error{1: errors.New("sync fail")}}
		resp := handleBatchEnableNsfw(context.Background(), ts, syncer, BatchTokenRequest{Operation: BatchOpEnableNsfw, IDs: []uint{1}}, nil)
		assert.Equal(t, 1, resp.Success)
	})

	t.Run("async upstream enable fires for stored tokens", func(t *testing.T) {
		ts := newMockTokenStore()
		require.NoError(t, ts.CreateToken(context.Background(), &store.Token{Token: longToken("n1_"), Pool: "basic", Status: store.TokenStatusActive}))
		require.NoError(t, ts.CreateToken(context.Background(), &store.Token{Token: longToken("n2_"), Pool: "basic", Status: store.TokenStatusActive}))
		// ID 99 does not exist: GetToken fails and must be skipped gracefully.
		ids := []uint{1, 2, 99}

		syncer := &fakePoolSyncer{}
		enabler := &recordingNsfwEnabler{calls: make(chan string, len(ids))}
		resp := handleBatchEnableNsfw(context.Background(), ts, syncer, BatchTokenRequest{Operation: BatchOpEnableNsfw, IDs: ids}, enabler)
		assert.Equal(t, 2, resp.Success, "only existing tokens counted by BatchUpdateTokens")

		timeout := time.After(5 * time.Second)
		var enabled []string
		for i := 0; i < 2; i++ {
			select {
			case tok := <-enabler.calls:
				enabled = append(enabled, tok)
			case <-timeout:
				t.Fatalf("timed out waiting for upstream enable calls; got %v", enabled)
			}
		}
		assert.ElementsMatch(t, []string{longToken("n1_"), longToken("n2_")}, enabled)

		for _, id := range ids {
			assert.Contains(t, syncer.synced, id)
		}
	})
}

func TestHandleBatchTokens_UnknownOperationAndStrictDecode(t *testing.T) {
	t.Run("unknown operation returns 400", func(t *testing.T) {
		w := httptest.NewRecorder()
		body := `{"operation":"explode"}`
		handleBatchTokens(newMockTokenStore(), nil, nil, nil)(w, httptest.NewRequest(http.MethodPost, "/admin/tokens/batch", strings.NewReader(body)))
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid_operation")
	})

	t.Run("unknown json field rejected", func(t *testing.T) {
		w := httptest.NewRecorder()
		body := `{"operation":"import","legacy_field":1}`
		handleBatchTokens(newMockTokenStore(), nil, nil, nil)(w, httptest.NewRequest(http.MethodPost, "/admin/tokens/batch", strings.NewReader(body)))
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("malformed json returns 400", func(t *testing.T) {
		w := httptest.NewRecorder()
		handleBatchTokens(newMockTokenStore(), nil, nil, nil)(w, httptest.NewRequest(http.MethodPost, "/admin/tokens/batch", strings.NewReader("{")))
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestPtrHelpers(t *testing.T) {
	assert.Equal(t, "x", *ptrString("x"))
	assert.True(t, *ptrBool(true))
}

func TestCopyIntMap(t *testing.T) {
	assert.Nil(t, copyIntMap(nil))
	src := store.IntMap{"a": 1}
	dst := copyIntMap(src)
	assert.Equal(t, src, dst)
	dst["a"] = 2
	assert.Equal(t, 1, src["a"], "copy must be independent")
}

// errOnceNsfwEnabler records calls and always fails upstream enable.
type errOnceNsfwEnabler struct{ calls chan string }

func (f *errOnceNsfwEnabler) EnableNsfwUpstream(_ context.Context, tokenString string) error {
	f.calls <- tokenString
	return errors.New("upstream rejected")
}

func TestHandleBatchEnableNsfw_UpstreamErrorIsLoggedNotFatal(t *testing.T) {
	ts := newMockTokenStore()
	require.NoError(t, ts.CreateToken(context.Background(), &store.Token{Token: longToken("e_"), Pool: "basic", Status: store.TokenStatusActive}))

	enabler := &errOnceNsfwEnabler{calls: make(chan string, 1)}
	resp := handleBatchEnableNsfw(context.Background(), ts, nil, BatchTokenRequest{Operation: BatchOpEnableNsfw, IDs: []uint{1}}, enabler)
	assert.Equal(t, 1, resp.Success)

	select {
	case <-enabler.calls:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for upstream enable call")
	}
}
