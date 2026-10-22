package xai

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientOptionsApply(t *testing.T) {
	tests := []struct {
		name   string
		opts   []ClientOption
		verify func(t *testing.T, c *client)
	}{
		{
			name: "dynamic statsig disabled uses static id",
			opts: []ClientOption{WithDynamicStatsig(false)},
			verify: func(t *testing.T, c *client) {
				assert.Equal(t, staticStatsigID, c.statsigID)
				assert.False(t, c.opts.DynamicStatsig)
			},
		},
		{
			name: "dynamic statsig enabled leaves id empty",
			opts: []ClientOption{WithDynamicStatsig(true)},
			verify: func(t *testing.T, c *client) {
				assert.Empty(t, c.statsigID)
				assert.True(t, c.opts.DynamicStatsig)
			},
		},
		{
			name: "browser profile",
			opts: []ClientOption{WithBrowser("firefox135")},
			verify: func(t *testing.T, c *client) {
				assert.Equal(t, "firefox135", c.opts.Browser)
			},
		},
		{
			name: "cf clearance",
			opts: []ClientOption{WithCFClearance("cf-val")},
			verify: func(t *testing.T, c *client) {
				assert.Equal(t, "cf-val", c.opts.CFClearance)
			},
		},
		{
			name: "cf cookies",
			opts: []ClientOption{WithCFCookies("cfk=1")},
			verify: func(t *testing.T, c *client) {
				assert.Equal(t, "cfk=1", c.opts.CFCookies)
			},
		},
		{
			name: "proxy",
			opts: []ClientOption{WithProxy("http://proxy.local:8080")},
			verify: func(t *testing.T, c *client) {
				assert.Equal(t, "http://proxy.local:8080", c.opts.ProxyURL)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewClient("tok", tt.opts...)
			require.NoError(t, err)
			defer c.Close()
			tt.verify(t, c.(*client))
		})
	}
}
