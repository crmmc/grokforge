package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/httpapi"
	"github.com/crmmc/grokforge/internal/registry"
	"github.com/crmmc/grokforge/internal/store"
)

func newTestRegistry(t *testing.T) *registry.ModelRegistry {
	t.Helper()
	return registry.NewTestRegistry([]registry.TestFamilyWithModes{
		{
			Family: store.ModelFamily{ID: 1, Model: "grok-2", DisplayName: "Grok 2", PoolFloor: "basic", Enabled: true, DefaultModeID: ptrUint(1)},
			Modes:  []store.ModelMode{{ID: 1, ModelID: 1, Mode: "default", Enabled: true, UpstreamModel: "grok-2", UpstreamMode: "default"}},
		},
		{
			Family: store.ModelFamily{ID: 2, Model: "grok-2-mini", DisplayName: "Grok 2 Mini", PoolFloor: "basic", Enabled: true, DefaultModeID: ptrUint(2)},
			Modes:  []store.ModelMode{{ID: 2, ModelID: 2, Mode: "default", Enabled: true, UpstreamModel: "grok-2-mini", UpstreamMode: "default"}},
		},
		{
			Family: store.ModelFamily{ID: 3, Model: "grok-3", DisplayName: "Grok 3", PoolFloor: "basic", Enabled: true, DefaultModeID: ptrUint(3)},
			Modes: []store.ModelMode{
				{ID: 3, ModelID: 3, Mode: "default", Enabled: true, UpstreamModel: "grok-3", UpstreamMode: "default"},
				{ID: 5, ModelID: 3, Mode: "heavy", Enabled: true, UpstreamModel: "grok-3", UpstreamMode: "heavy"},
			},
		},
		{
			Family: store.ModelFamily{ID: 4, Model: "grok-3-mini", DisplayName: "Grok 3 Mini", PoolFloor: "basic", Enabled: true, DefaultModeID: ptrUint(4)},
			Modes:  []store.ModelMode{{ID: 4, ModelID: 4, Mode: "default", Enabled: true, UpstreamModel: "grok-3-mini", UpstreamMode: "default"}},
		},
	})
}

type modelsMockAPIKeyStore struct {
	key *store.APIKey
	err error
}

func (m *modelsMockAPIKeyStore) List(context.Context, int, int, string) ([]*store.APIKey, int64, error) {
	return nil, 0, nil
}

func (m *modelsMockAPIKeyStore) GetByID(context.Context, uint) (*store.APIKey, error) {
	return nil, errors.New("not implemented")
}

func (m *modelsMockAPIKeyStore) GetByKey(context.Context, string) (*store.APIKey, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.key, nil
}

func (m *modelsMockAPIKeyStore) Create(context.Context, *store.APIKey) error {
	return errors.New("not implemented")
}

func (m *modelsMockAPIKeyStore) Update(context.Context, *store.APIKey) error {
	return errors.New("not implemented")
}

func (m *modelsMockAPIKeyStore) Delete(context.Context, uint) error {
	return errors.New("not implemented")
}

func (m *modelsMockAPIKeyStore) Regenerate(context.Context, uint) (string, error) {
	return "", errors.New("not implemented")
}

func (m *modelsMockAPIKeyStore) CountByStatus(context.Context) (int, int, int, int, int, error) {
	return 0, 0, 0, 0, 0, errors.New("not implemented")
}

func (m *modelsMockAPIKeyStore) IncrementUsage(context.Context, uint) error {
	return errors.New("not implemented")
}

func (m *modelsMockAPIKeyStore) ResetDailyUsage(context.Context) error {
	return errors.New("not implemented")
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

	if len(resp.Data) != 5 {
		t.Errorf("expected 5 models, got %d", len(resp.Data))
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

func TestHandleModelsFromRegistry_AppliesModelWhitelist(t *testing.T) {
	reg := newTestRegistry(t)
	store := &modelsMockAPIKeyStore{
		key: &store.APIKey{
			ID:             1,
			Key:            "test-key",
			Status:         "active",
			ModelWhitelist: store.StringSlice{"grok-2-mini", "grok-3"},
		},
	}
	handler := httpapi.APIKeyAuth(store)(HandleModelsFromRegistry(reg))

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer test-key")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp ModelsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 models, got %d", len(resp.Data))
	}
	if resp.Data[0].ID != "grok-2-mini" || resp.Data[1].ID != "grok-3" {
		t.Fatalf("unexpected filtered models: %#v", resp.Data)
	}
}

func TestHandleModelsFromRegistry_ExposesNonDefaultModeRequestNames(t *testing.T) {
	reg := newTestRegistry(t)
	handler := HandleModelsFromRegistry(reg)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp ModelsResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	found := false
	for _, model := range resp.Data {
		if model.ID == "grok-3-heavy" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected non-default request name grok-3-heavy in models response, got %#v", resp.Data)
	}
}
