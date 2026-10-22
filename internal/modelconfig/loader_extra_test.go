package modelconfig

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_EmbeddedCatalogMissing(t *testing.T) {
	// An empty FS has no models.toml: the embedded read must fail.
	_, _, err := Load(fstest.MapFS{}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read embedded catalog")
}

func TestValidate_UnsupportedVersion(t *testing.T) {
	err := validate(2, []ModeSpec{validMode()}, []ModelSpec{validModel()})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported version 2")
}

func TestValidate_NoModels(t *testing.T) {
	err := validate(1, []ModeSpec{validMode()}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "catalog must contain at least one model")
}

// validMode returns a minimal valid ModeSpec for direct validate() calls.
func validMode() ModeSpec {
	return ModeSpec{
		ID:            "auto",
		UpstreamName:  "auto",
		WindowSeconds: 7200,
		DefaultQuota:  map[string]int{PoolBasic: 30, PoolSuper: 30, PoolHeavy: 30},
	}
}

// validModel returns a minimal valid quota-tracked chat ModelSpec.
func validModel() ModelSpec {
	return ModelSpec{
		ID:           "test-chat",
		Type:         TypeChat,
		Enabled:      true,
		PoolFloor:    PoolBasic,
		Mode:         "auto",
		UpstreamMode: "auto",
	}
}

// TestValidate_FlagRestrictions covers the remaining per-model enum and flag
// restrictions (pool floor enum, force_thinking, enable_pro).
func TestValidate_FlagRestrictions(t *testing.T) {
	quotaSyncFalse := false
	tests := []struct {
		name  string
		model ModelSpec
		want  string
	}{
		{
			name:  "invalid pool_floor",
			model: ModelSpec{ID: "m", Type: TypeChat, Enabled: true, PoolFloor: "gold", Mode: "auto", UpstreamMode: "auto"},
			want:  `invalid pool_floor "gold"`,
		},
		{
			name:  "force_thinking only valid for chat",
			model: ModelSpec{ID: "m", Type: TypeImageWS, Enabled: true, PoolFloor: PoolBasic, QuotaSync: &quotaSyncFalse, CooldownSeconds: 60, ForceThinking: true},
			want:  `force_thinking is only valid for type "chat"`,
		},
		{
			name:  "enable_pro only valid for image_ws",
			model: ModelSpec{ID: "m", Type: TypeChat, Enabled: true, PoolFloor: PoolBasic, Mode: "auto", UpstreamMode: "auto", EnablePro: true},
			want:  `enable_pro is only valid for type "image_ws"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validate(1, []ModeSpec{validMode()}, []ModelSpec{tc.model})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// TestValidateUpstream_BranchTable exercises every validateUpstream branch
// through validate() with a valid mode context.
func TestValidateUpstream_BranchTable(t *testing.T) {
	quotaSyncFalse := false
	tests := []struct {
		name  string
		model ModelSpec
		want  string
	}{
		{
			name:  "chat requires upstream_mode",
			model: ModelSpec{ID: "m", Type: TypeChat, Enabled: true, PoolFloor: PoolBasic, Mode: "auto"},
			want:  `upstream_mode is required for type "chat"`,
		},
		{
			name:  "image_lite requires upstream_mode",
			model: ModelSpec{ID: "m", Type: TypeImageLite, Enabled: true, PoolFloor: PoolBasic, Mode: "auto"},
			want:  `upstream_mode is required for type "image_lite"`,
		},
		{
			name:  "chat forbids upstream_model",
			model: ModelSpec{ID: "m", Type: TypeChat, Enabled: true, PoolFloor: PoolBasic, Mode: "auto", UpstreamModel: "x", UpstreamMode: "auto"},
			want:  `upstream_model is forbidden for type "chat"`,
		},
		{
			name:  "image_edit requires upstream_model",
			model: ModelSpec{ID: "m", Type: TypeImageEdit, Enabled: true, PoolFloor: PoolBasic, Mode: "auto", UpstreamMode: "auto"},
			want:  `upstream_model is required for type "image_edit"`,
		},
		{
			name:  "image_edit requires upstream_mode",
			model: ModelSpec{ID: "m", Type: TypeImageEdit, Enabled: true, PoolFloor: PoolBasic, Mode: "auto", UpstreamModel: "x"},
			want:  `upstream_mode is required for type "image_edit"`,
		},
		{
			name:  "video requires upstream_model",
			model: ModelSpec{ID: "m", Type: TypeVideo, Enabled: true, PoolFloor: PoolBasic, Mode: "auto", UpstreamMode: "auto"},
			want:  `upstream_model is required for type "video"`,
		},
		{
			name:  "video requires upstream_mode",
			model: ModelSpec{ID: "m", Type: TypeVideo, Enabled: true, PoolFloor: PoolBasic, Mode: "auto", UpstreamModel: "x"},
			want:  `upstream_mode is required for type "video"`,
		},
		{
			name:  "image_ws forbids upstream_model",
			model: ModelSpec{ID: "m", Type: TypeImageWS, Enabled: true, PoolFloor: PoolBasic, QuotaSync: &quotaSyncFalse, CooldownSeconds: 300, UpstreamModel: "x"},
			want:  `upstream_model is forbidden for type "image_ws"`,
		},
		{
			name:  "image_ws forbids upstream_mode",
			model: ModelSpec{ID: "m", Type: TypeImageWS, Enabled: true, PoolFloor: PoolBasic, QuotaSync: &quotaSyncFalse, CooldownSeconds: 300, UpstreamMode: "auto"},
			want:  `upstream_mode is forbidden for type "image_ws"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validate(1, []ModeSpec{validMode()}, []ModelSpec{tc.model})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}
