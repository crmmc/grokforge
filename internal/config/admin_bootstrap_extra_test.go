package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureAdminAppKey_NilConfigIsNoOp(t *testing.T) {
	key, generated, err := EnsureAdminAppKey(nil, nil)
	require.NoError(t, err)
	assert.False(t, generated)
	assert.Empty(t, key)
}

func TestEnsureAdminAppKey_NilEntropyUsesCryptoRand(t *testing.T) {
	cfg := &Config{}

	key, generated, err := EnsureAdminAppKey(cfg, nil)
	require.NoError(t, err)
	assert.True(t, generated)
	assert.Contains(t, key, bootstrapAppKeyPrefix)
	assert.Equal(t, key, cfg.App.AppKey)

	// Two runs with nil entropy must produce different keys.
	other := &Config{}
	otherKey, _, err := EnsureAdminAppKey(other, nil)
	require.NoError(t, err)
	assert.NotEqual(t, key, otherKey)
}

func TestEnsureAdminAppKey_WhitespaceAppKeyStillGenerates(t *testing.T) {
	cfg := &Config{App: AppConfig{AppKey: "   "}}

	key, generated, err := EnsureAdminAppKey(cfg, nil)
	require.NoError(t, err)
	assert.True(t, generated, "blank app_key counts as missing and must be replaced")
	assert.NotEmpty(t, key)
	assert.Equal(t, key, cfg.App.AppKey)
}
