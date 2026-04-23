package modelconfig

import (
	"fmt"
	"io/fs"
	"os"

	"github.com/BurntSushi/toml"
)

// catalogFile is the top-level TOML structure.
type catalogFile struct {
	Version int         `toml:"version"`
	Modes   []ModeSpec  `toml:"mode"`
	Models  []ModelSpec `toml:"model"`
}

// IsQuotaTracked reports whether the model participates in mode quota tracking.
// Default is true (QuotaSync nil); only explicit quota_sync = false opts out.
func (m *ModelSpec) IsQuotaTracked() bool {
	return m.QuotaSync == nil || *m.QuotaSync
}

// Load loads the model catalog from the embedded FS or an external file.
// Returns validated model specs (with PublicType derived), mode specs, or an error.
func Load(embeddedFS fs.FS, externalPath string) ([]ModelSpec, []ModeSpec, error) {
	data, err := readCatalog(embeddedFS, externalPath)
	if err != nil {
		return nil, nil, err
	}

	var cat catalogFile
	md, err := toml.Decode(string(data), &cat)
	if err != nil {
		return nil, nil, fmt.Errorf("modelconfig: parse TOML: %w", err)
	}

	// Reject unknown fields.
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return nil, nil, fmt.Errorf("modelconfig: unknown field %q", undecoded[0])
	}

	if err := validate(cat.Version, cat.Modes, cat.Models); err != nil {
		return nil, nil, err
	}

	// Derive PublicType for each model.
	for i := range cat.Models {
		cat.Models[i].PublicType = DerivePublicType(cat.Models[i].Type)
	}

	return cat.Models, cat.Modes, nil
}

// readCatalog reads raw bytes from the appropriate source.
func readCatalog(embeddedFS fs.FS, externalPath string) ([]byte, error) {
	if externalPath != "" {
		data, err := os.ReadFile(externalPath)
		if err != nil {
			return nil, fmt.Errorf("modelconfig: read external file: %w", err)
		}
		return data, nil
	}
	data, err := fs.ReadFile(embeddedFS, "models.toml")
	if err != nil {
		return nil, fmt.Errorf("modelconfig: read embedded catalog: %w", err)
	}
	return data, nil
}

// validTypes is the set of allowed type values.
var validTypes = map[string]bool{
	TypeChat: true, TypeImageWS: true, TypeImageLite: true,
	TypeImageEdit: true, TypeVideo: true,
}

// validPoolFloors is the set of allowed pool_floor values.
var validPoolFloors = map[string]bool{
	PoolBasic: true, PoolSuper: true, PoolHeavy: true,
}

// requiredPools is the set of pool keys that every mode must define in default_quota.
var requiredPools = []string{PoolBasic, PoolSuper, PoolHeavy}

// validate checks all semantic rules on the parsed catalog.
func validate(version int, modes []ModeSpec, models []ModelSpec) error {
	if version != 1 {
		return fmt.Errorf("modelconfig: unsupported version %d (expected 1)", version)
	}

	// --- Mode validation ---
	if len(modes) == 0 {
		return fmt.Errorf("modelconfig: catalog must contain at least one mode")
	}

	modeIDs := make(map[string]bool, len(modes))
	upstreamNames := make(map[string]bool, len(modes))

	for _, mode := range modes {
		if modeIDs[mode.ID] {
			return fmt.Errorf("mode %q: duplicate id", mode.ID)
		}
		modeIDs[mode.ID] = true

		if mode.UpstreamName == "" {
			return fmt.Errorf("mode %q: upstream_name is required", mode.ID)
		}
		if upstreamNames[mode.UpstreamName] {
			return fmt.Errorf("mode %q: duplicate upstream_name %q", mode.ID, mode.UpstreamName)
		}
		upstreamNames[mode.UpstreamName] = true

		if mode.WindowSeconds <= 0 {
			return fmt.Errorf("mode %q: window_seconds must be > 0", mode.ID)
		}

		for _, pool := range requiredPools {
			if _, ok := mode.DefaultQuota[pool]; !ok {
				return fmt.Errorf("mode %q: default_quota missing pool %q", mode.ID, pool)
			}
		}
	}

	// --- Model validation ---
	if len(models) == 0 {
		return fmt.Errorf("modelconfig: catalog must contain at least one model")
	}

	seen := make(map[string]bool, len(models))
	hasEnabled := false

	for _, m := range models {
		// Unique ID.
		if seen[m.ID] {
			return fmt.Errorf("model %q: duplicate id", m.ID)
		}
		seen[m.ID] = true

		// Enum checks.
		if !validTypes[m.Type] {
			return fmt.Errorf("model %q: invalid type %q", m.ID, m.Type)
		}
		if !validPoolFloors[m.PoolFloor] {
			return fmt.Errorf("model %q: invalid pool_floor %q", m.ID, m.PoolFloor)
		}

		// Quota tracking rules.
		if m.IsQuotaTracked() {
			if m.Mode == "" {
				return fmt.Errorf("model %q: mode is required for quota-tracked models", m.ID)
			}
			if !modeIDs[m.Mode] {
				return fmt.Errorf("model %q: mode %q does not match any defined mode", m.ID, m.Mode)
			}
			if m.CooldownSeconds != 0 {
				return fmt.Errorf("model %q: cooldown_seconds is forbidden for quota-tracked models", m.ID)
			}
		} else {
			if m.Type != TypeImageWS {
				return fmt.Errorf("model %q: quota_sync = false is only valid for type %q", m.ID, TypeImageWS)
			}
			if m.Mode != "" {
				return fmt.Errorf("model %q: mode is forbidden when quota_sync = false", m.ID)
			}
			if m.CooldownSeconds <= 0 {
				return fmt.Errorf("model %q: cooldown_seconds must be > 0 when quota_sync = false", m.ID)
			}
		}

		// Upstream field rules per type.
		if err := validateUpstream(m); err != nil {
			return err
		}

		// Flag restrictions.
		if m.ForceThinking && m.Type != TypeChat {
			return fmt.Errorf("model %q: force_thinking is only valid for type %q", m.ID, TypeChat)
		}
		if m.EnablePro && m.Type != TypeImageWS {
			return fmt.Errorf("model %q: enable_pro is only valid for type %q", m.ID, TypeImageWS)
		}

		if m.Enabled {
			hasEnabled = true
		}
	}

	if !hasEnabled {
		return fmt.Errorf("modelconfig: at least one model must be enabled")
	}
	return nil
}

// validateUpstream checks upstream_model and upstream_mode rules per model type.
func validateUpstream(m ModelSpec) error {
	switch m.Type {
	case TypeChat, TypeImageLite:
		// Must define upstream_mode, must NOT define upstream_model.
		if m.UpstreamMode == "" {
			return fmt.Errorf("model %q: upstream_mode is required for type %q", m.ID, m.Type)
		}
		if m.UpstreamModel != "" {
			return fmt.Errorf("model %q: upstream_model is forbidden for type %q", m.ID, m.Type)
		}

	case TypeImageEdit, TypeVideo:
		// Must define both upstream_model and upstream_mode.
		if m.UpstreamModel == "" {
			return fmt.Errorf("model %q: upstream_model is required for type %q", m.ID, m.Type)
		}
		if m.UpstreamMode == "" {
			return fmt.Errorf("model %q: upstream_mode is required for type %q", m.ID, m.Type)
		}

	case TypeImageWS:
		// Must NOT define upstream_model or upstream_mode.
		if m.UpstreamModel != "" {
			return fmt.Errorf("model %q: upstream_model is forbidden for type %q", m.ID, m.Type)
		}
		if m.UpstreamMode != "" {
			return fmt.Errorf("model %q: upstream_mode is forbidden for type %q", m.ID, m.Type)
		}
	}
	return nil
}
