// Package registry provides an in-memory snapshot of enabled models
// for O(1) request name resolution on the hot path.
package registry

import (
	"sort"
	"sync"

	"github.com/crmmc/grokforge/internal/modelconfig"
)

// ResolvedModel holds the resolved model info for a model ID.
type ResolvedModel struct {
	ID            string
	DisplayName   string
	Type          string // internal runtime type
	PublicType    string // derived public type
	Enabled       bool
	PoolFloor     string // effective pool floor
	QuotaMode     string
	UpstreamModel string
	UpstreamMode  string
	ForceThinking bool
	EnablePro     bool
}

// ModelRegistry maintains an in-memory snapshot of enabled models.
// The registry is read-only after construction; the mutex is retained
// to preserve the thread-safe API contract.
type ModelRegistry struct {
	mu            sync.RWMutex
	byID          map[string]*ResolvedModel
	enabledByType map[string][]*ResolvedModel
}

// NewModelRegistry creates a read-only ModelRegistry from static catalog specs.
func NewModelRegistry(specs []modelconfig.ModelSpec) *ModelRegistry {
	r := &ModelRegistry{
		byID:          make(map[string]*ResolvedModel, len(specs)),
		enabledByType: make(map[string][]*ResolvedModel),
	}
	for _, s := range specs {
		if !s.Enabled {
			continue
		}
		rm := &ResolvedModel{
			ID:            s.ID,
			DisplayName:   s.DisplayName,
			Type:          s.Type,
			PublicType:    s.PublicType,
			Enabled:       s.Enabled,
			PoolFloor:     s.PoolFloor,
			QuotaMode:     s.QuotaMode,
			UpstreamModel: s.UpstreamModel,
			UpstreamMode:  s.UpstreamMode,
			ForceThinking: s.ForceThinking,
			EnablePro:     s.EnablePro,
		}
		r.byID[s.ID] = rm
		r.enabledByType[s.Type] = append(r.enabledByType[s.Type], rm)
	}
	return r
}

// NewTestRegistry creates a ModelRegistry from ModelSpec slice for testing.
// This is an alias for NewModelRegistry for semantic clarity in test code.
func NewTestRegistry(specs []modelconfig.ModelSpec) *ModelRegistry {
	return NewModelRegistry(specs)
}

// Resolve looks up a model ID and returns the resolved model.
func (r *ModelRegistry) Resolve(id string) (*ResolvedModel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rm, ok := r.byID[id]
	return rm, ok
}

// AllRequestNames returns all registered model IDs (sorted).
// Retained for downstream compatibility.
func (r *ModelRegistry) AllRequestNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.byID))
	for id := range r.byID {
		names = append(names, id)
	}
	sort.Strings(names)
	return names
}

// ResolvePoolFloor implements token.ModelResolver.
// Returns the effective pool floor for a model ID.
func (r *ModelRegistry) ResolvePoolFloor(requestName string) (floor string, ok bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	rm, found := r.byID[requestName]
	if !found {
		return "", false
	}
	return rm.PoolFloor, true
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
	out := make([]*ResolvedModel, 0, len(r.byID))
	for _, rm := range r.byID {
		out = append(out, rm)
	}
	return out
}

// Count returns the number of enabled models in the registry.
func (r *ModelRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byID)
}
