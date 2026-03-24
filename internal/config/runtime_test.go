package config

import "testing"

func TestRuntimeCloneIsolation(t *testing.T) {
	enabled := true
	cfg := &Config{
		App: AppConfig{
			FilterTags: []string{"xaiartifact"},
		},
		Retry: RetryConfig{
			ResetSessionStatusCodes: []int{401},
			CoolingStatusCodes:      []int{429},
		},
		Token: TokenConfig{
			BasicModels: []string{"grok-2"},
			SuperModels: []string{"grok-3"},
		},
		Image: ImageConfig{
			BlockedParallelEnabled: &enabled,
		},
	}

	runtime := NewRuntime(cfg)
	snapshot := runtime.Snapshot()
	snapshot.App.FilterTags[0] = "changed"
	snapshot.Retry.ResetSessionStatusCodes[0] = 500
	snapshot.Token.BasicModels[0] = "changed-model"
	*snapshot.Image.BlockedParallelEnabled = false

	current := runtime.Get()
	if current.App.FilterTags[0] != "xaiartifact" {
		t.Fatalf("runtime config mutated filter tags: %v", current.App.FilterTags)
	}
	if current.Retry.ResetSessionStatusCodes[0] != 401 {
		t.Fatalf("runtime config mutated retry config: %v", current.Retry.ResetSessionStatusCodes)
	}
	if current.Token.BasicModels[0] != "grok-2" {
		t.Fatalf("runtime config mutated token config: %v", current.Token.BasicModels)
	}
	if !*current.Image.BlockedParallelEnabled {
		t.Fatal("runtime config mutated image toggle")
	}
}

func TestApplyDBOverrides_TrimsModelsAndResolvesImageToggle(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ApplyDBOverrides(map[string]string{
		"token.basic_models":             " grok-2 , grok-vision-2 , ",
		"token.super_models":             " grok-3 ",
		"image.blocked_parallel_enabled": "false",
	})

	if len(cfg.Token.BasicModels) != 2 || cfg.Token.BasicModels[0] != "grok-2" || cfg.Token.BasicModels[1] != "grok-vision-2" {
		t.Fatalf("basic models not trimmed correctly: %#v", cfg.Token.BasicModels)
	}
	if len(cfg.Token.SuperModels) != 1 || cfg.Token.SuperModels[0] != "grok-3" {
		t.Fatalf("super models not trimmed correctly: %#v", cfg.Token.SuperModels)
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
