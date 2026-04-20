package modelconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// minimalCatalogTOML returns a valid single-model TOML catalog.
// Each override replaces the corresponding line by key prefix match.
func minimalCatalogTOML(overrides ...string) string {
	lines := []string{
		`version = 1`,
		``,
		`[[model]]`,
		`id = "test-chat"`,
		`display_name = "Test Chat"`,
		`type = "chat"`,
		`enabled = true`,
		`pool_floor = "basic"`,
		`quota_mode = "auto"`,
		`upstream_mode = "auto"`,
	}
	for _, ov := range overrides {
		key := strings.SplitN(ov, " ", 2)[0] // e.g. "type"
		replaced := false
		for i, l := range lines {
			if strings.HasPrefix(l, key+" ") {
				lines[i] = ov
				replaced = true
				break
			}
		}
		if !replaced {
			lines = append(lines, ov)
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// writeTempTOML writes content to a temp file and returns its path.
func writeTempTOML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "models.toml")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

// --- Load tests ---

func TestLoad_EmbeddedCatalog(t *testing.T) {
	specs, err := Load(EmbeddedFS, "")
	if err != nil {
		t.Fatalf("Load embedded: %v", err)
	}
	if got := len(specs); got != 10 {
		t.Fatalf("expected 10 models, got %d", got)
	}

	first := specs[0]
	if first.ID != "grok-4.20" {
		t.Errorf("first model id = %q, want %q", first.ID, "grok-4.20")
	}
	if first.DisplayName != "Grok 4.20" {
		t.Errorf("first model display_name = %q, want %q", first.DisplayName, "Grok 4.20")
	}
	if first.Type != TypeChat {
		t.Errorf("first model type = %q, want %q", first.Type, TypeChat)
	}
	if !first.Enabled {
		t.Error("first model should be enabled")
	}
	if first.PoolFloor != PoolBasic {
		t.Errorf("first model pool_floor = %q, want %q", first.PoolFloor, PoolBasic)
	}
	if first.QuotaMode != QuotaAuto {
		t.Errorf("first model quota_mode = %q, want %q", first.QuotaMode, QuotaAuto)
	}
	if first.PublicType != "chat" {
		t.Errorf("first model PublicType = %q, want %q", first.PublicType, "chat")
	}
}

func TestLoad_ExternalCatalog(t *testing.T) {
	content := minimalCatalogTOML()
	p := writeTempTOML(t, content)

	specs, err := Load(EmbeddedFS, p)
	if err != nil {
		t.Fatalf("Load external: %v", err)
	}
	if len(specs) != 1 {
		t.Fatalf("expected 1 model, got %d", len(specs))
	}
	if specs[0].ID != "test-chat" {
		t.Errorf("model id = %q, want %q", specs[0].ID, "test-chat")
	}
}

func TestLoad_ExternalFileNotFound(t *testing.T) {
	_, err := Load(EmbeddedFS, "/nonexistent/path/models.toml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "read external file") {
		t.Errorf("error = %q, want substring %q", err, "read external file")
	}
}

func TestLoad_InvalidTOML(t *testing.T) {
	p := writeTempTOML(t, "this is not valid [[[ toml")
	_, err := Load(EmbeddedFS, p)
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
	if !strings.Contains(err.Error(), "parse TOML") {
		t.Errorf("error = %q, want substring %q", err, "parse TOML")
	}
}

func TestLoad_UnknownFields(t *testing.T) {
	content := minimalCatalogTOML(`unknown_field = "x"`)
	p := writeTempTOML(t, content)
	_, err := Load(EmbeddedFS, p)
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("error = %q, want substring %q", err, "unknown field")
	}
}

// --- Validate tests ---

func TestValidate_DuplicateID(t *testing.T) {
	content := minimalCatalogTOML() + `
[[model]]
id = "test-chat"
display_name = "Dup"
type = "chat"
enabled = false
pool_floor = "basic"
quota_mode = "auto"
upstream_mode = "auto"
`
	p := writeTempTOML(t, content)
	_, err := Load(EmbeddedFS, p)
	if err == nil {
		t.Fatal("expected error for duplicate id")
	}
	if !strings.Contains(err.Error(), "duplicate id") {
		t.Errorf("error = %q, want substring %q", err, "duplicate id")
	}
}

func TestValidate_InvalidType(t *testing.T) {
	content := minimalCatalogTOML(`type = "invalid"`)
	p := writeTempTOML(t, content)
	_, err := Load(EmbeddedFS, p)
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
	if !strings.Contains(err.Error(), "invalid type") {
		t.Errorf("error = %q, want substring %q", err, "invalid type")
	}
}

func TestValidate_InvalidPoolFloor(t *testing.T) {
	content := minimalCatalogTOML(`pool_floor = "invalid"`)
	p := writeTempTOML(t, content)
	_, err := Load(EmbeddedFS, p)
	if err == nil {
		t.Fatal("expected error for invalid pool_floor")
	}
	if !strings.Contains(err.Error(), "invalid pool_floor") {
		t.Errorf("error = %q, want substring %q", err, "invalid pool_floor")
	}
}

func TestValidate_InvalidQuotaMode(t *testing.T) {
	content := minimalCatalogTOML(`quota_mode = "invalid"`)
	p := writeTempTOML(t, content)
	_, err := Load(EmbeddedFS, p)
	if err == nil {
		t.Fatal("expected error for invalid quota_mode")
	}
	if !strings.Contains(err.Error(), "invalid quota_mode") {
		t.Errorf("error = %q, want substring %q", err, "invalid quota_mode")
	}
}

func TestValidate_ExpertAllowsBasicPool(t *testing.T) {
	content := minimalCatalogTOML(
		`quota_mode = "expert"`,
		`pool_floor = "basic"`,
	)
	p := writeTempTOML(t, content)
	_, err := Load(EmbeddedFS, p)
	if err != nil {
		t.Fatalf("expert+basic should be valid, got: %v", err)
	}
}

func TestValidate_HeavyRequiresHeavy(t *testing.T) {
	content := minimalCatalogTOML(
		`quota_mode = "heavy"`,
		`pool_floor = "super"`,
	)
	p := writeTempTOML(t, content)
	_, err := Load(EmbeddedFS, p)
	if err == nil {
		t.Fatal("expected error for heavy+super")
	}
	if !strings.Contains(err.Error(), `requires pool_floor "heavy"`) {
		t.Errorf("error = %q, want substring %q", err, `requires pool_floor "heavy"`)
	}
}

func TestValidate_ChatForbidsUpstreamModel(t *testing.T) {
	content := minimalCatalogTOML(`upstream_model = "xxx"`)
	p := writeTempTOML(t, content)
	_, err := Load(EmbeddedFS, p)
	if err == nil {
		t.Fatal("expected error for chat with upstream_model")
	}
	if !strings.Contains(err.Error(), "upstream_model is forbidden") {
		t.Errorf("error = %q, want substring %q", err, "upstream_model is forbidden")
	}
}

func TestValidate_ChatRequiresUpstreamMode(t *testing.T) {
	// Remove upstream_mode by setting it to empty — but TOML doesn't allow that easily.
	// Instead, build a TOML without upstream_mode line.
	content := `version = 1

[[model]]
id = "test-chat"
display_name = "Test Chat"
type = "chat"
enabled = true
pool_floor = "basic"
quota_mode = "auto"
`
	p := writeTempTOML(t, content)
	_, err := Load(EmbeddedFS, p)
	if err == nil {
		t.Fatal("expected error for chat without upstream_mode")
	}
	if !strings.Contains(err.Error(), "upstream_mode is required") {
		t.Errorf("error = %q, want substring %q", err, "upstream_mode is required")
	}
}

func TestValidate_ImageWSForbidsUpstream(t *testing.T) {
	tests := []struct {
		name    string
		extra   string
		wantSub string
	}{
		{
			name:    "upstream_model",
			extra:   `upstream_model = "xxx"`,
			wantSub: "upstream_model is forbidden",
		},
		{
			name:    "upstream_mode",
			extra:   `upstream_mode = "xxx"`,
			wantSub: "upstream_mode is forbidden",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := `version = 1

[[model]]
id = "test-imgws"
display_name = "Test ImageWS"
type = "image_ws"
enabled = true
pool_floor = "super"
quota_mode = "auto"
` + tt.extra + "\n"
			p := writeTempTOML(t, content)
			_, err := Load(EmbeddedFS, p)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("error = %q, want substring %q", err, tt.wantSub)
			}
		})
	}
}

func TestValidate_ImageEditRequiresBoth(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantSub string
	}{
		{
			name: "missing_upstream_model",
			content: `version = 1

[[model]]
id = "test-edit"
display_name = "Test Edit"
type = "image_edit"
enabled = true
pool_floor = "super"
quota_mode = "auto"
upstream_mode = "auto"
`,
			wantSub: "upstream_model is required",
		},
		{
			name: "missing_upstream_mode",
			content: `version = 1

[[model]]
id = "test-edit"
display_name = "Test Edit"
type = "image_edit"
enabled = true
pool_floor = "super"
quota_mode = "auto"
upstream_model = "edit-model"
`,
			wantSub: "upstream_mode is required",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := writeTempTOML(t, tt.content)
			_, err := Load(EmbeddedFS, p)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("error = %q, want substring %q", err, tt.wantSub)
			}
		})
	}
}

func TestValidate_VideoRequiresBoth(t *testing.T) {
	content := `version = 1

[[model]]
id = "test-video"
display_name = "Test Video"
type = "video"
enabled = true
pool_floor = "super"
quota_mode = "auto"
upstream_mode = "auto"
`
	p := writeTempTOML(t, content)
	_, err := Load(EmbeddedFS, p)
	if err == nil {
		t.Fatal("expected error for video without upstream_model")
	}
	if !strings.Contains(err.Error(), "upstream_model is required") {
		t.Errorf("error = %q, want substring %q", err, "upstream_model is required")
	}
}

func TestValidate_ForceThinkingOnlyChat(t *testing.T) {
	content := `version = 1

[[model]]
id = "test-imgws"
display_name = "Test ImageWS"
type = "image_ws"
enabled = true
pool_floor = "super"
quota_mode = "auto"
force_thinking = true
`
	p := writeTempTOML(t, content)
	_, err := Load(EmbeddedFS, p)
	if err == nil {
		t.Fatal("expected error for force_thinking on non-chat")
	}
	if !strings.Contains(err.Error(), "force_thinking is only valid for type") {
		t.Errorf("error = %q, want substring %q", err, "force_thinking is only valid for type")
	}
}

func TestValidate_EnableProOnlyImageWS(t *testing.T) {
	content := minimalCatalogTOML(`enable_pro = true`)
	p := writeTempTOML(t, content)
	_, err := Load(EmbeddedFS, p)
	if err == nil {
		t.Fatal("expected error for enable_pro on chat")
	}
	if !strings.Contains(err.Error(), "enable_pro is only valid for type") {
		t.Errorf("error = %q, want substring %q", err, "enable_pro is only valid for type")
	}
}

func TestValidate_NoEnabledModel(t *testing.T) {
	content := minimalCatalogTOML(`enabled = false`)
	p := writeTempTOML(t, content)
	_, err := Load(EmbeddedFS, p)
	if err == nil {
		t.Fatal("expected error for no enabled model")
	}
	if !strings.Contains(err.Error(), "at least one model must be enabled") {
		t.Errorf("error = %q, want substring %q", err, "at least one model must be enabled")
	}
}

func TestValidate_VersionNot1(t *testing.T) {
	content := strings.Replace(minimalCatalogTOML(), "version = 1", "version = 2", 1)
	p := writeTempTOML(t, content)
	_, err := Load(EmbeddedFS, p)
	if err == nil {
		t.Fatal("expected error for version != 1")
	}
	if !strings.Contains(err.Error(), "unsupported version") {
		t.Errorf("error = %q, want substring %q", err, "unsupported version")
	}
}

func TestValidate_EmptyModels(t *testing.T) {
	content := "version = 1\n"
	p := writeTempTOML(t, content)
	_, err := Load(EmbeddedFS, p)
	if err == nil {
		t.Fatal("expected error for empty models")
	}
	if !strings.Contains(err.Error(), "at least one model") {
		t.Errorf("error = %q, want substring %q", err, "at least one model")
	}
}

func TestDerivePublicType(t *testing.T) {
	tests := []struct {
		internal string
		want     string
	}{
		{TypeChat, "chat"},
		{TypeImageWS, "image_ws"},
		{TypeImageLite, "image"},
		{TypeImageEdit, "image_edit"},
		{TypeVideo, "video"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.internal, func(t *testing.T) {
			got := DerivePublicType(tt.internal)
			if got != tt.want {
				t.Errorf("DerivePublicType(%q) = %q, want %q", tt.internal, got, tt.want)
			}
		})
	}
}
