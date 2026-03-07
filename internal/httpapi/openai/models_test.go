package openai

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/config"
)

func testTokenConfig() *config.TokenConfig {
	return &config.TokenConfig{
		BasicModels: []string{"grok-2", "grok-2-mini"},
		SuperModels: []string{"grok-3", "grok-3-mini"},
	}
}

func TestNewModelRegistryFromConfig(t *testing.T) {
	cfg := testTokenConfig()
	registry := NewModelRegistryFromConfig(cfg)

	// Should contain exactly the models from config
	allModels := registry.All()
	if len(allModels) != 4 {
		t.Errorf("expected 4 models, got %d", len(allModels))
	}

	// Basic models should exist in the registry.
	for _, m := range []string{"grok-2", "grok-2-mini"} {
		if _, ok := registry.models[m]; !ok {
			t.Errorf("expected model %s in registry", m)
		}
	}

	// Super models should exist in the registry.
	for _, m := range []string{"grok-3", "grok-3-mini"} {
		if _, ok := registry.models[m]; !ok {
			t.Errorf("expected model %s in registry", m)
		}
	}

	// Unknown model should not be in registry
	if _, ok := registry.models["not-a-model"]; ok {
		t.Error("expected unknown model to not be in registry")
	}
}

func TestNewModelRegistryFromConfig_FullDefaults(t *testing.T) {
	cfg := config.DefaultConfig()
	registry := NewModelRegistryFromConfig(&cfg.Token)

	// Unique models from default config.
	want := len(registry.models)
	allModels := registry.All()
	if len(allModels) != want {
		t.Errorf("expected %d models from default config, got %d", want, len(allModels))
	}
}

func TestHandleModels_ReturnsOpenAIFormat(t *testing.T) {
	cfg := testTokenConfig()
	registry := NewModelRegistryFromConfig(cfg)
	handler := HandleModels(registry)

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

func TestHandleModels_ModelEntryFields(t *testing.T) {
	cfg := testTokenConfig()
	registry := NewModelRegistryFromConfig(cfg)
	handler := HandleModels(registry)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var resp ModelsResponse
	json.NewDecoder(rec.Body).Decode(&resp)

	// Find grok-3 in response
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

func TestNewModelRegistryFromConfig_StripsCostSuffix(t *testing.T) {
	cfg := &config.TokenConfig{
		BasicModels: []string{"grok-2", "grok-2-mini"},
		SuperModels: []string{"grok-3-thinking#4", "grok-4-heavy#4", "grok-3"},
	}
	registry := NewModelRegistryFromConfig(cfg)

	// "#N" should be stripped — lookup by clean name
	for _, name := range []string{"grok-3-thinking", "grok-4-heavy", "grok-3"} {
		if _, ok := registry.models[name]; !ok {
			t.Errorf("expected model %s in registry (cost suffix stripped)", name)
		}
	}
	// Raw entry with suffix should NOT exist
	for _, raw := range []string{"grok-3-thinking#4", "grok-4-heavy#4"} {
		if _, ok := registry.models[raw]; ok {
			t.Errorf("model %s should not be in registry (cost suffix not stripped)", raw)
		}
	}
}

func TestHandleModels_ExcludesTierFromResponse(t *testing.T) {
	cfg := testTokenConfig()
	registry := NewModelRegistryFromConfig(cfg)
	handler := HandleModels(registry)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Decode to map to check for tier field
	var raw map[string]interface{}
	json.Unmarshal(rec.Body.Bytes(), &raw)

	data, ok := raw["data"].([]interface{})
	if !ok {
		t.Fatal("data field not found or not array")
	}

	for _, item := range data {
		model, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if _, hasTier := model["tier"]; hasTier {
			t.Error("tier field should not be exposed in API response")
		}
	}
}
