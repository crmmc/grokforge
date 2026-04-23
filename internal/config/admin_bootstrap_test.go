package config

import (
	"bytes"
	"strings"
	"testing"
)

func TestEnsureAdminAppKey_GeneratesBootstrapKeyWhenMissing(t *testing.T) {
	cfg := &Config{}

	key, generated, err := EnsureAdminAppKey(cfg, bytes.NewReader(bytes.Repeat([]byte{0x42}, bootstrapEntropyBytes)))
	if err != nil {
		t.Fatalf("EnsureAdminAppKey() error = %v", err)
	}
	if !generated {
		t.Fatal("EnsureAdminAppKey() should generate bootstrap key")
	}
	if cfg.App.AppKey != key {
		t.Fatalf("cfg.App.AppKey = %q, want %q", cfg.App.AppKey, key)
	}
	if !strings.HasPrefix(key, bootstrapAppKeyPrefix) {
		t.Fatalf("bootstrap key %q missing prefix %q", key, bootstrapAppKeyPrefix)
	}
	if len(key) <= len(bootstrapAppKeyPrefix) {
		t.Fatalf("bootstrap key %q too short", key)
	}
}

func TestEnsureAdminAppKey_NoOpWhenConfigured(t *testing.T) {
	cfg := &Config{
		App: AppConfig{
			AppKey: "configured-key",
		},
	}

	key, generated, err := EnsureAdminAppKey(cfg, bytes.NewReader(bytes.Repeat([]byte{0x42}, bootstrapEntropyBytes)))
	if err != nil {
		t.Fatalf("EnsureAdminAppKey() error = %v", err)
	}
	if generated {
		t.Fatal("EnsureAdminAppKey() should not generate when app_key already configured")
	}
	if key != "" {
		t.Fatalf("bootstrap key = %q, want empty", key)
	}
	if cfg.App.AppKey != "configured-key" {
		t.Fatalf("cfg.App.AppKey = %q, want configured-key", cfg.App.AppKey)
	}
}

func TestEnsureAdminAppKey_ReturnsErrorWhenEntropyShort(t *testing.T) {
	cfg := &Config{}

	_, generated, err := EnsureAdminAppKey(cfg, bytes.NewReader([]byte("short")))
	if err == nil {
		t.Fatal("EnsureAdminAppKey() error = nil, want error")
	}
	if generated {
		t.Fatal("EnsureAdminAppKey() should not report generated on error")
	}
	if cfg.App.AppKey != "" {
		t.Fatalf("cfg.App.AppKey = %q, want empty", cfg.App.AppKey)
	}
}

func TestEnsureAdminAppKey_GeneratesDifferentKeyAcrossRuns(t *testing.T) {
	cfgA := &Config{}
	cfgB := &Config{}

	keyA, generatedA, err := EnsureAdminAppKey(cfgA, bytes.NewReader(bytes.Repeat([]byte{0x11}, bootstrapEntropyBytes)))
	if err != nil {
		t.Fatalf("EnsureAdminAppKey(cfgA) error = %v", err)
	}
	keyB, generatedB, err := EnsureAdminAppKey(cfgB, bytes.NewReader(bytes.Repeat([]byte{0x22}, bootstrapEntropyBytes)))
	if err != nil {
		t.Fatalf("EnsureAdminAppKey(cfgB) error = %v", err)
	}
	if !generatedA || !generatedB {
		t.Fatal("EnsureAdminAppKey() should generate for both empty configs")
	}
	if keyA == keyB {
		t.Fatalf("bootstrap keys should differ across runs, both = %q", keyA)
	}
}
