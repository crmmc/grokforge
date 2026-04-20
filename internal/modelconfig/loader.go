package modelconfig

import (
	"embed"
	"fmt"
	"io/fs"
	"os"

	"github.com/BurntSushi/toml"
)

// catalogFile is the top-level TOML structure.
type catalogFile struct {
	Version int         `toml:"version"`
	Models  []ModelSpec `toml:"model"`
}

// Load loads the model catalog from the embedded FS or an external file.
// If externalPath is non-empty, it loads from that file (resolved as-is by the caller).
// If externalPath is empty, it loads from the embedded FS.
// Returns validated specs with PublicType derived, or an error.
func Load(embeddedFS embed.FS, externalPath string) ([]ModelSpec, error) {
	data, err := readCatalog(embeddedFS, externalPath)
	if err != nil {
		return nil, err
	}

	var cat catalogFile
	md, err := toml.Decode(string(data), &cat)
	if err != nil {
		return nil, fmt.Errorf("modelconfig: parse TOML: %w", err)
	}

	// Reject unknown fields.
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return nil, fmt.Errorf("modelconfig: unknown field %q", undecoded[0])
	}

	if err := validate(cat.Version, cat.Models); err != nil {
		return nil, err
	}

	// Derive PublicType for each model.
	for i := range cat.Models {
		cat.Models[i].PublicType = DerivePublicType(cat.Models[i].Type)
	}

	return cat.Models, nil
}

// readCatalog reads raw bytes from the appropriate source.
func readCatalog(embeddedFS embed.FS, externalPath string) ([]byte, error) {
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

// validQuotaModes is the set of allowed quota_mode values.
var validQuotaModes = map[string]bool{
	QuotaAuto: true, QuotaFast: true, QuotaExpert: true, QuotaHeavy: true,
}

// validate checks all semantic rules on the parsed catalog.
func validate(version int, models []ModelSpec) error {
	if version != 1 {
		return fmt.Errorf("modelconfig: unsupported version %d (expected 1)", version)
	}
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
		if !validQuotaModes[m.QuotaMode] {
			return fmt.Errorf("model %q: invalid quota_mode %q", m.ID, m.QuotaMode)
		}

		// Quota / pool compatibility.
		// quota_mode "expert" has no pool_floor restriction (tokens are picked from the floor up).
		if m.QuotaMode == QuotaHeavy && m.PoolFloor != PoolHeavy {
			return fmt.Errorf("model %q: quota_mode %q requires pool_floor %q", m.ID, QuotaHeavy, PoolHeavy)
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
