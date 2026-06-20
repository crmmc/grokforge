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

func TestLoad_IgnoresDeprecatedCoolDurationKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[token]
cool_duration_basic_sec = 86400
cool_duration_super_sec = 7200
cool_duration_heavy_sec = 7200
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(path); err != nil {
		t.Fatalf("deprecated config keys should be ignored, got %v", err)
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

func TestLoad_RejectsInvalidImageFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := "[image]\nformat = \"url\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "image.format") {
		t.Fatalf("expected image.format error, got %v", err)
	}
}

func TestLoad_AllowsLocalURLImageFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := "[image]\nformat = \"local_url\"\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Image.Format != ImageFormatLocalURL {
		t.Fatalf("expected local_url, got %q", cfg.Image.Format)
	}
}

func TestLoad_RejectsNegativeRecentUsePenalty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := "[token]\nrecent_use_penalty_sec = -1\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "token.recent_use_penalty_sec") {
		t.Fatalf("expected token.recent_use_penalty_sec error, got %v", err)
	}
}

func TestApplyDBOverrides_ImageFormat(t *testing.T) {
	cfg := DefaultConfig()

	err := cfg.ApplyDBOverrides(map[string]string{
		"image.format": " local_url ",
	})
	if err != nil {
		t.Fatalf("apply overrides: %v", err)
	}
	if cfg.Image.Format != ImageFormatLocalURL {
		t.Fatalf("expected local_url, got %q", cfg.Image.Format)
	}
}

func TestApplyDBOverrides_ConsoleConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Console.Enabled {
		t.Fatal("console.enabled default should be false")
	}
	if cfg.Console.WebSearch {
		t.Fatal("console.web_search default should be false")
	}

	err := cfg.ApplyDBOverrides(map[string]string{
		"console.enabled":    "true",
		"console.web_search": "true",
	})
	if err != nil {
		t.Fatalf("ApplyDBOverrides: %v", err)
	}
	if !cfg.Console.Enabled {
		t.Fatal("console.enabled override was not applied")
	}
	if !cfg.Console.WebSearch {
		t.Fatal("console.web_search override was not applied")
	}
}

func TestApplyDBOverrides_RejectsInvalidImageFormat(t *testing.T) {
	cfg := DefaultConfig()

	err := cfg.ApplyDBOverrides(map[string]string{
		"image.format": "url",
	})
	if err == nil || !strings.Contains(err.Error(), "image.format") {
		t.Fatalf("expected image.format error, got %v", err)
	}
}

func TestApplyDBOverrides_RejectsNegativeRecentUsePenalty(t *testing.T) {
	cfg := DefaultConfig()

	err := cfg.ApplyDBOverrides(map[string]string{
		"token.recent_use_penalty_sec": "-5",
	})
	if err == nil || !strings.Contains(err.Error(), "token.recent_use_penalty_sec") {
		t.Fatalf("expected token.recent_use_penalty_sec error, got %v", err)
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

func TestApplyDBOverrides_IgnoresDeprecatedCoolDurationKeys(t *testing.T) {
	cfg := DefaultConfig()

	err := cfg.ApplyDBOverrides(map[string]string{
		"token.cool_duration_basic_sec": "86400",
		"token.cool_duration_super_sec": "7200",
		"token.cool_duration_heavy_sec": "7200",
	})
	if err != nil {
		t.Fatalf("deprecated db override keys should be ignored, got %v", err)
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

func TestApplyDBOverrides_AllowsProxyBrowserAndUserAgentIndependently(t *testing.T) {
	cfg := DefaultConfig()

	err := cfg.ApplyDBOverrides(map[string]string{
		"proxy.user_agent": "Mozilla/5.0",
	})
	if err != nil {
		t.Fatalf("proxy.user_agent should be allowed without proxy.browser: %v", err)
	}
	if cfg.Proxy.UserAgent != "Mozilla/5.0" {
		t.Fatalf("proxy.user_agent override was not applied, got %q", cfg.Proxy.UserAgent)
	}

	err = cfg.ApplyDBOverrides(map[string]string{
		"proxy.browser": "firefox135",
	})
	if err != nil {
		t.Fatalf("proxy.browser should be allowed without proxy.user_agent: %v", err)
	}
	if cfg.Proxy.Browser != "firefox135" {
		t.Fatalf("proxy.browser override was not applied, got %q", cfg.Proxy.Browser)
	}
}
