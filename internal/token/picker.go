package token

import (
	"errors"

	"github.com/crmmc/grokforge/internal/store"
)

// ErrModelNotFound is returned when the model is not in any configured group.
var ErrModelNotFound = errors.New("model not found")

// ModelResolver resolves a model request name to pool floor info.
type ModelResolver interface {
	ResolvePoolFloor(requestName string) (floor string, ok bool)
}

// GetPoolForModel returns the eligible pool names for a given model based on its pool_floor.
// Pools are returned in ascending level order (basic → super → heavy).
// Uses >= matching: a model with pool_floor=basic can use basic, super, and heavy pools.
// Returns (nil, false) if the model is not found or the model name is empty.
func GetPoolForModel(model string, resolver ModelResolver) ([]string, bool) {
	if model == "" || resolver == nil {
		return nil, false
	}

	floor, ok := resolver.ResolvePoolFloor(model)
	if !ok {
		return nil, false
	}

	floorLevel := PoolLevelFor(floor)
	if floorLevel == 0 {
		return nil, false
	}

	var pools []string
	for _, name := range AllPoolNames() {
		if PoolLevelFor(name) >= floorLevel {
			pools = append(pools, name)
		}
	}

	if len(pools) == 0 {
		return nil, false
	}
	return pools, true
}

// PickForModel selects a token by trying each eligible pool in order.
// Returns the first available token, or ErrNoTokenAvailable if all pools are exhausted.
// Returns ErrModelNotFound if the model is not found in the resolver.
func (m *TokenManager) PickForModel(model string, resolver ModelResolver, cat QuotaCategory) (*store.Token, error) {
	pools, ok := GetPoolForModel(model, resolver)
	if !ok {
		return nil, ErrModelNotFound
	}

	var lastErr error
	for _, pool := range pools {
		tok, err := m.Pick(pool, cat)
		if err == nil {
			return tok, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrNoTokenAvailable
}
