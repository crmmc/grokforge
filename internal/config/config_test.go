package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad_RejectsUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := "[token]\nunknown_key = 1\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "unknown config keys") {
		t.Fatalf("expected unknown config keys error, got %v", err)
	}
}

func TestLoad_RejectsNegativeCacheImageLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := "[cache]\nimage_max_mb = -1\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "cache.image_max_mb") {
		t.Fatalf("expected cache.image_max_mb error, got %v", err)
	}
}

func TestLoad_RejectsNegativeCacheVideoLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := "[cache]\nvideo_max_mb = -1\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "cache.video_max_mb") {
		t.Fatalf("expected cache.video_max_mb error, got %v", err)
	}
}

func TestLoad_AllowsZeroCacheLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := "[cache]\nimage_max_mb = 0\nvideo_max_mb = 0\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Cache.ImageMaxMB != 0 || cfg.Cache.VideoMaxMB != 0 {
		t.Fatalf("expected zero cache limits, got %+v", cfg.Cache)
	}
}

func TestApplyDBOverrides_RejectsUnknownKey(t *testing.T) {
	cfg := DefaultConfig()

	err := cfg.ApplyDBOverrides(map[string]string{
		"token.preferred_pool": "super",
	})
	if err == nil || !strings.Contains(err.Error(), "unknown db override key") {
		t.Fatalf("expected unknown db override key error, got %v", err)
	}
}

func TestApplyDBOverrides_RejectsInvalidValue(t *testing.T) {
	cfg := DefaultConfig()

	err := cfg.ApplyDBOverrides(map[string]string{
		"retry.max_tokens": "abc",
	})
	if err == nil || !strings.Contains(err.Error(), "invalid integer") {
		t.Fatalf("expected invalid integer error, got %v", err)
	}
}

func TestApplyDBOverrides_RequiresProxyBrowserPair(t *testing.T) {
	cfg := DefaultConfig()

	err := cfg.ApplyDBOverrides(map[string]string{
		"proxy.user_agent": "Mozilla/5.0",
	})
	if err == nil || !strings.Contains(err.Error(), "must be overridden together") {
		t.Fatalf("expected paired proxy override error, got %v", err)
	}
}
