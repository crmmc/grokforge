package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/registry"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/glebarez/sqlite"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// mockModelStore implements a minimal in-memory ModelStore for testing.
type mockModelStore struct {
	families []*store.ModelFamily
	modes    []*store.ModelMode
	nextFID  uint
	nextMID  uint
}

func newMockModelStore() *mockModelStore {
	return &mockModelStore{nextFID: 1, nextMID: 1}
}

func (m *mockModelStore) ListFamilies(_ context.Context) ([]*store.ModelFamily, error) {
	return m.families, nil
}

func (m *mockModelStore) GetFamily(_ context.Context, id uint) (*store.ModelFamily, error) {
	for _, f := range m.families {
		if f.ID == id {
			return f, nil
		}
	}
	return nil, store.ErrNotFound
}

func (m *mockModelStore) CreateFamily(_ context.Context, f *store.ModelFamily) error {
	f.ID = m.nextFID
	m.nextFID++
	m.families = append(m.families, f)
	return nil
}

func (m *mockModelStore) UpdateFamily(_ context.Context, f *store.ModelFamily) error {
	for i, existing := range m.families {
		if existing.ID == f.ID {
			m.families[i] = f
			return nil
		}
	}
	return store.ErrNotFound
}

func (m *mockModelStore) DeleteFamily(_ context.Context, id uint) error {
	for i, f := range m.families {
		if f.ID == id {
			m.families = append(m.families[:i], m.families[i+1:]...)
			// Also delete modes
			var remaining []*store.ModelMode
			for _, mode := range m.modes {
				if mode.ModelID != id {
					remaining = append(remaining, mode)
				}
			}
			m.modes = remaining
			return nil
		}
	}
	return store.ErrNotFound
}

func (m *mockModelStore) ListModesByFamily(_ context.Context, familyID uint) ([]*store.ModelMode, error) {
	var result []*store.ModelMode
	for _, mode := range m.modes {
		if mode.ModelID == familyID {
			result = append(result, mode)
		}
	}
	return result, nil
}

func (m *mockModelStore) GetMode(_ context.Context, id uint) (*store.ModelMode, error) {
	for _, mode := range m.modes {
		if mode.ID == id {
			return mode, nil
		}
	}
	return nil, store.ErrNotFound
}

func (m *mockModelStore) CreateMode(_ context.Context, mode *store.ModelMode) error {
	mode.ID = m.nextMID
	m.nextMID++
	m.modes = append(m.modes, mode)
	return nil
}

func (m *mockModelStore) UpdateMode(_ context.Context, mode *store.ModelMode) error {
	for i, existing := range m.modes {
		if existing.ID == mode.ID {
			m.modes[i] = mode
			return nil
		}
	}
	return store.ErrNotFound
}

func (m *mockModelStore) DeleteMode(_ context.Context, id uint) error {
	for i, mode := range m.modes {
		if mode.ID == id {
			m.modes = append(m.modes[:i], m.modes[i+1:]...)
			return nil
		}
	}
	return store.ErrNotFound
}

// mockRegistryRefresher tracks Refresh calls.
type mockRegistryRefresher struct {
	refreshCount int
}

func (m *mockRegistryRefresher) Refresh(_ context.Context) error {
	m.refreshCount++
	return nil
}

// --- Family Tests ---

func TestAdminFamilyList(t *testing.T) {
	ms := newMockModelStore()
	ms.CreateFamily(context.Background(), &store.ModelFamily{
		Model: "grok-3", DisplayName: "Grok 3", Type: "chat", PoolFloor: "basic",
	})
	ms.CreateMode(context.Background(), &store.ModelMode{
		ModelID: 1, Mode: "default", UpstreamModel: "grok-3", UpstreamMode: "MODE_GROK_3",
	})

	srv := newTestModelServer(ms, nil)
	req := httptest.NewRequest(http.MethodGet, "/models/families", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var families []FamilyResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&families))
	assert.Len(t, families, 1)
	assert.Equal(t, "grok-3", families[0].Model)
	assert.Len(t, families[0].Modes, 1)
}

func TestAdminFamilyCreate(t *testing.T) {
	ms := newMockModelStore()
	reg := &mockRegistryRefresher{}

	srv := newTestModelServer(ms, reg)
	body := `{"model":"grok-4","display_name":"Grok 4","type":"chat","pool_floor":"super"}`
	req := httptest.NewRequest(http.MethodPost, "/models/families", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, 1, reg.refreshCount, "Registry should be refreshed after create")

	var resp FamilyResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "grok-4", resp.Model)
	assert.Equal(t, "Grok 4", resp.DisplayName)
}

func TestAdminFamilyCreate_NormalizesTrimmedFields(t *testing.T) {
	ms := newMockModelStore()
	reg := &mockRegistryRefresher{}

	srv := newTestModelServer(ms, reg)
	body := `{"model":"  grok-4  ","display_name":"Grok 4","type":"  chat  ","pool_floor":"  super  ","quota_default":"   "}`
	req := httptest.NewRequest(http.MethodPost, "/models/families", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	require.Len(t, ms.families, 1)
	assert.Equal(t, "grok-4", ms.families[0].Model)
	assert.Equal(t, "chat", ms.families[0].Type)
	assert.Equal(t, "super", ms.families[0].PoolFloor)
	assert.Nil(t, ms.families[0].QuotaDefault)
}

func TestAdminFamilyCreate_RejectsUnknownField(t *testing.T) {
	ms := newMockModelStore()
	reg := &mockRegistryRefresher{}

	srv := newTestModelServer(ms, reg)
	body := `{"model":"grok-4","display_name":"Grok 4","type":"chat","pool_floor":"super","unknown":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/models/families", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, ms.families)
	assert.Equal(t, 0, reg.refreshCount)
}

func TestAdminFamilyGet(t *testing.T) {
	ms := newMockModelStore()
	ms.CreateFamily(context.Background(), &store.ModelFamily{
		Model: "grok-3", DisplayName: "Grok 3", Type: "chat", PoolFloor: "basic",
	})

	srv := newTestModelServer(ms, nil)
	req := httptest.NewRequest(http.MethodGet, "/models/families/1", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp FamilyResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "grok-3", resp.Model)
}

func TestAdminFamilyGet_NotFound(t *testing.T) {
	ms := newMockModelStore()

	srv := newTestModelServer(ms, nil)
	req := httptest.NewRequest(http.MethodGet, "/models/families/999", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestAdminFamilyUpdate(t *testing.T) {
	ms := newMockModelStore()
	ms.CreateFamily(context.Background(), &store.ModelFamily{
		Model: "grok-3", DisplayName: "Grok 3", Type: "chat", PoolFloor: "basic",
	})
	reg := &mockRegistryRefresher{}

	srv := newTestModelServer(ms, reg)
	body := `{"model":"grok-3","display_name":"Grok 3 Updated","type":"chat","pool_floor":"super"}`
	req := httptest.NewRequest(http.MethodPut, "/models/families/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, reg.refreshCount, "Registry should be refreshed after update")

	var resp FamilyResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "Grok 3 Updated", resp.DisplayName)
	assert.Equal(t, "super", resp.PoolFloor)
}

func TestAdminFamilyUpdate_ClearsNullableFields(t *testing.T) {
	ms := newMockModelStore()
	defaultModeID := uint(7)
	quotaDefault := `{"chat":100}`
	ms.CreateFamily(context.Background(), &store.ModelFamily{
		Model: "grok-3", DisplayName: "Grok 3", Type: "chat", PoolFloor: "basic",
		DefaultModeID: &defaultModeID, QuotaDefault: &quotaDefault,
	})
	reg := &mockRegistryRefresher{}

	srv := newTestModelServer(ms, reg)
	body := `{"model":"grok-3","display_name":"Grok 3","type":"chat","pool_floor":"basic","default_mode_id":null,"quota_default":null}`
	req := httptest.NewRequest(http.MethodPut, "/models/families/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp FamilyResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Nil(t, resp.DefaultModeID)
	assert.Nil(t, resp.QuotaDefault)
}

func TestAdminFamilyUpdate_RejectsUnknownField(t *testing.T) {
	ms := newMockModelStore()
	ms.CreateFamily(context.Background(), &store.ModelFamily{
		Model: "grok-3", DisplayName: "Grok 3", Type: "chat", PoolFloor: "basic",
	})
	reg := &mockRegistryRefresher{}

	srv := newTestModelServer(ms, reg)
	body := `{"model":"grok-3","display_name":"Grok 3","type":"chat","pool_floor":"basic","unknown":"x"}`
	req := httptest.NewRequest(http.MethodPut, "/models/families/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "grok-3", ms.families[0].Model)
	assert.Equal(t, 0, reg.refreshCount)
}

func TestAdminFamilyDelete(t *testing.T) {
	ms := newMockModelStore()
	ms.CreateFamily(context.Background(), &store.ModelFamily{
		Model: "grok-3", DisplayName: "Grok 3", Type: "chat", PoolFloor: "basic",
	})
	reg := &mockRegistryRefresher{}

	srv := newTestModelServer(ms, reg)
	req := httptest.NewRequest(http.MethodDelete, "/models/families/1", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, 1, reg.refreshCount, "Registry should be refreshed after delete")
}

// --- Mode Tests ---

func TestAdminModeCreate(t *testing.T) {
	ms := newMockModelStore()
	ms.CreateFamily(context.Background(), &store.ModelFamily{
		Model: "grok-3", DisplayName: "Grok 3", Type: "chat", PoolFloor: "basic",
	})
	reg := &mockRegistryRefresher{}

	srv := newTestModelServer(ms, reg)
	body := `{"model_id":1,"mode":"thinking","upstream_model":"grok-3","upstream_mode":"MODE_GROK_3_THINK"}`
	req := httptest.NewRequest(http.MethodPost, "/models/modes", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, 1, reg.refreshCount)

	var resp store.ModelMode
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "thinking", resp.Mode)
	assert.Equal(t, uint(1), resp.ModelID)
}

func TestAdminModeCreate_ImageFamilyRejectsUpstreamMapping(t *testing.T) {
	db := setupAdminModelDB(t)
	ms := store.NewModelStore(db)
	reg := registry.NewModelRegistry(ms)
	require.NoError(t, reg.Refresh(context.Background()))

	ctx := context.Background()
	family := &store.ModelFamily{
		Model:     "grok-imagine-image",
		Type:      "image",
		PoolFloor: "super",
		Enabled:   true,
	}
	require.NoError(t, ms.CreateFamily(ctx, family))

	srv := newTestModelServer(ms, reg)
	body := `{"model_id":1,"mode":"default","upstream_model":"grok-3","upstream_mode":"MODEL_MODE_FAST"}`
	req := httptest.NewRequest(http.MethodPost, "/models/modes", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestAdminModeCreate_NormalizesTrimmedFields(t *testing.T) {
	ms := newMockModelStore()
	ms.CreateFamily(context.Background(), &store.ModelFamily{
		Model: "grok-3", DisplayName: "Grok 3", Type: "chat", PoolFloor: "basic",
	})
	reg := &mockRegistryRefresher{}

	srv := newTestModelServer(ms, reg)
	body := `{"model_id":1,"mode":"  default  ","pool_floor_override":"  basic  ","upstream_model":"  grok-3  ","upstream_mode":"  MODE_GROK_3  ","quota_override":"   "}`
	req := httptest.NewRequest(http.MethodPost, "/models/modes", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
	require.Len(t, ms.modes, 1)
	assert.Equal(t, "default", ms.modes[0].Mode)
	require.NotNil(t, ms.modes[0].PoolFloorOverride)
	assert.Equal(t, "basic", *ms.modes[0].PoolFloorOverride)
	assert.Equal(t, "grok-3", ms.modes[0].UpstreamModel)
	assert.Equal(t, "MODE_GROK_3", ms.modes[0].UpstreamMode)
	assert.Nil(t, ms.modes[0].QuotaOverride)
}

func TestAdminModeCreate_RejectsUnknownField(t *testing.T) {
	ms := newMockModelStore()
	ms.CreateFamily(context.Background(), &store.ModelFamily{
		Model: "grok-3", DisplayName: "Grok 3", Type: "chat", PoolFloor: "basic",
	})
	reg := &mockRegistryRefresher{}

	srv := newTestModelServer(ms, reg)
	body := `{"model_id":1,"mode":"default","upstream_model":"grok-3","upstream_mode":"MODEL_MODE_AUTO","unknown":"x"}`
	req := httptest.NewRequest(http.MethodPost, "/models/modes", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Empty(t, ms.modes)
	assert.Equal(t, 0, reg.refreshCount)
}

func TestAdminModeGet(t *testing.T) {
	ms := newMockModelStore()
	ms.CreateMode(context.Background(), &store.ModelMode{
		ModelID: 1, Mode: "default", UpstreamModel: "grok-3", UpstreamMode: "MODE_GROK_3",
	})

	srv := newTestModelServer(ms, nil)
	req := httptest.NewRequest(http.MethodGet, "/models/modes/1", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp store.ModelMode
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "default", resp.Mode)
}

func TestAdminModeUpdate(t *testing.T) {
	ms := newMockModelStore()
	ms.CreateMode(context.Background(), &store.ModelMode{
		ModelID: 1, Mode: "default", UpstreamModel: "grok-3", UpstreamMode: "MODE_GROK_3",
	})
	reg := &mockRegistryRefresher{}

	srv := newTestModelServer(ms, reg)
	body := `{"model_id":1,"mode":"default","upstream_model":"grok-3","upstream_mode":"MODE_GROK_3_V2","enabled":true}`
	req := httptest.NewRequest(http.MethodPut, "/models/modes/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 1, reg.refreshCount)

	var resp store.ModelMode
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.Equal(t, "MODE_GROK_3_V2", resp.UpstreamMode)
}

func TestAdminModeUpdate_RejectsUnknownField(t *testing.T) {
	ms := newMockModelStore()
	ms.CreateMode(context.Background(), &store.ModelMode{
		ModelID: 1, Mode: "default", UpstreamModel: "grok-3", UpstreamMode: "MODE_GROK_3",
	})
	reg := &mockRegistryRefresher{}

	srv := newTestModelServer(ms, reg)
	body := `{"model_id":1,"mode":"default","upstream_model":"grok-3","upstream_mode":"MODE_GROK_3","unknown":"x"}`
	req := httptest.NewRequest(http.MethodPut, "/models/modes/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "MODE_GROK_3", ms.modes[0].UpstreamMode)
	assert.Equal(t, 0, reg.refreshCount)
}

func TestAdminModeDelete(t *testing.T) {
	ms := newMockModelStore()
	ms.CreateMode(context.Background(), &store.ModelMode{
		ModelID: 1, Mode: "default", UpstreamModel: "grok-3", UpstreamMode: "MODE_GROK_3",
	})
	reg := &mockRegistryRefresher{}

	srv := newTestModelServer(ms, reg)
	req := httptest.NewRequest(http.MethodDelete, "/models/modes/1", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, 1, reg.refreshCount)
}

// --- Refresh Test ---

func TestAdminCRUDRefresh(t *testing.T) {
	ms := newMockModelStore()
	reg := &mockRegistryRefresher{}

	srv := newTestModelServer(ms, reg)

	// Create family
	body := `{"model":"grok-5","display_name":"Grok 5","type":"chat","pool_floor":"basic"}`
	req := httptest.NewRequest(http.MethodPost, "/models/families", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Create mode
	body = `{"model_id":1,"mode":"fast","upstream_model":"grok-5","upstream_mode":"MODE_GROK_5_FAST"}`
	req = httptest.NewRequest(http.MethodPost, "/models/modes", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Update family
	body = `{"model":"grok-5","display_name":"Grok 5 Pro","type":"chat","pool_floor":"super"}`
	req = httptest.NewRequest(http.MethodPut, "/models/families/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Delete mode
	req = httptest.NewRequest(http.MethodDelete, "/models/modes/1", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)

	assert.Equal(t, 4, reg.refreshCount, "Each write op should trigger one Refresh")
}

func TestAdminFamilyUpdate_RollsBackOnInvalidTypeChange(t *testing.T) {
	db := setupAdminModelDB(t)
	ms := store.NewModelStore(db)
	reg := registry.NewModelRegistry(ms)
	require.NoError(t, reg.Refresh(context.Background()))

	ctx := context.Background()
	family := &store.ModelFamily{
		Model:     "grok-imagine-image",
		Type:      "image",
		PoolFloor: "super",
		Enabled:   true,
	}
	require.NoError(t, ms.CreateFamily(ctx, family))

	mode := &store.ModelMode{
		ModelID: family.ID,
		Mode:    "default",
		Enabled: true,
	}
	require.NoError(t, ms.CreateMode(ctx, mode))
	require.NoError(t, reg.Refresh(ctx))

	srv := newTestModelServer(ms, reg)
	body := fmt.Sprintf(`{"model":"grok-imagine-image","display_name":"","type":"chat","pool_floor":"super","default_mode_id":%d}`, mode.ID)
	req := httptest.NewRequest(http.MethodPut, "/models/families/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)

	stored, err := ms.GetFamily(ctx, family.ID)
	require.NoError(t, err)
	assert.Equal(t, "image", stored.Type)

	resolved, ok := reg.Resolve("grok-imagine-image")
	require.True(t, ok)
	assert.Equal(t, "image", resolved.Family.Type)
}

// newTestModelServer creates a chi router with model CRUD routes for testing.
func newTestModelServer(ms ModelStoreInterface, reg RegistryRefresher) http.Handler {
	r := chi.NewRouter()
	r.Route("/models", func(r chi.Router) {
		r.Get("/families", handleListFamilies(ms, reg))
		r.Post("/families", handleCreateFamily(ms, reg))
		r.Get("/families/{id}", handleGetFamily(ms, reg))
		r.Put("/families/{id}", handleUpdateFamily(ms, reg))
		r.Delete("/families/{id}", handleDeleteFamily(ms, reg))
		r.Post("/modes", handleCreateMode(ms, reg))
		r.Get("/modes/{id}", handleGetMode(ms))
		r.Put("/modes/{id}", handleUpdateMode(ms, reg))
		r.Delete("/modes/{id}", handleDeleteMode(ms, reg))
	})
	return r
}

func setupAdminModelDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, store.AutoMigrate(db))
	return db
}
