package httpapi

import (
	"net/http"
	"sort"

	"github.com/crmmc/grokforge/internal/registry"
)

// modelCatalogEntry is the JSON response for a single model in the catalog.
type modelCatalogEntry struct {
	ID            string `json:"id"`
	DisplayName   string `json:"display_name"`
	Type          string `json:"type"`
	PublicType    string `json:"public_type"`
	PoolFloor     string `json:"pool_floor"`
	Mode          string `json:"mode"`
	UpstreamModel string `json:"upstream_model,omitempty"`
	UpstreamMode  string `json:"upstream_mode,omitempty"`
	ForceThinking bool   `json:"force_thinking,omitempty"`
	EnablePro     bool   `json:"enable_pro,omitempty"`
	Enabled       bool   `json:"enabled"`
}

// handleListModels returns a handler that lists all models from the registry.
func handleListModels(reg *registry.ModelRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		all := reg.AllEnabled()
		entries := make([]modelCatalogEntry, 0, len(all))
		for _, rm := range all {
			entries = append(entries, modelCatalogEntry{
				ID:            rm.ID,
				DisplayName:   rm.DisplayName,
				Type:          rm.Type,
				PublicType:    rm.PublicType,
				PoolFloor:     rm.PoolFloor,
				Mode:          rm.Mode,
				UpstreamModel: rm.UpstreamModel,
				UpstreamMode:  rm.UpstreamMode,
				ForceThinking: rm.ForceThinking,
				EnablePro:     rm.EnablePro,
				Enabled:       rm.Enabled,
			})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].ID < entries[j].ID
		})
		WriteJSON(w, http.StatusOK, entries)
	}
}
