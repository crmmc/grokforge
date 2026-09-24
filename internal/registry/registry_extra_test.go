package registry

import (
	"testing"

	"github.com/crmmc/grokforge/internal/modelconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistry_ResolveMode(t *testing.T) {
	specs := []modelconfig.ModelSpec{
		{ID: "grok-4", Type: "chat", Enabled: true, PoolFloor: "basic", Mode: "auto", PublicType: "chat"},
		{ID: "flux-1", Type: "image_ws", Enabled: true, PoolFloor: "basic", PublicType: "image_ws"},
	}
	reg := NewModelRegistry(specs, testModes())

	tests := []struct {
		name      string
		requestID string
		wantMode  string
		wantOK    bool
	}{
		{name: "quota-tracked model resolves its mode", requestID: "grok-4", wantMode: "auto", wantOK: true},
		{name: "model without mode resolves empty mode", requestID: "flux-1", wantMode: "", wantOK: true},
		{name: "unknown model fails", requestID: "nonexistent", wantMode: "", wantOK: false},
		{name: "disabled model is absent", requestID: "disabled-model", wantMode: "", wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mode, ok := reg.ResolveMode(tc.requestID)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantMode, mode)
		})
	}
}

func TestRegistry_ResolveMode_EmptyRegistry(t *testing.T) {
	reg := NewModelRegistry(nil, nil)
	mode, ok := reg.ResolveMode("anything")
	require.False(t, ok)
	assert.Empty(t, mode)
}
