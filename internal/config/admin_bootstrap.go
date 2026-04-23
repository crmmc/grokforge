package config

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"strings"
)

const (
	bootstrapAppKeyPrefix = "gf_bootstrap_"
	bootstrapEntropyBytes = 24
)

// EnsureAdminAppKey injects a temporary in-memory admin app_key when none is configured.
// The generated key is process-local and must not be persisted by callers.
func EnsureAdminAppKey(cfg *Config, entropy io.Reader) (string, bool, error) {
	if cfg == nil {
		return "", false, nil
	}
	if strings.TrimSpace(cfg.App.AppKey) != "" {
		return "", false, nil
	}
	if entropy == nil {
		entropy = rand.Reader
	}

	buf := make([]byte, bootstrapEntropyBytes)
	if _, err := io.ReadFull(entropy, buf); err != nil {
		return "", false, err
	}

	appKey := bootstrapAppKeyPrefix + base64.RawURLEncoding.EncodeToString(buf)
	cfg.App.AppKey = appKey
	return appKey, true, nil
}
