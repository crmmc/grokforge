package token

import (
	"errors"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
)

// ErrModelNotFound is returned when the model is not in any configured group.
var ErrModelNotFound = errors.New("model not found")

// modelInList checks if a model name matches any entry in the list,
// ignoring the optional #cost suffix.
func modelInList(model string, list []string) bool {
	for _, entry := range list {
		name, _ := ParseModelEntry(entry)
		if name == model {
			return true
		}
	}
	return false
}

// GetPoolForModel returns the pool name for a given model based on config.
// Returns ("", false) if the model is not in any configured group.
func GetPoolForModel(model string, cfg *config.TokenConfig) (string, bool) {
	if model == "" {
		return "", false
	}

	inBasic := modelInList(model, cfg.BasicModels)
	inSuper := modelInList(model, cfg.SuperModels)

	if !inBasic && !inSuper {
		return "", false
	}
	if inBasic && inSuper {
		return cfg.PreferredPool, true
	}
	if inSuper {
		return PoolSuper, true
	}
	return PoolBasic, true
}

// GetPoolsForModel returns the preferred pool and an optional fallback pool.
// When the model is in both pools, the non-preferred pool is the fallback.
func GetPoolsForModel(model string, cfg *config.TokenConfig) (primary, fallback string, ok bool) {
	primary, ok = GetPoolForModel(model, cfg)
	if !ok {
		return "", "", false
	}
	inBasic := modelInList(model, cfg.BasicModels)
	inSuper := modelInList(model, cfg.SuperModels)
	if inBasic && inSuper {
		if primary == PoolBasic {
			fallback = PoolSuper
		} else {
			fallback = PoolBasic
		}
	}
	return primary, fallback, true
}

// PickForModel selects a token from the appropriate pool for the given model.
// When the model exists in both pools and the preferred pool has no tokens,
// it falls back to the other pool automatically.
// Returns ErrModelNotFound if the model is not in any configured group.
func (m *TokenManager) PickForModel(model string, cfg *config.TokenConfig, cat QuotaCategory) (*store.Token, error) {
	pool, ok := GetPoolForModel(model, cfg)
	if !ok {
		return nil, ErrModelNotFound
	}
	tok, err := m.Pick(pool, cat)
	if err == nil {
		return tok, nil
	}
	// Fallback: if model is in both pools, try the other one.
	inBasic := modelInList(model, cfg.BasicModels)
	inSuper := modelInList(model, cfg.SuperModels)
	if inBasic && inSuper {
		alt := PoolBasic
		if pool == PoolBasic {
			alt = PoolSuper
		}
		if altTok, altErr := m.Pick(alt, cat); altErr == nil {
			return altTok, nil
		}
	}
	return nil, err
}
