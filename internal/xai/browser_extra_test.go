package xai

import (
	"testing"

	"github.com/crmmc/grokforge/internal/upstream/transport"
	"github.com/stretchr/testify/assert"
)

func TestResolveBrowserProfile(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"supported profile", "chrome136", "chrome136"},
		{"normalized input", "Chrome-136", "chrome136"},
		{"family fallback", "chrome999", "chrome"},
		{"firefox family", "firefox999", "firefox"},
		{"unknown falls back to default", "mosaic", transport.DefaultProfile},
		{"empty falls back to default", "", transport.DefaultProfile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ResolveBrowserProfile(tt.input))
		})
	}
}
