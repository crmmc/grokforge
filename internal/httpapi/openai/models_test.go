package openai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/registry"
	"github.com/crmmc/grokforge/internal/store"
)

func newTestRegistry(t *testing.T) *registry.ModelRegistry {
	t.Helper()
	return registry.NewTestRegistry([]registry.TestFamilyWithModes{
		{
			Family: store.ModelFamily{ID: 1, Model: "grok-2", DisplayName: "Grok 2", PoolFloor: "basic", Enabled: true},
			Modes:  []store.ModelMode{{ID: 1, ModelID: 1, Mode: "default", Enabled: true, UpstreamModel: "grok-2", UpstreamMode: "default", QuotaCost: 1}},
		},
		{
			Family: store.ModelFamily{ID: 2, Model: "grok-2-mini", DisplayName: "Grok 2 Mini", PoolFloor: "basic", Enabled: true},
			Modes:  []store.ModelMode{{ID: 2, ModelID: 2, Mode: "default", Enabled: true, UpstreamModel: "grok-2-mini", UpstreamMode: "default", QuotaCost: 1}},
		},
		{
			Family: store.ModelFamily{ID: 3, Model: "grok-3", DisplayName: "Grok 3", PoolFloor: "basic", Enabled: true},
			Modes:  []store.ModelMode{{ID: 3, ModelID: 3, Mode: "default", Enabled: true, UpstreamModel: "grok-3", UpstreamMode: "default", QuotaCost: 1}},
		},
		{
			Family: store.ModelFamily{ID: 4, Model: "grok-3-mini", DisplayName: "Grok 3 Mini", PoolFloor: "basic", Enabled: true},
			Modes:  []store.ModelMode{{ID: 4, ModelID: 4, Mode: "default", Enabled: true, UpstreamModel: "grok-3-mini", UpstreamMode: "default", QuotaCost: 1}},
		},
	})
}

func TestHandleModelsFromRegistry_ReturnsOpenAIFormat(t *testing.T) {
	reg := newTestRegistry(t)
	handler := HandleModelsFromRegistry(reg)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp ModelsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Object != "list" {
		t.Errorf("expected object 'list', got %s", resp.Object)
	}

	if len(resp.Data) != 4 {
		t.Errorf("expected 4 models, got %d", len(resp.Data))
	}
}

func TestHandleModelsFromRegistry_ModelEntryFields(t *testing.T) {
	reg := newTestRegistry(t)
	handler := HandleModelsFromRegistry(reg)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp ModelsResponse
	json.NewDecoder(rec.Body).Decode(&resp)

	var found *ModelEntry
	for i := range resp.Data {
		if resp.Data[i].ID == "grok-3" {
			found = &resp.Data[i]
			break
		}
	}

	if found == nil {
		t.Fatal("grok-3 not found in response")
	}
	if found.Object != "model" {
		t.Errorf("expected object 'model', got %s", found.Object)
	}
	if found.OwnedBy != "xai" {
		t.Errorf("expected owned_by 'xai', got %s", found.OwnedBy)
	}
	if found.Created == 0 {
		t.Error("expected non-zero created timestamp")
	}
}
