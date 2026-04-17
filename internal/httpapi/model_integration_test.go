package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	seedconfig "github.com/crmmc/grokforge/config"
	"github.com/crmmc/grokforge/internal/registry"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/crmmc/grokforge/internal/token"
	"github.com/glebarez/sqlite"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// setupModelIntegrationStack creates a real SQLite DB + ModelStore + ModelRegistry + HTTP router.
// The DB is seeded with models.seed.toml via embed fallback.
func setupModelIntegrationStack(t *testing.T, seed bool) (
	*store.ModelStore, *registry.ModelRegistry, http.Handler,
) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, store.AutoMigrate(db))

	if seed {
		require.NoError(t, store.SeedModels(context.Background(), db, "", seedconfig.SeedFS))
	}

	ms := store.NewModelStore(db)
	reg := registry.NewModelRegistry(ms)
	require.NoError(t, reg.Refresh(context.Background()))

	r := chi.NewRouter()
	// Admin model routes
	r.Route("/admin/models", func(r chi.Router) {
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
	// OpenAI models endpoint (local handler to avoid import cycle)
	r.Get("/v1/models", handleModelsForTest(reg))

	return ms, reg, r
}

// modelsResponseIDs extracts sorted model IDs from GET /v1/models.
func modelsResponseIDs(t *testing.T, srv http.Handler) []string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp modelsListResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	ids := make([]string, len(resp.Data))
	for i, e := range resp.Data {
		ids[i] = e.ID
	}
	return ids
}

func postJSON(t *testing.T, srv http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func putJSON(t *testing.T, srv http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func doDelete(t *testing.T, srv http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, path, nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

func doGet(t *testing.T, srv http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	return rec
}

// modelsListResponse mirrors openai.ModelsResponse to avoid import cycle.
type modelsListResponse struct {
	Object string             `json:"object"`
	Data   []modelsListEntry  `json:"data"`
}

type modelsListEntry struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
	Type    string `json:"type,omitempty"`
}

// handleModelsForTest creates a /v1/models handler without importing openai package.
func handleModelsForTest(reg *registry.ModelRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		all := reg.AllEnabled()
		entries := make([]modelsListEntry, 0, len(all))
		for _, rm := range all {
			typ := ""
			if rm.Family != nil {
				typ = rm.Family.Type
			}
			entries = append(entries, modelsListEntry{
				ID:      rm.RequestName,
				Object:  "model",
				Created: 1709251200,
				OwnedBy: "xai",
				Type:    typ,
			})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].ID < entries[j].ID
		})
		WriteJSON(w, http.StatusOK, modelsListResponse{Object: "list", Data: entries})
	}
}

// --- P0 Integration Tests ---

// BE-001: Seed → GET /v1/models returns correct derived request names.
func TestIntegration_SeedToModelsEndpoint(t *testing.T) {
	_, _, srv := setupModelIntegrationStack(t, true)

	ids := modelsResponseIDs(t, srv)

	// Seed has 5 families with these modes:
	// grok-4.20: default, fast, expert, heavy → 4 request names
	// grok-4.20-mini: default → 1 request name
	// grok-imagine-image: default, lite → 2 request names
	// grok-imagine-image-edit: default → 1 request name
	// grok-imagine-video: default → 1 request name
	// Total: 9
	assert.Len(t, ids, 9, "seed should produce 9 model entries")

	expected := []string{
		"grok-4.20",
		"grok-4.20-expert",
		"grok-4.20-fast",
		"grok-4.20-heavy",
		"grok-4.20-mini",
		"grok-imagine-image",
		"grok-imagine-image-edit",
		"grok-imagine-image-lite",
		"grok-imagine-video",
	}
	assert.Equal(t, expected, ids)
}

// BE-002: POST create family + default mode → GET /v1/models includes new model.
func TestIntegration_CreateFamilyMode_VisibleInModels(t *testing.T) {
	_, _, srv := setupModelIntegrationStack(t, false)

	// Create family
	rec := postJSON(t, srv, "/admin/models/families",
		`{"model":"test-chat","display_name":"Test Chat","type":"chat","pool_floor":"basic"}`)
	require.Equal(t, http.StatusCreated, rec.Code)

	var fam FamilyResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&fam))

	// Create default mode
	body := fmt.Sprintf(`{"model_id":%d,"mode":"default","upstream_model":"grok-3","upstream_mode":"MODEL_MODE_AUTO"}`, fam.ID)
	rec = postJSON(t, srv, "/admin/models/modes", body)
	require.Equal(t, http.StatusCreated, rec.Code)

	ids := modelsResponseIDs(t, srv)
	assert.Contains(t, ids, "test-chat")
}

// BE-003: DELETE family → GET /v1/models no longer includes it.
func TestIntegration_DeleteFamily_RemovedFromModels(t *testing.T) {
	_, _, srv := setupModelIntegrationStack(t, false)

	// Create family + mode
	rec := postJSON(t, srv, "/admin/models/families",
		`{"model":"ephemeral","display_name":"Ephemeral","type":"chat","pool_floor":"basic"}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	var fam FamilyResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&fam))

	body := fmt.Sprintf(`{"model_id":%d,"mode":"default","upstream_model":"grok-3","upstream_mode":"MODEL_MODE_AUTO"}`, fam.ID)
	rec = postJSON(t, srv, "/admin/models/modes", body)
	require.Equal(t, http.StatusCreated, rec.Code)

	ids := modelsResponseIDs(t, srv)
	require.Contains(t, ids, "ephemeral")

	// Delete family
	rec = doDelete(t, srv, fmt.Sprintf("/admin/models/families/%d", fam.ID))
	require.Equal(t, http.StatusNoContent, rec.Code)

	ids = modelsResponseIDs(t, srv)
	assert.NotContains(t, ids, "ephemeral")
}

// BE-004: image family → PUT type=chat → 400 (mode lacks upstream) → registry unchanged.
func TestIntegration_TypeChangeRollback_AtomicRegistry(t *testing.T) {
	ms, reg, srv := setupModelIntegrationStack(t, false)

	ctx := context.Background()
	// Create image family + mode (no upstream)
	rec := postJSON(t, srv, "/admin/models/families",
		`{"model":"img-test","display_name":"Img","type":"image","pool_floor":"super"}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	var fam FamilyResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&fam))

	body := fmt.Sprintf(`{"model_id":%d,"mode":"default"}`, fam.ID)
	rec = postJSON(t, srv, "/admin/models/modes", body)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Try to change type to chat → should fail because mode has no upstream
	var mode store.ModelMode
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&mode))

	body = fmt.Sprintf(`{"model":"img-test","display_name":"Img","type":"chat","pool_floor":"super","default_mode_id":%d}`, mode.ID)
	rec = putJSON(t, srv, fmt.Sprintf("/admin/models/families/%d", fam.ID), body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// Verify DB unchanged
	stored, err := ms.GetFamily(ctx, fam.ID)
	require.NoError(t, err)
	assert.Equal(t, "image", stored.Type)

	// Verify registry unchanged
	resolved, ok := reg.Resolve("img-test")
	require.True(t, ok)
	assert.Equal(t, "image", resolved.Family.Type)
}

// BE-005: Non-default mode request name derivation.
func TestIntegration_NonDefaultModeRequestName(t *testing.T) {
	_, reg, srv := setupModelIntegrationStack(t, false)

	// Create family
	rec := postJSON(t, srv, "/admin/models/families",
		`{"model":"mymodel","display_name":"My Model","type":"chat","pool_floor":"basic"}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	var fam FamilyResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&fam))

	// Create default mode
	body := fmt.Sprintf(`{"model_id":%d,"mode":"default","upstream_model":"grok-3","upstream_mode":"MODE_AUTO"}`, fam.ID)
	rec = postJSON(t, srv, "/admin/models/modes", body)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Create "fast" mode
	body = fmt.Sprintf(`{"model_id":%d,"mode":"fast","upstream_model":"grok-3","upstream_mode":"MODE_FAST"}`, fam.ID)
	rec = postJSON(t, srv, "/admin/models/modes", body)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Resolve("mymodel") → default mode
	resolved, ok := reg.Resolve("mymodel")
	require.True(t, ok)
	assert.Equal(t, "default", resolved.Mode.Mode)

	// Resolve("mymodel-fast") → fast mode
	resolved, ok = reg.Resolve("mymodel-fast")
	require.True(t, ok)
	assert.Equal(t, "fast", resolved.Mode.Mode)

	// Resolve("mymodel-default") → not found (default mode uses family name)
	_, ok = reg.Resolve("mymodel-default")
	assert.False(t, ok)
}

// BE-006: Pool floor override → GetPoolForModel returns correct pools.
func TestIntegration_PoolFloorOverride_TokenRouting(t *testing.T) {
	_, reg, srv := setupModelIntegrationStack(t, false)

	// Create family with pool_floor=basic
	rec := postJSON(t, srv, "/admin/models/families",
		`{"model":"route-test","display_name":"Route Test","type":"chat","pool_floor":"basic"}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	var fam FamilyResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&fam))

	// Create default mode
	body := fmt.Sprintf(`{"model_id":%d,"mode":"default","upstream_model":"grok-3","upstream_mode":"MODE_AUTO"}`, fam.ID)
	rec = postJSON(t, srv, "/admin/models/modes", body)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Create "heavy" mode with pool_floor_override=heavy
	body = fmt.Sprintf(`{"model_id":%d,"mode":"heavy","upstream_model":"grok-3","upstream_mode":"MODE_HEAVY","pool_floor_override":"heavy"}`, fam.ID)
	rec = postJSON(t, srv, "/admin/models/modes", body)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Default mode: floor=basic → all 3 pools
	floor, ok := reg.ResolvePoolFloor("route-test")
	require.True(t, ok)
	assert.Equal(t, "basic", floor)

	pools, ok := token.GetPoolForModel("route-test", reg)
	require.True(t, ok)
	assert.Equal(t, []string{"ssoBasic", "ssoSuper", "ssoHeavy"}, pools)

	// Heavy mode: floor=heavy → only ssoHeavy
	floor, ok = reg.ResolvePoolFloor("route-test-heavy")
	require.True(t, ok)
	assert.Equal(t, "heavy", floor)

	pools, ok = token.GetPoolForModel("route-test-heavy", reg)
	require.True(t, ok)
	assert.Equal(t, []string{"ssoHeavy"}, pools)
}

// --- P1 Integration Tests ---

// BE-007: Conflict detection — derived mode name collides with existing family name.
func TestIntegration_ConflictDetection_FamilyVsDerivedName(t *testing.T) {
	_, _, srv := setupModelIntegrationStack(t, false)

	// Create "grok-3" family + default mode
	rec := postJSON(t, srv, "/admin/models/families",
		`{"model":"grok-3","display_name":"Grok 3","type":"chat","pool_floor":"basic"}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	var fam FamilyResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&fam))

	body := fmt.Sprintf(`{"model_id":%d,"mode":"default","upstream_model":"grok-3","upstream_mode":"MODE_AUTO"}`, fam.ID)
	rec = postJSON(t, srv, "/admin/models/modes", body)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Create "mini" mode → derived name "grok-3-mini"
	body = fmt.Sprintf(`{"model_id":%d,"mode":"mini","upstream_model":"grok-3","upstream_mode":"MODE_MINI"}`, fam.ID)
	rec = postJSON(t, srv, "/admin/models/modes", body)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Try to create "grok-3-mini" family → 409 conflict
	rec = postJSON(t, srv, "/admin/models/families",
		`{"model":"grok-3-mini","display_name":"Grok 3 Mini","type":"chat","pool_floor":"basic"}`)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

// BE-008: Conflict detection — new mode name collides with existing family name.
func TestIntegration_ConflictDetection_ModeVsFamilyName(t *testing.T) {
	_, _, srv := setupModelIntegrationStack(t, false)

	// Create "grok-3-fast" family
	rec := postJSON(t, srv, "/admin/models/families",
		`{"model":"grok-3-fast","display_name":"Grok 3 Fast","type":"chat","pool_floor":"basic"}`)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Create "grok-3" family + default mode
	rec = postJSON(t, srv, "/admin/models/families",
		`{"model":"grok-3","display_name":"Grok 3","type":"chat","pool_floor":"basic"}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	var fam FamilyResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&fam))

	body := fmt.Sprintf(`{"model_id":%d,"mode":"default","upstream_model":"grok-3","upstream_mode":"MODE_AUTO"}`, fam.ID)
	rec = postJSON(t, srv, "/admin/models/modes", body)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Try to create "fast" mode on grok-3 → derived "grok-3-fast" conflicts with existing family
	body = fmt.Sprintf(`{"model_id":%d,"mode":"fast","upstream_model":"grok-3","upstream_mode":"MODE_FAST"}`, fam.ID)
	rec = postJSON(t, srv, "/admin/models/modes", body)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

// BE-009: Disabled family excluded from /v1/models.
func TestIntegration_DisabledFamily_ExcludedFromModels(t *testing.T) {
	_, _, srv := setupModelIntegrationStack(t, false)

	// Create enabled family
	rec := postJSON(t, srv, "/admin/models/families",
		`{"model":"enabled-fam","display_name":"Enabled","type":"chat","pool_floor":"basic","enabled":true}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	var enabledFam FamilyResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&enabledFam))

	body := fmt.Sprintf(`{"model_id":%d,"mode":"default","upstream_model":"grok-3","upstream_mode":"MODE_AUTO"}`, enabledFam.ID)
	rec = postJSON(t, srv, "/admin/models/modes", body)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Create disabled family
	rec = postJSON(t, srv, "/admin/models/families",
		`{"model":"disabled-fam","display_name":"Disabled","type":"chat","pool_floor":"basic","enabled":false}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	var disabledFam FamilyResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&disabledFam))

	body = fmt.Sprintf(`{"model_id":%d,"mode":"default","upstream_model":"grok-3","upstream_mode":"MODE_AUTO"}`, disabledFam.ID)
	rec = postJSON(t, srv, "/admin/models/modes", body)
	require.Equal(t, http.StatusCreated, rec.Code)

	ids := modelsResponseIDs(t, srv)
	assert.Contains(t, ids, "enabled-fam")
	assert.NotContains(t, ids, "disabled-fam")
}

// BE-010: Disabled mode excluded from /v1/models.
func TestIntegration_DisabledMode_ExcludedFromModels(t *testing.T) {
	_, _, srv := setupModelIntegrationStack(t, false)

	rec := postJSON(t, srv, "/admin/models/families",
		`{"model":"mode-test","display_name":"Mode Test","type":"chat","pool_floor":"basic","enabled":true}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	var fam FamilyResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&fam))

	// Create enabled default mode
	body := fmt.Sprintf(`{"model_id":%d,"mode":"default","upstream_model":"grok-3","upstream_mode":"MODE_AUTO","enabled":true}`, fam.ID)
	rec = postJSON(t, srv, "/admin/models/modes", body)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Create disabled "fast" mode
	body = fmt.Sprintf(`{"model_id":%d,"mode":"fast","upstream_model":"grok-3","upstream_mode":"MODE_FAST","enabled":false}`, fam.ID)
	rec = postJSON(t, srv, "/admin/models/modes", body)
	require.Equal(t, http.StatusCreated, rec.Code)

	ids := modelsResponseIDs(t, srv)
	assert.Contains(t, ids, "mode-test")
	assert.NotContains(t, ids, "mode-test-fast")
}

// BE-011: PUT family pool_floor basic→super → ResolvePoolFloor immediately returns "super".
func TestIntegration_UpdatePoolFloor_RegistryImmediate(t *testing.T) {
	_, reg, srv := setupModelIntegrationStack(t, false)

	rec := postJSON(t, srv, "/admin/models/families",
		`{"model":"floor-test","display_name":"Floor","type":"chat","pool_floor":"basic","enabled":true}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	var fam FamilyResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&fam))

	body := fmt.Sprintf(`{"model_id":%d,"mode":"default","upstream_model":"grok-3","upstream_mode":"MODE_AUTO","enabled":true}`, fam.ID)
	rec = postJSON(t, srv, "/admin/models/modes", body)
	require.Equal(t, http.StatusCreated, rec.Code)
	var mode store.ModelMode
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&mode))

	floor, ok := reg.ResolvePoolFloor("floor-test")
	require.True(t, ok)
	assert.Equal(t, "basic", floor)

	// Update pool_floor to super
	body = fmt.Sprintf(`{"model":"floor-test","display_name":"Floor","type":"chat","pool_floor":"super","default_mode_id":%d}`, mode.ID)
	rec = putJSON(t, srv, fmt.Sprintf("/admin/models/families/%d", fam.ID), body)
	require.Equal(t, http.StatusOK, rec.Code)

	floor, ok = reg.ResolvePoolFloor("floor-test")
	require.True(t, ok)
	assert.Equal(t, "super", floor)
}

// BE-012: POST create mode → GET /v1/models includes new derived name.
func TestIntegration_CreateMode_RegistryImmediate(t *testing.T) {
	_, _, srv := setupModelIntegrationStack(t, false)

	rec := postJSON(t, srv, "/admin/models/families",
		`{"model":"turbo-fam","display_name":"Turbo","type":"chat","pool_floor":"basic","enabled":true}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	var fam FamilyResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&fam))

	body := fmt.Sprintf(`{"model_id":%d,"mode":"default","upstream_model":"grok-3","upstream_mode":"MODE_AUTO","enabled":true}`, fam.ID)
	rec = postJSON(t, srv, "/admin/models/modes", body)
	require.Equal(t, http.StatusCreated, rec.Code)

	ids := modelsResponseIDs(t, srv)
	require.Contains(t, ids, "turbo-fam")
	require.NotContains(t, ids, "turbo-fam-turbo")

	// Add "turbo" mode
	body = fmt.Sprintf(`{"model_id":%d,"mode":"turbo","upstream_model":"grok-3","upstream_mode":"MODE_TURBO","enabled":true}`, fam.ID)
	rec = postJSON(t, srv, "/admin/models/modes", body)
	require.Equal(t, http.StatusCreated, rec.Code)

	ids = modelsResponseIDs(t, srv)
	assert.Contains(t, ids, "turbo-fam-turbo")
}

// BE-013: Image family mode rejects upstream mapping.
func TestIntegration_ImageModeRejectsUpstream(t *testing.T) {
	_, _, srv := setupModelIntegrationStack(t, false)

	rec := postJSON(t, srv, "/admin/models/families",
		`{"model":"img-reject","display_name":"Img","type":"image","pool_floor":"super","enabled":true}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	var fam FamilyResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&fam))

	// Try to create mode with upstream on image family → 400
	body := fmt.Sprintf(`{"model_id":%d,"mode":"default","upstream_model":"grok-3","upstream_mode":"MODE_FAST"}`, fam.ID)
	rec = postJSON(t, srv, "/admin/models/modes", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// BE-014: First mode must be named "default".
func TestIntegration_FirstModeMustBeDefault(t *testing.T) {
	_, _, srv := setupModelIntegrationStack(t, false)

	rec := postJSON(t, srv, "/admin/models/families",
		`{"model":"first-mode","display_name":"First","type":"chat","pool_floor":"basic","enabled":true}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	var fam FamilyResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&fam))

	// Try to create non-default first mode → 400
	body := fmt.Sprintf(`{"model_id":%d,"mode":"expert","upstream_model":"grok-3","upstream_mode":"MODE_EXPERT"}`, fam.ID)
	rec = postJSON(t, srv, "/admin/models/modes", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// BE-015: Delete default mode when other modes exist → rejected.
func TestIntegration_DeleteDefaultModeWithOthers_Rejected(t *testing.T) {
	_, _, srv := setupModelIntegrationStack(t, false)

	rec := postJSON(t, srv, "/admin/models/families",
		`{"model":"del-default","display_name":"Del","type":"chat","pool_floor":"basic","enabled":true}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	var fam FamilyResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&fam))

	// Create default mode
	body := fmt.Sprintf(`{"model_id":%d,"mode":"default","upstream_model":"grok-3","upstream_mode":"MODE_AUTO","enabled":true}`, fam.ID)
	rec = postJSON(t, srv, "/admin/models/modes", body)
	require.Equal(t, http.StatusCreated, rec.Code)
	var defaultMode store.ModelMode
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&defaultMode))

	// Create expert mode
	body = fmt.Sprintf(`{"model_id":%d,"mode":"expert","upstream_model":"grok-3","upstream_mode":"MODE_EXPERT","enabled":true}`, fam.ID)
	rec = postJSON(t, srv, "/admin/models/modes", body)
	require.Equal(t, http.StatusCreated, rec.Code)

	// Try to delete default mode → 400
	rec = doDelete(t, srv, fmt.Sprintf("/admin/models/modes/%d", defaultMode.ID))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
