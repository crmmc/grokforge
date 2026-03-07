package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAPIKeyStore is a mock for APIKeyStoreInterface.
type mockAPIKeyStore struct {
	keys             []*store.APIKey
	nextID           uint
	createErr        error
	listErr          error
	getByIDErr       error
	updateErr        error
	deleteErr        error
	regenerateErr    error
	countByStatusErr error
	incrementCalled  bool
}

func newMockAPIKeyStore() *mockAPIKeyStore {
	return &mockAPIKeyStore{nextID: 1}
}

func (m *mockAPIKeyStore) List(_ context.Context, page, pageSize int, status string) ([]*store.APIKey, int64, error) {
	if m.listErr != nil {
		return nil, 0, m.listErr
	}
	var filtered []*store.APIKey
	for _, k := range m.keys {
		if status == "" || k.Status == status {
			filtered = append(filtered, k)
		}
	}
	total := int64(len(filtered))
	offset := (page - 1) * pageSize
	end := offset + pageSize
	if offset > len(filtered) {
		offset = len(filtered)
	}
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[offset:end], total, nil
}

func (m *mockAPIKeyStore) GetByID(_ context.Context, id uint) (*store.APIKey, error) {
	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}
	for _, k := range m.keys {
		if k.ID == id {
			return k, nil
		}
	}
	return nil, store.ErrNotFound
}

func (m *mockAPIKeyStore) GetByKey(_ context.Context, key string) (*store.APIKey, error) {
	for _, k := range m.keys {
		if k.Key == key {
			return k, nil
		}
	}
	return nil, store.ErrNotFound
}

func (m *mockAPIKeyStore) Create(_ context.Context, ak *store.APIKey) error {
	if m.createErr != nil {
		return m.createErr
	}
	ak.ID = m.nextID
	m.nextID++
	ak.Key = "gf-mock1234567890abcdef1234567890abcdef1234567890ab"
	if ak.Status == "" {
		ak.Status = "active"
	}
	m.keys = append(m.keys, ak)
	return nil
}

func (m *mockAPIKeyStore) Update(_ context.Context, ak *store.APIKey) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	for i, k := range m.keys {
		if k.ID == ak.ID {
			m.keys[i] = ak
			return nil
		}
	}
	return store.ErrNotFound
}

func (m *mockAPIKeyStore) Delete(_ context.Context, id uint) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	for i, k := range m.keys {
		if k.ID == id {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *mockAPIKeyStore) Regenerate(_ context.Context, id uint) (string, error) {
	if m.regenerateErr != nil {
		return "", m.regenerateErr
	}
	for _, k := range m.keys {
		if k.ID == id {
			k.Key = "gf-newkey567890abcdef1234567890abcdef1234567890ab"
			return k.Key, nil
		}
	}
	return "", store.ErrNotFound
}

func (m *mockAPIKeyStore) CountByStatus(_ context.Context) (int, int, int, int, int, error) {
	if m.countByStatusErr != nil {
		return 0, 0, 0, 0, 0, m.countByStatusErr
	}
	total, active, inactive, expired, rateLimited := 0, 0, 0, 0, 0
	for _, k := range m.keys {
		total++
		switch k.Status {
		case "active":
			active++
		case "inactive":
			inactive++
		case "expired":
			expired++
		case "rate_limited":
			rateLimited++
		}
	}
	return total, active, inactive, expired, rateLimited, nil
}

func (m *mockAPIKeyStore) IncrementUsage(_ context.Context, _ uint) error {
	m.incrementCalled = true
	return nil
}

func (m *mockAPIKeyStore) ResetDailyUsage(_ context.Context) error {
	return nil
}

func TestHandleListAPIKeys(t *testing.T) {
	ms := newMockAPIKeyStore()
	for i := 0; i < 5; i++ {
		ms.Create(context.Background(), &store.APIKey{Name: "key"})
	}

	r := chi.NewRouter()
	r.Get("/apikeys", handleListAPIKeys(ms))

	req := httptest.NewRequest(http.MethodGet, "/apikeys?page=1&page_size=3", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp PaginatedResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, int64(5), resp.Total)
	assert.Equal(t, 1, resp.Page)
	assert.Equal(t, 3, resp.PageSize)
	assert.Equal(t, 2, resp.TotalPages)
}

func TestHandleCreateAPIKey(t *testing.T) {
	ms := newMockAPIKeyStore()

	r := chi.NewRouter()
	r.Post("/apikeys", handleCreateAPIKey(ms))

	body := `{"name":"my-key","rate_limit":30,"daily_limit":500}`
	req := httptest.NewRequest(http.MethodPost, "/apikeys", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)

	var resp APIKeyCreateResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.NotZero(t, resp.ID)
	assert.NotEmpty(t, resp.Key)
	assert.Equal(t, "my-key", resp.Name)
}

func TestHandleCreateAPIKey_MissingName(t *testing.T) {
	ms := newMockAPIKeyStore()

	r := chi.NewRouter()
	r.Post("/apikeys", handleCreateAPIKey(ms))

	body := `{"rate_limit":30}`
	req := httptest.NewRequest(http.MethodPost, "/apikeys", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleUpdateAPIKey(t *testing.T) {
	ms := newMockAPIKeyStore()
	ms.Create(context.Background(), &store.APIKey{Name: "orig"})

	r := chi.NewRouter()
	r.Patch("/apikeys/{id}", handleUpdateAPIKey(ms))

	body := `{"name":"updated","rate_limit":120}`
	req := httptest.NewRequest(http.MethodPatch, "/apikeys/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp store.APIKey
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "updated", resp.Name)
	assert.Equal(t, 120, resp.RateLimit)
}

func TestHandleDeleteAPIKey(t *testing.T) {
	ms := newMockAPIKeyStore()
	ms.Create(context.Background(), &store.APIKey{Name: "del"})

	r := chi.NewRouter()
	r.Delete("/apikeys/{id}", handleDeleteAPIKey(ms))

	req := httptest.NewRequest(http.MethodDelete, "/apikeys/1", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestHandleRegenerateAPIKey(t *testing.T) {
	ms := newMockAPIKeyStore()
	ms.Create(context.Background(), &store.APIKey{Name: "regen"})
	oldKey := ms.keys[0].Key

	r := chi.NewRouter()
	r.Post("/apikeys/{id}/regenerate", handleRegenerateAPIKey(ms))

	req := httptest.NewRequest(http.MethodPost, "/apikeys/1/regenerate", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp APIKeyCreateResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.NotEqual(t, oldKey, resp.Key)
	assert.Equal(t, "regen", resp.Name)
}

func TestHandleAPIKeyStats(t *testing.T) {
	ms := newMockAPIKeyStore()
	ms.Create(context.Background(), &store.APIKey{Name: "a1", Status: "active"})
	ms.Create(context.Background(), &store.APIKey{Name: "a2", Status: "active"})
	ms.Create(context.Background(), &store.APIKey{Name: "i1", Status: "inactive"})

	r := chi.NewRouter()
	r.Get("/apikeys/stats", handleAPIKeyStats(ms))

	req := httptest.NewRequest(http.MethodGet, "/apikeys/stats", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp map[string]int
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, 3, resp["total"])
	assert.Equal(t, 2, resp["active"])
	assert.Equal(t, 1, resp["inactive"])
}

func TestHandleGetAPIKey_InvalidID(t *testing.T) {
	ms := newMockAPIKeyStore()

	r := chi.NewRouter()
	r.Get("/apikeys/{id}", handleGetAPIKey(ms))

	req := httptest.NewRequest(http.MethodGet, "/apikeys/abc", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestHandleGetAPIKey_NotFound(t *testing.T) {
	ms := newMockAPIKeyStore()

	r := chi.NewRouter()
	r.Get("/apikeys/{id}", handleGetAPIKey(ms))

	req := httptest.NewRequest(http.MethodGet, "/apikeys/999", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// Ensure time import is used
var _ = time.Now
