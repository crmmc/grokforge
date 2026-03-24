package config

import (
	"sync"
	"sync/atomic"
)

// Runtime provides atomic access to the live configuration snapshot.
type Runtime struct {
	current atomic.Pointer[Config]
	mu      sync.Mutex
}

// NewRuntime creates a runtime config container initialized with a deep copy.
func NewRuntime(cfg *Config) *Runtime {
	r := &Runtime{}
	r.current.Store(Clone(cfg))
	return r
}

// Get returns the current immutable configuration snapshot.
func (r *Runtime) Get() *Config {
	if r == nil {
		return nil
	}
	return r.current.Load()
}

// Snapshot returns a deep copy of the current configuration.
func (r *Runtime) Snapshot() *Config {
	return Clone(r.Get())
}

// Store replaces the current config with a cloned copy.
func (r *Runtime) Store(cfg *Config) {
	if r == nil {
		return
	}
	r.current.Store(Clone(cfg))
}

// Update clones the current config, applies fn, and atomically swaps it in.
func (r *Runtime) Update(fn func(*Config) error) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	next := Clone(r.Get())
	if err := fn(next); err != nil {
		return err
	}
	r.current.Store(next)
	return nil
}

// Clone deep-copies a configuration tree so callers can mutate safely.
func Clone(cfg *Config) *Config {
	if cfg == nil {
		return DefaultConfig()
	}

	cloned := *cfg
	cloned.App.FilterTags = append([]string(nil), cfg.App.FilterTags...)
	cloned.Retry.ResetSessionStatusCodes = append([]int(nil), cfg.Retry.ResetSessionStatusCodes...)
	cloned.Retry.CoolingStatusCodes = append([]int(nil), cfg.Retry.CoolingStatusCodes...)
	cloned.Token.BasicModels = append([]string(nil), cfg.Token.BasicModels...)
	cloned.Token.SuperModels = append([]string(nil), cfg.Token.SuperModels...)

	if cfg.Image.BlockedParallelEnabled != nil {
		v := *cfg.Image.BlockedParallelEnabled
		cloned.Image.BlockedParallelEnabled = &v
	}

	return &cloned
}

// EffectiveBlockedParallelEnabled resolves the image toggle with the runtime default.
func EffectiveBlockedParallelEnabled(cfg *ImageConfig) bool {
	if cfg == nil || cfg.BlockedParallelEnabled == nil {
		return true
	}
	return *cfg.BlockedParallelEnabled
}
