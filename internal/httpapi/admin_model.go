package httpapi

import (
	"net/http"
	"sort"

	"github.com/crmmc/grokforge/internal/registry"
)

// modelCatalogEntry is the JSON response for a single model in the catalog.
type modelCatalogEntry struct {
	ID              string `json:"id"`
	DisplayName     string `json:"display_name"`
	Type            string `json:"type"`
	PublicType      string `json:"public_type"`
	PoolFloor       string `json:"pool_floor"`
	Mode            string `json:"mode,omitempty"`
	QuotaSync       bool   `json:"quota_sync"`
	CooldownSeconds int    `json:"cooldown_seconds,omitempty"`
	UpstreamModel   string `json:"upstream_model,omitempty"`
	UpstreamMode    string `json:"upstream_mode,omitempty"`
	ForceThinking   bool   `json:"force_thinking,omitempty"`
	EnablePro       bool   `json:"enable_pro,omitempty"`
	Enabled         bool   `json:"enabled"`
}

type modeGroupEntry struct {
	Mode          string         `json:"mode"`
	DisplayName   string         `json:"display_name"`
	UpstreamName  string         `json:"upstream_name"`
	WindowSeconds int            `json:"window_seconds"`
	DefaultQuotas map[string]int `json:"default_quotas"`
	Models        []string       `json:"models"`
}

type modelCatalogResponse struct {
	Models     []modelCatalogEntry `json:"models"`
	ModeGroups []modeGroupEntry    `json:"mode_groups"`
}

// handleListModels returns a handler that lists models and mode-group metadata.
func handleListModels(reg *registry.ModelRegistry) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		models := make([]modelCatalogEntry, 0)
		all := reg.AllEnabled()
		for _, rm := range all {
			models = append(models, modelCatalogEntry{
				ID:              rm.ID,
				DisplayName:     rm.DisplayName,
				Type:            rm.Type,
				PublicType:      rm.PublicType,
				PoolFloor:       rm.PoolFloor,
				Mode:            rm.Mode,
				QuotaSync:       rm.QuotaSync,
				CooldownSeconds: rm.CooldownSeconds,
				UpstreamModel:   rm.UpstreamModel,
				UpstreamMode:    rm.UpstreamMode,
				ForceThinking:   rm.ForceThinking,
				EnablePro:       rm.EnablePro,
				Enabled:         rm.Enabled,
			})
		}
		sort.Slice(models, func(i, j int) bool {
			return models[i].ID < models[j].ID
		})

		modeGroups := make([]modeGroupEntry, 0)
		for _, mode := range reg.AllModes() {
			group := modeGroupEntry{
				Mode:          mode.ID,
				DisplayName:   mode.ID,
				UpstreamName:  mode.UpstreamName,
				WindowSeconds: mode.WindowSeconds,
				DefaultQuotas: copyDefaultQuotas(mode.DefaultQuota),
			}
			for _, rm := range all {
				if !rm.QuotaSync || rm.Mode != mode.ID {
					continue
				}
				group.Models = append(group.Models, rm.ID)
			}
			sort.Strings(group.Models)
			modeGroups = append(modeGroups, group)
		}

		WriteJSON(w, http.StatusOK, modelCatalogResponse{
			Models:     models,
			ModeGroups: modeGroups,
		})
	}
}

func copyDefaultQuotas(src map[string]int) map[string]int {
	dst := make(map[string]int, len(src))
	for key, val := range src {
		dst[key] = val
	}
	return dst
}
