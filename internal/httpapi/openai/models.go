package openai

import (
	"net/http"
	"sort"

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
	Type    string `json:"type,omitempty"`
}

// HandleModelsFromRegistry returns a handler that lists models from the model registry.
func HandleModelsFromRegistry(reg *registry.ModelRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		all := reg.AllEnabled()
		created := int64(1709251200) // 2024-03-01
		entries := make([]ModelEntry, 0, len(all))
		for _, rm := range all {
			if !httpapi.CheckModelWhitelist(r.Context(), rm.RequestName) {
				continue
			}
			typ := ""
			if rm.Family != nil {
				typ = rm.Family.Type
			}
			entries = append(entries, ModelEntry{
				ID:      rm.RequestName,
				Object:  "model",
				Created: created,
				OwnedBy: "xai",
				Type:    typ,
			})
		}
		// Sort by ID for stable output
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].ID < entries[j].ID
		})
		resp := ModelsResponse{
			Object: "list",
			Data:   entries,
		}
		httpapi.WriteJSON(w, http.StatusOK, resp)
	}
}
