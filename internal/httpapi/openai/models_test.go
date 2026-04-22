package openai

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/httpapi"
	"github.com/crmmc/grokforge/internal/modelconfig"
	"github.com/crmmc/grokforge/internal/registry"
	"github.com/crmmc/grokforge/internal/store"
)

func newTestRegistry(t *testing.T) *registry.ModelRegistry {
	t.Helper()
	return registry.NewTestRegistry([]modelconfig.ModelSpec{
		{ID: "grok-2", DisplayName: "Grok 2", Type: modelconfig.TypeChat, Enabled: true, PoolFloor: modelconfig.PoolBasic, Mode: "auto", UpstreamModel: "grok-2", PublicType: "chat"},
		{ID: "grok-2-mini", DisplayName: "Grok 2 Mini", Type: modelconfig.TypeChat, Enabled: true, PoolFloor: modelconfig.PoolBasic, Mode: "auto", UpstreamModel: "grok-2-mini", PublicType: "chat"},
		{ID: "grok-3", DisplayName: "Grok 3", Type: modelconfig.TypeChat, Enabled: true, PoolFloor: modelconfig.PoolBasic, Mode: "auto", UpstreamModel: "grok-3", PublicType: "chat"},
		{ID: "grok-3-heavy", DisplayName: "Grok 3 Heavy", Type: modelconfig.TypeChat, Enabled: true, PoolFloor: modelconfig.PoolHeavy, Mode: "heavy", UpstreamModel: "grok-3", UpstreamMode: "heavy", PublicType: "chat"},
		{ID: "grok-3-mini", DisplayName: "Grok 3 Mini", Type: modelconfig.TypeChat, Enabled: true, PoolFloor: modelconfig.PoolBasic, Mode: "auto", UpstreamModel: "grok-3-mini", PublicType: "chat"},
	}, nil)
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

func TestHandleModelsFromRegistry_ExposesAllRegisteredModels(t *testing.T) {
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
		t.Fatalf("expected grok-3-heavy in models response, got %#v", resp.Data)
	}
}
