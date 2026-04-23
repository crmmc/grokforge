package config

import (
	"strconv"
	"sync"
	"testing"
)

func TestRuntimeCloneIsolation(t *testing.T) {
	enabled := true
	cfg := &Config{
		App: AppConfig{
			FilterTags: []string{"xaiartifact"},
		},
		Retry: RetryConfig{
			ResetSessionStatusCodes: []int{401},
		},
		Image: ImageConfig{
			BlockedParallelEnabled: &enabled,
		},
	}

	runtime := NewRuntime(cfg)
	snapshot := runtime.Snapshot()
	snapshot.App.FilterTags[0] = "changed"
	snapshot.Retry.ResetSessionStatusCodes[0] = 500
	*snapshot.Image.BlockedParallelEnabled = false

	current := runtime.Get()
	if current.App.FilterTags[0] != "xaiartifact" {
		t.Fatalf("runtime config mutated filter tags: %v", current.App.FilterTags)
	}
	if current.Retry.ResetSessionStatusCodes[0] != 401 {
		t.Fatalf("runtime config mutated retry config: %v", current.Retry.ResetSessionStatusCodes)
	}
	if !*current.Image.BlockedParallelEnabled {
		t.Fatal("runtime config mutated image toggle")
	}
}

func TestApplyDBOverrides_ResolvesImageToggle(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.ApplyDBOverrides(map[string]string{
		"image.blocked_parallel_enabled": "false",
	}); err != nil {
		t.Fatalf("ApplyDBOverrides() error = %v", err)
	}

	if EffectiveBlockedParallelEnabled(&cfg.Image) {
		t.Fatal("expected blocked_parallel_enabled override to disable parallel recovery")
	}
}

func TestEffectiveBlockedParallelEnabled_DefaultsTrue(t *testing.T) {
	if !EffectiveBlockedParallelEnabled(nil) {
		t.Fatal("nil image config should default to true")
	}
	if !EffectiveBlockedParallelEnabled(&ImageConfig{}) {
		t.Fatal("nil pointer field should default to true")
	}
}

func TestRuntimeUpdateSerializesConcurrentWriters(t *testing.T) {
	runtime := NewRuntime(&Config{})

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := runtime.Update(func(cfg *Config) error {
				cfg.App.CustomInstruction += strconv.Itoa(i) + ","
				cfg.Token.FailThreshold++
				return nil
			})
			if err != nil {
				t.Errorf("Update() error = %v", err)
			}
		}()
	}
	wg.Wait()

	current := runtime.Get()
	if current.Token.FailThreshold != 32 {
		t.Fatalf("expected all concurrent updates to be preserved, got fail_threshold=%d", current.Token.FailThreshold)
	}
}
