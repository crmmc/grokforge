package openai

import (
	"net/http"

	"github.com/crmmc/grokforge/internal/httpapi"
	"github.com/crmmc/grokforge/internal/registry"
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

// HandleModelsFromRegistry returns a handler that lists models from the model registry.
func HandleModelsFromRegistry(reg *registry.ModelRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		models := reg.AllRequestNames()
		created := int64(1709251200) // 2024-03-01
		entries := make([]ModelEntry, len(models))
		for i, name := range models {
			entries[i] = ModelEntry{
				ID:      name,
				Object:  "model",
				Created: created,
				OwnedBy: "xai",
			}
		}
		resp := ModelsResponse{
			Object: "list",
			Data:   entries,
		}
		httpapi.WriteJSON(w, http.StatusOK, resp)
	}
}
