package transport

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveProfile(t *testing.T) {
	tests := []struct {
		name      string
		candidate string
		want      string
	}{
		{"empty", "", ""},
		{"exact", "chrome136", "chrome136"},
		{"uppercase", "CHROME136", "chrome136"},
		{"dash separated", "Chrome-136", "chrome136"},
		{"underscore separated", "chrome_136", "chrome136"},
		{"surrounding spaces", "  chrome136  ", "chrome136"},
		{"firefox exact", "firefox135", "firefox135"},
		{"safari family via digits", "safari999", "safari"},
		{"chrome family via digits", "chrome999", "chrome"},
		{"firefox family with trailing letters", "firefox999x", "firefox"},
		{"unsupported family", "edge120", ""},
		{"letters only unknown", "safari_ios", ""},
		{"fully unknown", "lynx", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ResolveProfile(tt.candidate))
		})
	}
}

func TestIsSupportedProfile(t *testing.T) {
	tests := []struct {
		candidate string
		want      bool
	}{
		{"chrome136", true},
		{"chrome999", true},
		{"Firefox-144", true},
		{"edge120", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.candidate, func(t *testing.T) {
			assert.Equal(t, tt.want, IsSupportedProfile(tt.candidate))
		})
	}
}

func TestSupportedProfileList(t *testing.T) {
	list := SupportedProfileList()
	require.Len(t, list, len(SupportedProfiles))
	for i := 1; i < len(list); i++ {
		require.Less(t, list[i-1], list[i], "list must be sorted")
	}

	// The returned slice must be a copy; mutating it must not affect the
	// package-level source of truth.
	if len(list) > 0 {
		list[0] = "mutated"
		assert.NotEqual(t, "mutated", SupportedProfiles[0])
	}
}

func TestDefaultUserAgent(t *testing.T) {
	assert.Equal(t, ProfileUAMap[DefaultProfile], DefaultUserAgent())
	assert.NotEmpty(t, DefaultUserAgent())
}

func TestUserAgentForProfile(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		wantUA  string
		wantOK  bool
	}{
		{"exact match", "chrome136", ProfileUAMap["chrome136"], true},
		{"normalized match", "Chrome-136", ProfileUAMap["chrome136"], true},
		{"family fallback", "chrome999", ProfileUAMap["chrome"], true},
		{"unknown profile", "edge120", "", false},
		{"empty profile", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ua, ok := UserAgentForProfile(tt.profile)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.wantUA, ua)
		})
	}
}

func TestBrowserFromUserAgent(t *testing.T) {
	firefox := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:133.0) Gecko/20100101 Firefox/133.0"
	chrome := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"
	chromium := "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chromium/133.0.0.0 Safari/537.36"
	crios := "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/131.0.0.0 Mobile/15E148 Safari/604.1"
	safari := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.4 Safari/605.1.15"
	safariIOS := "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1"
	edge := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36 Edg/120.0.0.0"
	androidChrome := "Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Mobile Safari/537.36"
	oldChrome := "Mozilla/5.0 (Windows NT 10.0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/99.0.0.0 Safari/537.36"

	tests := []struct {
		name string
		ua   string
		want string
	}{
		{"firefox resolves version profile", firefox, "firefox133"},
		{"firefox ahead of supported set falls back to family", "Mozilla/5.0 rv:999.0 Gecko/20100101 Firefox/999.0", "firefox"},
		{"chrome resolves version profile", chrome, "chrome136"},
		{"chromium falls back to family", chromium, "chrome"},
		{"crios resolves version profile", crios, "chrome131"},
		{"edge has no supported profile", edge, ""},
		{"android chrome falls back to family", androidChrome, "chrome"},
		{"old chrome falls back to family", oldChrome, "chrome"},
		{"desktop safari resolves family profile", safari, "safari"},
		{"ios safari has no ios profile", safariIOS, ""},
		{"unknown browser", "curl/8.4.0", ""},
		{"empty user agent", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, BrowserFromUserAgent(tt.ua))
		})
	}
}

func TestEffectiveProfile(t *testing.T) {
	chromeUA := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"
	firefoxUA := "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:135.0) Gecko/20100101 Firefox/135.0"

	tests := []struct {
		name      string
		browser   string
		userAgent string
		want      string
	}{
		{"user agent wins over browser", "firefox135", chromeUA, "chrome136"},
		{"browser used without user agent", "firefox135", "", "firefox135"},
		{"unsupported browser falls back to default", "edge120", "", DefaultProfile},
		{"browser family resolution", "chrome999", "", "chrome"},
		{"nothing set falls back to default", "", "", DefaultProfile},
		{"unsupported user agent falls to browser", "safari184", "curl/8.4.0", "safari184"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, EffectiveProfile(tt.browser, tt.userAgent))
		})
	}

	// A user agent that resolves keeps its profile even when the browser
	// field would resolve differently.
	assert.Equal(t, "firefox135", EffectiveProfile("chrome136", firefoxUA))
}

func TestProfileUAMapCoversSupportedProfiles(t *testing.T) {
	for _, profile := range SupportedProfiles {
		ua, ok := ProfileUAMap[profile]
		require.True(t, ok, "profile %q must have a paired user agent", profile)
		assert.NotEmpty(t, ua)
	}
}
