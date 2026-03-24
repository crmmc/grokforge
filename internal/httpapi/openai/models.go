package openai

import (
	"net/http"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/httpapi"
	tkn "github.com/crmmc/grokforge/internal/token"
)

// ModelsResponse is the OpenAI models list response.
type ModelsResponse struct {
	Object string       `json:"object"`
	Data   []ModelEntry `json:"data"`
}

// ModelEntry represents a single model in the OpenAI models list.
type ModelEntry struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

// ModelInfo holds model metadata.
type ModelInfo struct {
	ID      string
	Object  string
	Created int64
	OwnedBy string
}

// ModelRegistry holds available models.
type ModelRegistry struct {
	models map[string]ModelInfo
}

// NewModelRegistry creates an empty model registry.
func NewModelRegistry() *ModelRegistry {
	return &ModelRegistry{
		models: make(map[string]ModelInfo),
	}
}

// Add adds a model to the registry.
func (r *ModelRegistry) Add(info ModelInfo) {
	r.models[info.ID] = info
}

// All returns all models in the registry.
func (r *ModelRegistry) All() []ModelInfo {
	result := make([]ModelInfo, 0, len(r.models))
	for _, info := range r.models {
		result = append(result, info)
	}
	return result
}

// NewModelRegistryFromConfig builds a model registry from config.
// Model entries with "#N" cost suffixes are stripped to expose clean model IDs.
func NewModelRegistryFromConfig(cfg *config.TokenConfig) *ModelRegistry {
	r := NewModelRegistry()
	created := int64(1709251200) // 2024-03-01

	for _, m := range cfg.BasicModels {
		name, _ := tkn.ParseModelEntry(m)
		r.Add(ModelInfo{ID: name, Object: "model", Created: created, OwnedBy: "xai"})
	}
	for _, m := range cfg.SuperModels {
		name, _ := tkn.ParseModelEntry(m)
		r.Add(ModelInfo{ID: name, Object: "model", Created: created, OwnedBy: "xai"})
	}
	return r
}

// HandleModels returns an http.HandlerFunc that lists all models.
// The response follows OpenAI's /v1/models format.
func HandleModels(registry *ModelRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		models := registry.All()
		entries := make([]ModelEntry, len(models))

		for i, m := range models {
			entries[i] = ModelEntry{
				ID:      m.ID,
				Object:  m.Object,
				Created: m.Created,
				OwnedBy: m.OwnedBy,
			}
		}

		resp := ModelsResponse{
			Object: "list",
			Data:   entries,
		}
		httpapi.WriteJSON(w, http.StatusOK, resp)
	}
}

// HandleModelsFromConfig returns a handler that rebuilds the model list from config
// on each request, enabling hot-reload of model groups.
func HandleModelsFromConfig(cfg *config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		registry := NewModelRegistryFromConfig(&cfg.Token)
		HandleModels(registry)(w, r)
	}
}

func HandleModelsFromRuntime(runtime *config.Runtime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cfg := runtime.Get()
		if cfg == nil {
			httpapi.WriteJSON(w, http.StatusOK, ModelsResponse{Object: "list", Data: []ModelEntry{}})
			return
		}
		registry := NewModelRegistryFromConfig(&cfg.Token)
		HandleModels(registry)(w, r)
	}
}
