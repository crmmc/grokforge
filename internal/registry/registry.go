// Package registry provides an in-memory snapshot of enabled models
// for O(1) request name resolution on the hot path.
package registry

import (
	"context"
	"fmt"
	"sort"
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
	ForceThinking  bool
	EnablePro      bool
}

// ModelRegistry maintains an in-memory snapshot of enabled models.
type ModelRegistry struct {
	mu            sync.RWMutex
	store         *store.ModelStore
	byRequestName map[string]*ResolvedModel
	enabledByType map[string][]*ResolvedModel
}

// requiresUpstreamModel returns true if the type needs upstream_model on the family.
func requiresUpstreamModel(modelType string) bool {
	return modelType != "image_ws" && modelType != "image"
}

// requiresUpstreamMode returns true if the type needs upstream_mode on modes.
func requiresUpstreamMode(modelType string) bool {
	return modelType != "image_ws"
}

type modelReader interface {
	ListEnabledFamilies(ctx context.Context) ([]*store.ModelFamily, error)
	ListModesByFamily(ctx context.Context, familyID uint) ([]*store.ModelMode, error)
}

type committer interface {
	Commit() error
}

// ModelSnapshot is a prebuilt registry snapshot ready for installation.
type ModelSnapshot struct {
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

// TestFamilyWithModes is a test helper struct that bundles a family with its modes.
type TestFamilyWithModes struct {
	Family store.ModelFamily
	Modes  []store.ModelMode
}

// NewTestRegistry creates a pre-populated ModelRegistry for testing.
// It does not require a store — data is loaded directly from the provided families.
func NewTestRegistry(data []TestFamilyWithModes) *ModelRegistry {
	r := &ModelRegistry{
		byRequestName: make(map[string]*ResolvedModel),
		enabledByType: make(map[string][]*ResolvedModel),
	}
	for i := range data {
		family := &data[i].Family
		for j := range data[i].Modes {
			mode := &data[i].Modes[j]
			if !mode.Enabled {
				continue
			}
			isDefault := family.DefaultModeID != nil && *family.DefaultModeID == mode.ID
			requestName := store.DeriveRequestName(family.Model, mode.Mode, isDefault)
			effectiveFloor := family.PoolFloor
			if mode.PoolFloorOverride != nil && *mode.PoolFloorOverride != "" {
				effectiveFloor = *mode.PoolFloorOverride
			}
			rm := &ResolvedModel{
				RequestName:    requestName,
				Family:         family,
				Mode:           mode,
				EffectiveFloor: effectiveFloor,
				UpstreamModel:  family.UpstreamModel,
				UpstreamMode:   mode.UpstreamMode,
				ForceThinking:  mode.ForceThinking,
				EnablePro:      mode.EnablePro,
			}
			r.byRequestName[requestName] = rm
			r.enabledByType[family.Type] = append(r.enabledByType[family.Type], rm)
		}
	}
	return r
}

// Resolve looks up a request name and returns the resolved model.
func (r *ModelRegistry) Resolve(requestName string) (*ResolvedModel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rm, ok := r.byRequestName[requestName]
	return rm, ok
}

// AllRequestNames returns all registered request names (sorted).
func (r *ModelRegistry) AllRequestNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.byRequestName))
	for name := range r.byRequestName {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ResolvePoolFloor implements token.ModelResolver.
// Returns the effective pool floor for a request name.
func (r *ModelRegistry) ResolvePoolFloor(requestName string) (floor string, ok bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rm, found := r.byRequestName[requestName]
	if !found {
		return "", false
	}
	return rm.EffectiveFloor, true
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

// BuildSnapshotFromStore loads enabled models from a store view and builds a snapshot.
func (r *ModelRegistry) BuildSnapshotFromStore(ctx context.Context, reader modelReader) (*ModelSnapshot, error) {
	return buildSnapshot(ctx, reader)
}

// CommitAndApply commits a mutation transaction and atomically swaps the registry snapshot.
func (r *ModelRegistry) CommitAndApply(tx committer, snapshot *ModelSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := tx.Commit(); err != nil {
		return err
	}
	r.applySnapshotLocked(snapshot)
	return nil
}

// Refresh reloads enabled models from the database and rebuilds indexes.
// DB queries run outside the write lock; the lock is held only for the
// pointer swap (copy-on-write pattern).
func (r *ModelRegistry) Refresh(ctx context.Context) error {
	snapshot, err := buildSnapshot(ctx, r.store)
	if err != nil {
		return err
	}

	r.mu.Lock()
	r.applySnapshotLocked(snapshot)
	r.mu.Unlock()

	return nil
}

func buildSnapshot(ctx context.Context, reader modelReader) (*ModelSnapshot, error) {
	families, err := reader.ListEnabledFamilies(ctx)
	if err != nil {
		return nil, fmt.Errorf("list enabled families: %w", err)
	}

	newByName := make(map[string]*ResolvedModel)
	newByType := make(map[string][]*ResolvedModel)

	for _, family := range families {
		modes, err := reader.ListModesByFamily(ctx, family.ID)
		if err != nil {
			return nil, fmt.Errorf("list modes for family %s: %w", family.Model, err)
		}
		if len(modes) > 0 && family.DefaultModeID == nil {
			return nil, fmt.Errorf("family %s has modes but no default_mode_id", family.Model)
		}

		defaultFound := false
		for _, mode := range modes {
			if !mode.Enabled {
				continue
			}

			isDefault := family.DefaultModeID != nil && *family.DefaultModeID == mode.ID
			if isDefault {
				defaultFound = true
			}
			requestName := store.DeriveRequestName(family.Model, mode.Mode, isDefault)
			if _, exists := newByName[requestName]; exists {
				return nil, fmt.Errorf("duplicate request name in registry: %s", requestName)
			}
			if requiresUpstreamModel(family.Type) {
				if family.UpstreamModel == "" {
					return nil, fmt.Errorf("family %s has empty upstream_model", family.Model)
				}
			} else if family.UpstreamModel != "" {
				return nil, fmt.Errorf("family %s must not define upstream_model for type %s", family.Model, family.Type)
			}
			if requiresUpstreamMode(family.Type) {
				if mode.UpstreamMode == "" {
					return nil, fmt.Errorf("mode %s for family %s has empty upstream_mode", mode.Mode, family.Model)
				}
			} else if mode.UpstreamMode != "" {
				return nil, fmt.Errorf("mode %s for family %s must not define upstream_mode", mode.Mode, family.Model)
			}

			effectiveFloor := family.PoolFloor
			if mode.PoolFloorOverride != nil && *mode.PoolFloorOverride != "" {
				effectiveFloor = *mode.PoolFloorOverride
			}

			var upstreamModel, upstreamMode string
			if requiresUpstreamModel(family.Type) {
				upstreamModel = family.UpstreamModel
			}
			if requiresUpstreamMode(family.Type) {
				upstreamMode = mode.UpstreamMode
			}

			rm := &ResolvedModel{
				RequestName:    requestName,
				Family:         family,
				Mode:           mode,
				EffectiveFloor: effectiveFloor,
				UpstreamModel:  upstreamModel,
				UpstreamMode:   upstreamMode,
				ForceThinking:  mode.ForceThinking,
				EnablePro:      mode.EnablePro,
			}

			newByName[requestName] = rm
			newByType[family.Type] = append(newByType[family.Type], rm)
		}
		if len(modes) > 0 && !defaultFound {
			return nil, fmt.Errorf("family %s default_mode_id does not reference an enabled mode", family.Model)
		}
	}

	return &ModelSnapshot{
		byRequestName: newByName,
		enabledByType: newByType,
	}, nil
}

func (r *ModelRegistry) applySnapshotLocked(snapshot *ModelSnapshot) {
	r.byRequestName = snapshot.byRequestName
	r.enabledByType = snapshot.enabledByType
}
