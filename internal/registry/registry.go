// Package registry provides an in-memory snapshot of enabled models
// for O(1) request name resolution on the hot path.
package registry

import (
	"context"
	"sync"

	"github.com/crmmc/grokforge/internal/store"
)

// ResolvedModel holds the resolved model info for a request name.
type ResolvedModel struct {
	RequestName    string
	Family         *store.ModelFamily
	Mode           *store.ModelMode
	EffectiveFloor string // mode PoolFloorOverride > family PoolFloor
	UpstreamModel  string
	UpstreamMode   string
}

// ModelRegistry maintains an in-memory snapshot of enabled models.
type ModelRegistry struct {
	mu            sync.RWMutex
	store         *store.ModelStore
	byRequestName map[string]*ResolvedModel
	enabledByType map[string][]*ResolvedModel
}

// NewModelRegistry creates a new ModelRegistry. Call Refresh to load data.
func NewModelRegistry(modelStore *store.ModelStore) *ModelRegistry {
	return &ModelRegistry{
		store:         modelStore,
		byRequestName: make(map[string]*ResolvedModel),
		enabledByType: make(map[string][]*ResolvedModel),
	}
}

// Resolve looks up a request name and returns the resolved model.
func (r *ModelRegistry) Resolve(requestName string) (*ResolvedModel, bool) {
	return nil, false
}

// EnabledByType returns all enabled models of a given type.
func (r *ModelRegistry) EnabledByType(typ string) []*ResolvedModel {
	return nil
}

// AllEnabled returns all enabled models.
func (r *ModelRegistry) AllEnabled() []*ResolvedModel {
	return nil
}

// Count returns the number of enabled models in the registry.
func (r *ModelRegistry) Count() int {
	return 0
}

// Refresh reloads enabled models from the database and rebuilds indexes.
func (r *ModelRegistry) Refresh(ctx context.Context) error {
	return nil
}
