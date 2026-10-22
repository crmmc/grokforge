package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/crmmc/grokforge/internal/store"
)

// withIDParam injects a chi URL "id" parameter into the request, mirroring
// what the chi router does for /apikeys/{id} routes.
func withIDParam(t *testing.T, r *http.Request, id string) *http.Request {
	t.Helper()
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestHandleListAPIKeys_Extra(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		listErr    error
		wantStatus int
		wantPage   int
		wantSize   int
		wantTotal  int64
		wantPages  int
	}{
		{name: "list error returns 500", query: "", listErr: errors.New("db down"), wantStatus: http.StatusInternalServerError},
		{name: "defaults applied when no params", query: "", wantStatus: http.StatusOK, wantPage: 1, wantSize: 20, wantPages: 1},
		{name: "valid page params", query: "?page=2&page_size=1", wantStatus: http.StatusOK, wantPage: 2, wantSize: 1, wantPages: 2},
		{name: "invalid page falls back to default", query: "?page=abc", wantStatus: http.StatusOK, wantPage: 1, wantSize: 20, wantPages: 1},
		{name: "zero page falls back to default", query: "?page=0", wantStatus: http.StatusOK, wantPage: 1, wantSize: 20, wantPages: 1},
		{name: "page_size above 100 falls back to default", query: "?page_size=500", wantStatus: http.StatusOK, wantPage: 1, wantSize: 20, wantPages: 1},
		{name: "page_size zero falls back to default", query: "?page_size=0", wantStatus: http.StatusOK, wantPage: 1, wantSize: 20, wantPages: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ms := newMockAPIKeyStore()
			require.NoError(t, ms.Create(context.Background(), storeAPIKeyForTest("k1")))
			require.NoError(t, ms.Create(context.Background(), storeAPIKeyForTest("k2")))
			ms.listErr = tc.listErr

			req := httptest.NewRequest(http.MethodGet, "/admin/apikeys"+tc.query, nil)
			w := httptest.NewRecorder()
			handleListAPIKeys(ms)(w, req)

			require.Equal(t, tc.wantStatus, w.Code)
			if tc.wantStatus != http.StatusOK {
				return
			}

			var resp PaginatedResponse
			require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
			assert.Equal(t, tc.wantPage, resp.Page)
			assert.Equal(t, tc.wantSize, resp.PageSize)
			assert.Equal(t, int64(2), resp.Total)
			assert.Equal(t, tc.wantPages, resp.TotalPages)
		})
	}
}

func storeAPIKeyForTest(name string) *store.APIKey {
	return &store.APIKey{Name: name, Key: "gf-key-" + name}
}

func TestHandleListAPIKeys_StatusFilterAndTotalPages(t *testing.T) {
	ms := newMockAPIKeyStore()
	require.NoError(t, ms.Create(context.Background(), &store.APIKey{Name: "a", Status: "active"}))
	require.NoError(t, ms.Create(context.Background(), &store.APIKey{Name: "b", Status: "inactive"}))

	req := httptest.NewRequest(http.MethodGet, "/admin/apikeys?status=inactive&page_size=1", nil)
	w := httptest.NewRecorder()
	handleListAPIKeys(ms)(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var resp PaginatedResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, int64(1), resp.Total)
	assert.Equal(t, 1, resp.TotalPages)
	keys, ok := resp.Data.([]any)
	require.True(t, ok)
	require.Len(t, keys, 1)
}

func TestHandleGetAPIKey_Extra(t *testing.T) {
	ms := newMockAPIKeyStore()
	require.NoError(t, ms.Create(context.Background(), &store.APIKey{Name: "found", Key: "gf-long-key-value"}))

	tests := []struct {
		name       string
		id         string
		getErr     error
		wantStatus int
	}{
		{name: "invalid id returns 400", id: "abc", wantStatus: http.StatusBadRequest},
		{name: "not found returns 404", id: "999", wantStatus: http.StatusNotFound},
		{name: "generic store error returns 500", id: "1", getErr: errors.New("boom"), wantStatus: http.StatusInternalServerError},
		{name: "success returns masked key", id: "1", wantStatus: http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ms.getByIDErr = tc.getErr
			req := withIDParam(t, httptest.NewRequest(http.MethodGet, "/admin/apikeys/1", nil), tc.id)
			w := httptest.NewRecorder()
			handleGetAPIKey(ms)(w, req)

			require.Equal(t, tc.wantStatus, w.Code)
			if tc.wantStatus != http.StatusOK {
				return
			}
			var resp APIKeyResponse
			require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
			assert.Equal(t, maskKey("gf-mock1234567890abcdef1234567890abcdef1234567890ab"), resp.Key)
		})
	}
}

func TestHandleCreateAPIKey_Extra(t *testing.T) {
	t.Run("invalid json returns 400", func(t *testing.T) {
		w := httptest.NewRecorder()
		handleCreateAPIKey(newMockAPIKeyStore())(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{bad")))
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid expires_at returns 400", func(t *testing.T) {
		body := `{"name":"k","expires_at":"not-a-time"}`
		w := httptest.NewRecorder()
		handleCreateAPIKey(newMockAPIKeyStore())(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("create error returns 500", func(t *testing.T) {
		ms := newMockAPIKeyStore()
		ms.createErr = errors.New("db down")
		w := httptest.NewRecorder()
		handleCreateAPIKey(ms)(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"k"}`)))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("full payload success", func(t *testing.T) {
		ms := newMockAPIKeyStore()
		body := `{"name":"k","model_whitelist":["m1"],"rate_limit":10,"daily_limit":100,"expires_at":"2030-01-02T03:04:05Z"}`
		w := httptest.NewRecorder()
		handleCreateAPIKey(ms)(w, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body)))

		require.Equal(t, http.StatusCreated, w.Code)
		var resp APIKeyCreateResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "k", resp.Name)
		require.Len(t, ms.keys, 1)
		created := ms.keys[0]
		assert.Equal(t, 10, created.RateLimit)
		assert.Equal(t, 100, created.DailyLimit)
		require.NotNil(t, created.ExpiresAt)
		assert.Equal(t, "m1", string(created.ModelWhitelist[0]))
	})
}

func TestHandleUpdateAPIKey_Extra(t *testing.T) {
	newStoreWithKey := func(t *testing.T) *mockAPIKeyStore {
		t.Helper()
		ms := newMockAPIKeyStore()
		require.NoError(t, ms.Create(context.Background(), &store.APIKey{Name: "old", Key: "gf-key-old", Status: "active"}))
		return ms
	}

	tests := []struct {
		name       string
		id         string
		body       string
		getErr     error
		updateErr  error
		wantStatus int
	}{
		{name: "invalid id returns 400", id: "abc", body: "{}", wantStatus: http.StatusBadRequest},
		{name: "not found returns 404", id: "999", body: "{}", wantStatus: http.StatusNotFound},
		{name: "get error returns 500", id: "1", body: "{}", getErr: errors.New("boom"), wantStatus: http.StatusInternalServerError},
		{name: "invalid json returns 400", id: "1", body: "{bad", wantStatus: http.StatusBadRequest},
		{name: "invalid expires_at returns 400", id: "1", body: `{"expires_at":"nope"}`, wantStatus: http.StatusBadRequest},
		{name: "update error returns 500", id: "1", body: "{}", updateErr: errors.New("boom"), wantStatus: http.StatusInternalServerError},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ms := newStoreWithKey(t)
			ms.getByIDErr = tc.getErr
			ms.updateErr = tc.updateErr

			req := withIDParam(t, httptest.NewRequest(http.MethodPatch, "/admin/apikeys/1", strings.NewReader(tc.body)), tc.id)
			w := httptest.NewRecorder()
			handleUpdateAPIKey(ms)(w, req)

			require.Equal(t, tc.wantStatus, w.Code)
		})
	}

	t.Run("full partial update applies all fields", func(t *testing.T) {
		ms := newStoreWithKey(t)
		body := `{"name":"new","status":"inactive","model_whitelist":["m"],"rate_limit":5,"daily_limit":50,"expires_at":"2030-01-02T03:04:05Z"}`
		req := withIDParam(t, httptest.NewRequest(http.MethodPatch, "/admin/apikeys/1", strings.NewReader(body)), "1")
		w := httptest.NewRecorder()
		handleUpdateAPIKey(ms)(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		updated := ms.keys[0]
		assert.Equal(t, "new", updated.Name)
		assert.Equal(t, "inactive", updated.Status)
		assert.Equal(t, store.StringSlice{"m"}, updated.ModelWhitelist)
		assert.Equal(t, 5, updated.RateLimit)
		assert.Equal(t, 50, updated.DailyLimit)
		require.NotNil(t, updated.ExpiresAt)
	})
}

func TestHandleDeleteAPIKey_Extra(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		deleteErr  error
		wantStatus int
	}{
		{name: "invalid id returns 400", id: "abc", wantStatus: http.StatusBadRequest},
		{name: "delete error returns 500", id: "1", deleteErr: errors.New("boom"), wantStatus: http.StatusInternalServerError},
		{name: "success returns 204", id: "1", wantStatus: http.StatusNoContent},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ms := newMockAPIKeyStore()
			require.NoError(t, ms.Create(context.Background(), &store.APIKey{Name: "a"}))
			ms.deleteErr = tc.deleteErr

			req := withIDParam(t, httptest.NewRequest(http.MethodDelete, "/admin/apikeys/1", nil), tc.id)
			w := httptest.NewRecorder()
			handleDeleteAPIKey(ms)(w, req)

			require.Equal(t, tc.wantStatus, w.Code)
		})
	}
}

func TestHandleRegenerateAPIKey_Extra(t *testing.T) {
	tests := []struct {
		name          string
		id            string
		getErr        error
		regenerateErr error
		wantStatus    int
	}{
		{name: "invalid id returns 400", id: "abc", wantStatus: http.StatusBadRequest},
		{name: "not found returns 404", id: "999", wantStatus: http.StatusNotFound},
		{name: "get error returns 500", id: "1", getErr: errors.New("boom"), wantStatus: http.StatusInternalServerError},
		{name: "regenerate error returns 500", id: "1", regenerateErr: errors.New("boom"), wantStatus: http.StatusInternalServerError},
		{name: "success returns new key", id: "1", wantStatus: http.StatusOK},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ms := newMockAPIKeyStore()
			require.NoError(t, ms.Create(context.Background(), &store.APIKey{Name: "a", Key: "gf-key-a"}))
			ms.getByIDErr = tc.getErr
			ms.regenerateErr = tc.regenerateErr

			req := withIDParam(t, httptest.NewRequest(http.MethodPost, "/admin/apikeys/1/regenerate", nil), tc.id)
			w := httptest.NewRecorder()
			handleRegenerateAPIKey(ms)(w, req)

			require.Equal(t, tc.wantStatus, w.Code)
			if tc.wantStatus != http.StatusOK {
				return
			}
			var resp APIKeyCreateResponse
			require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
			assert.Equal(t, "gf-newkey567890abcdef1234567890abcdef1234567890ab", resp.Key)
		})
	}
}

func TestHandleAPIKeyStats_Extra(t *testing.T) {
	t.Run("error returns 500", func(t *testing.T) {
		ms := newMockAPIKeyStore()
		ms.countByStatusErr = errors.New("boom")
		w := httptest.NewRecorder()
		handleAPIKeyStats(ms)(w, httptest.NewRequest(http.MethodGet, "/", nil))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("success returns counts", func(t *testing.T) {
		ms := newMockAPIKeyStore()
		require.NoError(t, ms.Create(context.Background(), &store.APIKey{Name: "a", Status: "active"}))
		require.NoError(t, ms.Create(context.Background(), &store.APIKey{Name: "b", Status: "inactive"}))
		require.NoError(t, ms.Create(context.Background(), &store.APIKey{Name: "c", Status: "expired"}))
		require.NoError(t, ms.Create(context.Background(), &store.APIKey{Name: "d", Status: "rate_limited"}))

		w := httptest.NewRecorder()
		handleAPIKeyStats(ms)(w, httptest.NewRequest(http.MethodGet, "/", nil))

		require.Equal(t, http.StatusOK, w.Code)
		var resp APIKeyStatsResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, 4, resp.Total)
		assert.Equal(t, 1, resp.Active)
		assert.Equal(t, 1, resp.Inactive)
		assert.Equal(t, 1, resp.Expired)
		assert.Equal(t, 1, resp.RateLimited)
	})
}

func TestMaskKey_Extra(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{name: "short key fully masked", key: "abc", want: "****"},
		{name: "exactly 8 chars masked", key: "12345678", want: "****"},
		{name: "long key shows head and tail", key: "gf-abcdefghijklmnop", want: "gf-a...mnop"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, maskKey(tc.key))
		})
	}
}

func TestToAPIKeyResponse_Fields(t *testing.T) {
	now := timeNow()
	ak := &store.APIKey{
		ID:             7,
		Key:            "gf-abcdefghijklmnop",
		Name:           "n",
		Status:         "active",
		ModelWhitelist: store.StringSlice{"m1", "m2"},
		RateLimit:      3,
		DailyLimit:     9,
		DailyUsed:      1,
		TotalUsed:      11,
		LastUsedAt:     &now,
		ExpiresAt:      &now,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	resp := toAPIKeyResponse(ak)
	assert.Equal(t, uint(7), resp.ID)
	assert.Equal(t, "gf-a...mnop", resp.Key)
	assert.Equal(t, "n", resp.Name)
	assert.Len(t, resp.ModelWhitelist, 2)
	assert.Equal(t, 3, resp.RateLimit)
	assert.Equal(t, 9, resp.DailyLimit)
	assert.Equal(t, 1, resp.DailyUsed)
	assert.Equal(t, 11, resp.TotalUsed)
	assert.Equal(t, fmt.Sprint(now.Unix()), fmt.Sprint(resp.LastUsedAt.Unix()))
	assert.NotNil(t, resp.ExpiresAt)
}
