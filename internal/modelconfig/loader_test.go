package modelconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// validModeTOML is a minimal valid [[mode]] block.
const validModeTOML = `
[[mode]]
id = "auto"
upstream_name = "auto"
window_seconds = 72000
[mode.default_quota]
basic = 20
super = 50
heavy = 150
`

// validModelTOML is a minimal valid [[model]] block referencing mode "auto".
const validModelTOML = `
[[model]]
id = "test-chat"
display_name = "Test Chat"
type = "chat"
enabled = true
pool_floor = "basic"
mode = "auto"
upstream_mode = "auto"
`

// minimalCatalog returns a valid catalog with version=1, one mode, one model.
func minimalCatalog() string {
	return "version = 1\n" + validModeTOML + validModelTOML
}

// makeFS builds an in-memory FS with the given TOML as models.toml.
func makeFS(toml string) fstest.MapFS {
	return fstest.MapFS{
		"models.toml": &fstest.MapFile{Data: []byte(toml)},
	}
}

// mustContain asserts err is non-nil and contains substr.
func mustContain(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("error = %q, want substring %q", err, substr)
	}
}

// --- Success path ---

func TestLoad_EmbeddedCatalog(t *testing.T) {
	models, modes, err := Load(EmbeddedFS, "")
	if err != nil {
		t.Fatalf("Load embedded: %v", err)
	}

	if got := len(models); got != 20 {
		t.Fatalf("expected 20 models, got %d", got)
	}
	if got := len(modes); got != 5 {
		t.Fatalf("expected 5 modes, got %d", got)
	}

	first := models[0]
	if first.ID != "grok-4.20-0309-non-reasoning" {
		t.Errorf("first model id = %q, want %q", first.ID, "grok-4.20-0309-non-reasoning")
	}
	if first.Mode != "fast" {
		t.Errorf("first model mode = %q, want %q", first.Mode, "fast")
	}

	m0 := modes[0]
	if m0.ID != "auto" {
		t.Errorf("first mode id = %q, want %q", m0.ID, "auto")
	}
	if m0.UpstreamName != "auto" {
		t.Errorf("first mode upstream_name = %q, want %q", m0.UpstreamName, "auto")
	}
}

func TestLoad_ExternalFile(t *testing.T) {
	content := minimalCatalog()
	dir := t.TempDir()
	p := filepath.Join(dir, "models.toml")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	models, modes, err := Load(EmbeddedFS, p)
	if err != nil {
		t.Fatalf("Load external: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if len(modes) != 1 {
		t.Fatalf("expected 1 mode, got %d", len(modes))
	}
	if models[0].ID != "test-chat" {
		t.Errorf("model id = %q, want %q", models[0].ID, "test-chat")
	}
}

func TestDerivePublicType_ImageLite(t *testing.T) {
	got := DerivePublicType(TypeImageLite)
	if got != "image" {
		t.Errorf("DerivePublicType(%q) = %q, want %q", TypeImageLite, got, "image")
	}
}

// --- Mode validation failures ---

func TestValidate_NoMode(t *testing.T) {
	content := "version = 1\n" + validModelTOML
	fs := makeFS(content)
	_, _, err := Load(fs, "")
	mustContain(t, err, "at least one mode")
}

func TestValidate_DuplicateModeID(t *testing.T) {
	content := "version = 1\n" + validModeTOML + `
[[mode]]
id = "auto"
upstream_name = "other"
window_seconds = 3600
[mode.default_quota]
basic = 10
super = 20
heavy = 30
` + validModelTOML
	fs := makeFS(content)
	_, _, err := Load(fs, "")
	mustContain(t, err, "duplicate id")
}

func TestValidate_DuplicateUpstreamName(t *testing.T) {
	content := "version = 1\n" + validModeTOML + `
[[mode]]
id = "other"
upstream_name = "auto"
window_seconds = 3600
[mode.default_quota]
basic = 10
super = 20
heavy = 30
` + validModelTOML
	fs := makeFS(content)
	_, _, err := Load(fs, "")
	mustContain(t, err, "duplicate upstream_name")
}

func TestValidate_EmptyUpstreamName(t *testing.T) {
	content := `version = 1

[[mode]]
id = "auto"
upstream_name = ""
window_seconds = 72000
[mode.default_quota]
basic = 20
super = 50
heavy = 150
` + validModelTOML
	fs := makeFS(content)
	_, _, err := Load(fs, "")
	mustContain(t, err, "upstream_name is required")
}

func TestValidate_WindowSecondsZero(t *testing.T) {
	content := `version = 1

[[mode]]
id = "auto"
upstream_name = "auto"
window_seconds = 0
[mode.default_quota]
basic = 20
super = 50
heavy = 150
` + validModelTOML
	fs := makeFS(content)
	_, _, err := Load(fs, "")
	mustContain(t, err, "window_seconds must be > 0")
}

func TestValidate_WindowSecondsNegative(t *testing.T) {
	content := `version = 1

[[mode]]
id = "auto"
upstream_name = "auto"
window_seconds = -100
[mode.default_quota]
basic = 20
super = 50
heavy = 150
` + validModelTOML
	fs := makeFS(content)
	_, _, err := Load(fs, "")
	mustContain(t, err, "window_seconds must be > 0")
}

func TestValidate_DefaultQuotaMissingPool(t *testing.T) {
	content := `version = 1

[[mode]]
id = "auto"
upstream_name = "auto"
window_seconds = 72000
[mode.default_quota]
basic = 20
super = 50
` + validModelTOML
	fs := makeFS(content)
	_, _, err := Load(fs, "")
	mustContain(t, err, `default_quota missing pool "heavy"`)
}

// --- Model validation failures (quota tracking) ---

func TestValidate_QuotaTrackedMissingMode(t *testing.T) {
	// quota-tracked (default) model without mode field
	content := "version = 1\n" + validModeTOML + `
[[model]]
id = "test-chat"
display_name = "Test Chat"
type = "chat"
enabled = true
pool_floor = "basic"
upstream_mode = "auto"
`
	fs := makeFS(content)
	_, _, err := Load(fs, "")
	mustContain(t, err, "mode is required for quota-tracked")
}

func TestValidate_QuotaTrackedUnknownMode(t *testing.T) {
	content := "version = 1\n" + validModeTOML + `
[[model]]
id = "test-chat"
display_name = "Test Chat"
type = "chat"
enabled = true
pool_floor = "basic"
mode = "nonexistent"
upstream_mode = "auto"
`
	fs := makeFS(content)
	_, _, err := Load(fs, "")
	mustContain(t, err, "does not match any defined mode")
}

func TestValidate_QuotaTrackedWithCooldown(t *testing.T) {
	content := "version = 1\n" + validModeTOML + `
[[model]]
id = "test-chat"
display_name = "Test Chat"
type = "chat"
enabled = true
pool_floor = "basic"
mode = "auto"
upstream_mode = "auto"
cooldown_seconds = 300
`
	fs := makeFS(content)
	_, _, err := Load(fs, "")
	mustContain(t, err, "cooldown_seconds is forbidden for quota-tracked")
}

func TestValidate_NonTrackedWithMode(t *testing.T) {
	content := "version = 1\n" + validModeTOML + `
[[model]]
id = "test-ws"
display_name = "Test WS"
type = "image_ws"
enabled = true
pool_floor = "super"
quota_sync = false
cooldown_seconds = 300
mode = "auto"
`
	fs := makeFS(content)
	_, _, err := Load(fs, "")
	mustContain(t, err, "mode is forbidden when quota_sync = false")
}

func TestValidate_NonTrackedMissingCooldown(t *testing.T) {
	content := "version = 1\n" + validModeTOML + `
[[model]]
id = "test-ws"
display_name = "Test WS"
type = "image_ws"
enabled = true
pool_floor = "super"
quota_sync = false
`
	fs := makeFS(content)
	_, _, err := Load(fs, "")
	mustContain(t, err, "cooldown_seconds must be > 0 when quota_sync = false")
}

func TestValidate_QuotaSyncFalseOnlyAllowedForImageWS(t *testing.T) {
	content := "version = 1\n" + validModeTOML + `
[[model]]
id = "test-chat"
display_name = "Test Chat"
type = "chat"
enabled = true
pool_floor = "basic"
quota_sync = false
cooldown_seconds = 300
upstream_mode = "auto"
`
	fs := makeFS(content)
	_, _, err := Load(fs, "")
	mustContain(t, err, `quota_sync = false is only valid for type "image_ws"`)
}

// --- Model validation failures (general) ---

func TestValidate_InvalidTOML(t *testing.T) {
	fs := makeFS("this is not valid [[[ toml")
	_, _, err := Load(fs, "")
	mustContain(t, err, "parse TOML")
}

func TestValidate_DuplicateModelID(t *testing.T) {
	content := minimalCatalog() + `
[[model]]
id = "test-chat"
display_name = "Dup"
type = "chat"
enabled = false
pool_floor = "basic"
mode = "auto"
upstream_mode = "auto"
`
	fs := makeFS(content)
	_, _, err := Load(fs, "")
	mustContain(t, err, "duplicate id")
}

func TestValidate_InvalidType(t *testing.T) {
	content := "version = 1\n" + validModeTOML + `
[[model]]
id = "test-bad"
display_name = "Bad"
type = "invalid"
enabled = true
pool_floor = "basic"
mode = "auto"
upstream_mode = "auto"
`
	fs := makeFS(content)
	_, _, err := Load(fs, "")
	mustContain(t, err, "invalid type")
}

func TestValidate_UnknownField(t *testing.T) {
	content := minimalCatalog() + "\nunknown_field = \"x\"\n"
	fs := makeFS(content)
	_, _, err := Load(fs, "")
	mustContain(t, err, "unknown field")
}

func TestValidate_NoEnabledModel(t *testing.T) {
	content := "version = 1\n" + validModeTOML + `
[[model]]
id = "test-chat"
display_name = "Test Chat"
type = "chat"
enabled = false
pool_floor = "basic"
mode = "auto"
upstream_mode = "auto"
`
	fs := makeFS(content)
	_, _, err := Load(fs, "")
	mustContain(t, err, "at least one model must be enabled")
}

func TestLoad_ExternalFileNotFound(t *testing.T) {
	_, _, err := Load(EmbeddedFS, "/nonexistent/path/models.toml")
	mustContain(t, err, "read external file")
}
