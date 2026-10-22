package config

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClone_NilConfigReturnsDefaults(t *testing.T) {
	cloned := Clone(nil)
	require.NotNil(t, cloned)
	assert.Equal(t, DefaultConfig(), cloned)
}

func TestClone_DeepCopiesSlicesAndPointers(t *testing.T) {
	enabled := true
	original := &Config{
		App: AppConfig{FilterTags: []string{"a", "b"}},
		Retry: RetryConfig{
			ResetSessionStatusCodes: []int{403},
		},
		Image: ImageConfig{BlockedParallelEnabled: &enabled},
	}

	cloned := Clone(original)
	require.NotNil(t, cloned)
	cloned.App.FilterTags[0] = "mutated"
	cloned.Retry.ResetSessionStatusCodes[0] = 500
	*cloned.Image.BlockedParallelEnabled = false

	assert.Equal(t, []string{"a", "b"}, original.App.FilterTags)
	assert.Equal(t, []int{403}, original.Retry.ResetSessionStatusCodes)
	assert.True(t, *original.Image.BlockedParallelEnabled)
}

func TestClone_NilSliceFieldsStayEmpty(t *testing.T) {
	cloned := Clone(&Config{})
	assert.Empty(t, cloned.App.FilterTags)
	assert.Empty(t, cloned.Retry.ResetSessionStatusCodes)
	assert.Nil(t, cloned.Image.BlockedParallelEnabled)
}

func TestRuntime_NilReceiverSafety(t *testing.T) {
	var r *Runtime
	assert.Nil(t, r.Get())
	assert.Equal(t, DefaultConfig(), r.Snapshot())
	assert.NotPanics(t, func() { r.Store(&Config{}) })
	assert.NoError(t, r.Update(func(*Config) error { return errors.New("must not run") }))
}

func TestRuntime_StoreReplacesSnapshot(t *testing.T) {
	r := NewRuntime(&Config{App: AppConfig{AppKey: "first"}})

	next := &Config{App: AppConfig{AppKey: "second"}}
	r.Store(next)
	// Mutating the source after Store must not affect the stored clone.
	next.App.AppKey = "mutated"

	current := r.Get()
	require.NotNil(t, current)
	assert.Equal(t, "second", current.App.AppKey)
}

func TestRuntime_StoreNilReceiverIsNoOp(t *testing.T) {
	var r *Runtime
	assert.NotPanics(t, func() { r.Store(DefaultConfig()) })
}

func TestRuntime_UpdatePropagatesErrorAndKeepsCurrent(t *testing.T) {
	r := NewRuntime(&Config{App: AppConfig{AppKey: "keep"}})

	wantErr := errors.New("validation failed")
	err := r.Update(func(cfg *Config) error {
		cfg.App.AppKey = "rejected"
		return wantErr
	})
	require.ErrorIs(t, err, wantErr)
	assert.Equal(t, "keep", r.Get().App.AppKey)
}

func TestRuntime_UpdateSnapshotIsDeepCopy(t *testing.T) {
	r := NewRuntime(&Config{
		App: AppConfig{FilterTags: []string{"a"}},
	})

	err := r.Update(func(cfg *Config) error {
		cfg.App.FilterTags = append(cfg.App.FilterTags, "b")
		return nil
	})
	require.NoError(t, err)

	current := r.Get()
	assert.Equal(t, []string{"a", "b"}, current.App.FilterTags)

	snapshot := r.Snapshot()
	snapshot.App.FilterTags[0] = "mutated"
	assert.Equal(t, "a", r.Get().App.FilterTags[0], "Snapshot must be isolated from current")
}

func TestRuntime_ConcurrentStoreAndGet(t *testing.T) {
	r := NewRuntime(DefaultConfig())

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			r.Store(&Config{App: AppConfig{AppKey: "writer"}})
		}()
		go func() {
			defer wg.Done()
			if cfg := r.Get(); cfg != nil {
				_ = cfg.App.AppKey
			}
		}()
	}
	wg.Wait()
}
