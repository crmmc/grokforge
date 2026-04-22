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
	ID              string
	DisplayName     string
	Type            string // internal runtime type
	PublicType      string // derived public type
	Enabled         bool
	PoolFloor       string // effective pool floor
	Mode            string
	QuotaSync       bool // whether this model participates in quota tracking
	CooldownSeconds int  // cooldown duration; only used when QuotaSync is false
	UpstreamModel   string
	UpstreamMode    string
	ForceThinking   bool
	EnablePro       bool
}

// ModelRegistry maintains an in-memory snapshot of enabled models.
// The registry is read-only after construction; the mutex is retained
// to preserve the thread-safe API contract.
type ModelRegistry struct {
	mu            sync.RWMutex
	byID          map[string]*ResolvedModel
	enabledByType map[string][]*ResolvedModel
	modeByID      map[string]modelconfig.ModeSpec
	modeOrder     []modelconfig.ModeSpec // preserves original order
}

// NewModelRegistry creates a read-only ModelRegistry from static catalog specs.
func NewModelRegistry(specs []modelconfig.ModelSpec, modes []modelconfig.ModeSpec) *ModelRegistry {
	modeByID := make(map[string]modelconfig.ModeSpec, len(modes))
	for _, m := range modes {
		modeByID[m.ID] = m
	}

	r := &ModelRegistry{
		byID:          make(map[string]*ResolvedModel, len(specs)),
		enabledByType: make(map[string][]*ResolvedModel),
		modeByID:      modeByID,
		modeOrder:     append([]modelconfig.ModeSpec(nil), modes...),
	}
	for _, s := range specs {
		if !s.Enabled {
			continue
		}
		rm := &ResolvedModel{
			ID:              s.ID,
			DisplayName:     s.DisplayName,
			Type:            s.Type,
			PublicType:      s.PublicType,
			Enabled:         s.Enabled,
			PoolFloor:       s.PoolFloor,
			Mode:            s.Mode,
			QuotaSync:       s.IsQuotaTracked(),
			CooldownSeconds: s.CooldownSeconds,
			UpstreamModel:   s.UpstreamModel,
			UpstreamMode:    s.UpstreamMode,
			ForceThinking:   s.ForceThinking,
			EnablePro:       s.EnablePro,
		}
		r.byID[s.ID] = rm
		r.enabledByType[s.Type] = append(r.enabledByType[s.Type], rm)
	}
	return r
}

// NewTestRegistry creates a ModelRegistry from ModelSpec and ModeSpec slices for testing.
// This is an alias for NewModelRegistry for semantic clarity in test code.
func NewTestRegistry(specs []modelconfig.ModelSpec, modes []modelconfig.ModeSpec) *ModelRegistry {
	return NewModelRegistry(specs, modes)
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

// GetMode looks up a mode by ID.
func (r *ModelRegistry) GetMode(id string) (modelconfig.ModeSpec, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	m, ok := r.modeByID[id]
	return m, ok
}

// AllModes returns all modes in their original catalog order.
func (r *ModelRegistry) AllModes() []modelconfig.ModeSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]modelconfig.ModeSpec, len(r.modeOrder))
	copy(out, r.modeOrder)
	return out
}

// SupportedModes returns modes whose default_quota for the given pool is > 0,
// preserving the original catalog order.
func (r *ModelRegistry) SupportedModes(pool string) []modelconfig.ModeSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []modelconfig.ModeSpec
	for _, m := range r.modeOrder {
		if m.DefaultQuota[pool] > 0 {
			out = append(out, m)
		}
	}
	return out
}

// Count returns the number of enabled models in the registry.
func (r *ModelRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byID)
}
