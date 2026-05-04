package modelconfig

// Internal runtime type constants.
const (
	TypeChat      = "chat"
	TypeImageWS   = "image_ws"
	TypeImageLite = "image_lite"
	TypeImageEdit = "image_edit"
	TypeVideo     = "video"
)

// Pool floor constants.
const (
	PoolBasic = "basic"
	PoolSuper = "super"
	PoolHeavy = "heavy"
)

// ModeSpec represents a quota mode entry in the static catalog.
type ModeSpec struct {
	ID            string         `toml:"id"`
	UpstreamName  string         `toml:"upstream_name"`
	WindowSeconds int            `toml:"window_seconds"`
	DefaultQuota  map[string]int `toml:"default_quota"` // pool -> default quota
	LocalQuota    bool           `toml:"local_quota,omitempty"` // skip upstream rate-limits sync
}

// ModelSpec represents a single model entry in the static catalog.
type ModelSpec struct {
	ID              string `toml:"id"`
	DisplayName     string `toml:"display_name"`
	Type            string `toml:"type"`
	Enabled         bool   `toml:"enabled"`
	PoolFloor       string `toml:"pool_floor"`
	Mode            string `toml:"mode,omitempty"`
	QuotaSync       *bool  `toml:"quota_sync,omitempty"`
	CooldownSeconds int    `toml:"cooldown_seconds,omitempty"`
	UpstreamModel   string `toml:"upstream_model,omitempty"`
	UpstreamMode    string `toml:"upstream_mode,omitempty"`
	ForceThinking   bool   `toml:"force_thinking,omitempty"`
	EnablePro       bool   `toml:"enable_pro,omitempty"`

	PublicType string `toml:"-"` // derived at load time, not serialized
}

// DerivePublicType maps an internal runtime type to its public API type.
func DerivePublicType(internalType string) string {
	switch internalType {
	case TypeChat:
		return "chat"
	case TypeImageWS:
		return "image_ws"
	case TypeImageLite:
		return "image"
	case TypeImageEdit:
		return "image_edit"
	case TypeVideo:
		return "video"
	default:
		return internalType
	}
}
