// Package registry provides an in-memory snapshot of enabled models
// for O(1) request name resolution on the hot path.
package registry

import (
	"context"
	"fmt"
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
	r.mu.RLock()
	defer r.mu.RUnlock()
	rm, ok := r.byRequestName[requestName]
	return rm, ok
}

// EnabledByType returns all enabled models of a given type.
// Returns an empty non-nil slice if no models match.
func (r *ModelRegistry) EnabledByType(typ string) []*ResolvedModel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	src := r.enabledByType[typ]
	out := make([]*ResolvedModel, len(src))
	copy(out, src)
	return out
}

// AllEnabled returns all enabled models as a slice.
func (r *ModelRegistry) AllEnabled() []*ResolvedModel {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*ResolvedModel, 0, len(r.byRequestName))
	for _, rm := range r.byRequestName {
		out = append(out, rm)
	}
	return out
}

// Count returns the number of enabled models in the registry.
func (r *ModelRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byRequestName)
}

// Refresh reloads enabled models from the database and rebuilds indexes.
// DB queries run outside the write lock; the lock is held only for the
// pointer swap (copy-on-write pattern).
func (r *ModelRegistry) Refresh(ctx context.Context) error {
	families, err := r.store.ListEnabledFamilies(ctx)
	if err != nil {
		return fmt.Errorf("list enabled families: %w", err)
	}

	newByName := make(map[string]*ResolvedModel)
	newByType := make(map[string][]*ResolvedModel)

	for _, family := range families {
		modes, err := r.store.ListModesByFamily(ctx, family.ID)
		if err != nil {
			return fmt.Errorf("list modes for family %s: %w", family.Model, err)
		}

		for _, mode := range modes {
			if !mode.Enabled {
				continue
			}

			isDefault := family.DefaultModeID != nil && *family.DefaultModeID == mode.ID
			requestName := store.DeriveRequestName(family.Model, mode.Mode, isDefault)

			effectiveFloor := family.PoolFloor
			if mode.PoolFloorOverride != nil {
				effectiveFloor = *mode.PoolFloorOverride
			}

			rm := &ResolvedModel{
				RequestName:    requestName,
				Family:         family,
				Mode:           mode,
				EffectiveFloor: effectiveFloor,
				UpstreamModel:  mode.UpstreamModel,
				UpstreamMode:   mode.UpstreamMode,
			}

			newByName[requestName] = rm
			newByType[family.Type] = append(newByType[family.Type], rm)
		}
	}

	r.mu.Lock()
	r.byRequestName = newByName
	r.enabledByType = newByType
	r.mu.Unlock()

	return nil
}
