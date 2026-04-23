package token

import (
	"log/slog"
	"sort"

	"github.com/crmmc/grokforge/internal/modelconfig"
	"github.com/crmmc/grokforge/internal/store"
)

type normalizationResult struct {
	changed   bool
	zeroModes []string
}

func normalizeTokenQuotas(token *store.Token, modes []modelconfig.ModeSpec) normalizationResult {
	if token == nil {
		return normalizationResult{}
	}

	result := normalizationResult{}
	if token.Quotas == nil {
		token.Quotas = make(store.IntMap)
		result.changed = true
	}
	if token.LimitQuotas == nil {
		token.LimitQuotas = make(store.IntMap)
		result.changed = true
	}

	modeByID := make(map[string]modelconfig.ModeSpec, len(modes))
	for _, mode := range modes {
		modeByID[mode.ID] = mode
	}

	for key := range token.Quotas {
		if _, ok := modeByID[key]; ok {
			continue
		}
		delete(token.Quotas, key)
		result.changed = true
	}
	for key := range token.LimitQuotas {
		if _, ok := modeByID[key]; ok {
			continue
		}
		delete(token.LimitQuotas, key)
		result.changed = true
	}

	poolKey := PoolToShort(token.Pool)
	for _, mode := range modes {
		defaultQuota := mode.DefaultQuota[poolKey]
		if defaultQuota <= 0 {
			continue
		}

		limit, ok := token.LimitQuotas[mode.ID]
		if !ok {
			limit = defaultQuota
			token.LimitQuotas[mode.ID] = limit
			result.changed = true
		}

		quota, ok := token.Quotas[mode.ID]
		if !ok {
			quota = limit
			token.Quotas[mode.ID] = quota
			result.changed = true
		}

		if quota > limit {
			slog.Warn("token: normalization clamp",
				"token_id", token.ID,
				"pool", token.Pool,
				"mode", mode.ID,
				"action", "normalization_clamp",
				"remaining", quota,
				"limit", limit)
			token.Quotas[mode.ID] = limit
			quota = limit
			result.changed = true
		}

		if quota == 0 {
			result.zeroModes = append(result.zeroModes, mode.ID)
		}
	}

	sort.Strings(result.zeroModes)
	return result
}
