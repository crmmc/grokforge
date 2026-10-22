package xai

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractMajorVersionFromUA(t *testing.T) {
	tests := []struct {
		name string
		ua   string
		want string
	}{
		{"chrome", "Mozilla/5.0 AppleWebKit/537.36 Chrome/136.0.0.0", "136"},
		{"chromium", "Mozilla/5.0 Chromium/120.0", "120"},
		{"edge", "Mozilla/5.0 Edg/120.0.0", "120"},
		{"no match", "Mozilla/5.0 Gecko/20100101", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractMajorVersionFromUA(tt.ua))
		})
	}
}

func TestBuildClientHintsVariants(t *testing.T) {
	tests := []struct {
		name    string
		browser string
		ua      string
		check   func(t *testing.T, h clientHints)
	}{
		{
			name:    "android platform and mobile flag",
			browser: "chrome136",
			ua:      "Mozilla/5.0 (Linux; Android 13) Chrome/136.0.0.0 Mobile",
			check: func(t *testing.T, h clientHints) {
				assert.Equal(t, `"Android"`, h.SecChUaPlatform)
				assert.Equal(t, "?1", h.SecChUaMobile)
			},
		},
		{
			name:    "mobile ua flag with macOS",
			browser: "chrome136",
			ua:      "Mozilla/5.0 (Mobile) Chrome/136.0.0.0",
			check: func(t *testing.T, h clientHints) {
				assert.Equal(t, "?1", h.SecChUaMobile)
				assert.Equal(t, `"macOS"`, h.SecChUaPlatform)
			},
		},
		{
			name:    "arm architecture",
			browser: "chrome136",
			ua:      "Mozilla/5.0 (Macintosh; aarch64) Chrome/136.0.0.0",
			check: func(t *testing.T, h clientHints) {
				assert.Equal(t, "arm", h.SecChUaArch)
				assert.Equal(t, "64", h.SecChUaBitness)
			},
		},
		{
			name:    "safari ua has no hints",
			browser: "",
			ua:      "Mozilla/5.0 (Macintosh) Version/18.0 Safari/605.1.15",
			check: func(t *testing.T, h clientHints) {
				assert.Empty(t, h.SecChUa)
				assert.Empty(t, h.SecChUaPlatform)
			},
		},
		{
			name:    "safari browser name has no hints",
			browser: "safari184",
			ua:      "Mozilla/5.0 (Macintosh)",
			check: func(t *testing.T, h clientHints) {
				assert.Empty(t, h.SecChUa)
			},
		},
		{
			name:    "brave counts as chromium",
			browser: "brave",
			ua:      "Mozilla/5.0 Chrome/136.0.0.0",
			check: func(t *testing.T, h clientHints) {
				assert.NotEmpty(t, h.SecChUa)
			},
		},
		{
			name:    "edge ua counts as chromium",
			browser: "chrome136",
			ua:      "Mozilla/5.0 Edg/120.0.0",
			check: func(t *testing.T, h clientHints) {
				assert.NotEmpty(t, h.SecChUa)
				// Browser profile version wins over the UA version.
				assert.Equal(t, "136", versionInSecChUa(h.SecChUa))
			},
		},
		{
			name:    "fallback version 146 without digits",
			browser: "chrome",
			ua:      "Mozilla/5.0 (Windows NT 10.0)",
			check: func(t *testing.T, h clientHints) {
				assert.Equal(t, "146", versionInSecChUa(h.SecChUa))
				assert.Equal(t, `"Windows"`, h.SecChUaPlatform)
				assert.Empty(t, h.SecChUaArch)
				assert.Empty(t, h.SecChUaBitness)
				assert.Equal(t, "?0", h.SecChUaMobile)
			},
		},
		{
			name:    "desktop chromium without arch",
			browser: "chrome136",
			ua:      "Mozilla/5.0 (X11; Linux) Chrome/136.0.0.0",
			check: func(t *testing.T, h clientHints) {
				assert.Equal(t, `"Linux"`, h.SecChUaPlatform)
				assert.Empty(t, h.SecChUaArch)
				assert.Empty(t, h.SecChUaBitness)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, buildClientHints(tt.browser, tt.ua))
		})
	}
}

// versionInSecChUa extracts the first v="..." value from a Sec-Ch-Ua string.
func versionInSecChUa(secChUa string) string {
	start := strings.Index(secChUa, `v="`)
	if start < 0 {
		return ""
	}
	rest := secChUa[start+3:]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	return rest[:end]
}

func TestSSOCookieAppendsClearance(t *testing.T) {
	tests := []struct {
		name      string
		cfCookies string
		cfClear   string
		want      string
	}{
		{"appended to existing cookies", "a=1; b=2", "XYZ", "sso=T; sso-rw=T; a=1; b=2; cf_clearance=XYZ"},
		{"trims trailing separators", "a=1; ", "XYZ", "sso=T; sso-rw=T; a=1; cf_clearance=XYZ"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, ssoCookie("T", tt.cfCookies, tt.cfClear))
		})
	}
}

func TestBuildHeadersWithOriginDefaults(t *testing.T) {
	opts := &Options{UserAgent: "plain-ua/1", Browser: "chrome136"}

	t.Run("empty origin and referer fall back to grok.com", func(t *testing.T) {
		h := buildHeadersWithOrigin("tok", opts, "sid", "", "")
		assert.Equal(t, "https://grok.com", h.Get("Origin"))
		assert.Equal(t, "https://grok.com/", h.Get("Referer"))
	})

	t.Run("empty referer derived from origin", func(t *testing.T) {
		h := buildHeadersWithOrigin("tok", opts, "sid", "https://console.x.ai", "")
		assert.Equal(t, "https://console.x.ai", h.Get("Origin"))
		assert.Equal(t, "https://console.x.ai/", h.Get("Referer"))
	})

	t.Run("provided origin and referer win", func(t *testing.T) {
		h := buildHeadersWithOrigin("tok", opts, "sid", "https://a.example", "https://a.example/page")
		assert.Equal(t, "https://a.example", h.Get("Origin"))
		assert.Equal(t, "https://a.example/page", h.Get("Referer"))
	})
}

func TestBuildHeadersWithCFCookiesAndLongToken(t *testing.T) {
	opts := &Options{
		UserAgent:   "Mozilla/5.0 Chrome/136.0.0.0",
		Browser:     "chrome136",
		CFCookies:   "cfA=1; cfB=2",
		CFClearance: "CLR",
	}
	longToken := strings.Repeat("x", 24)
	h := buildHeaders(longToken, opts, "sid")

	cookie := h.Get("Cookie")
	assert.Contains(t, cookie, "sso="+longToken)
	assert.Contains(t, cookie, "cfA=1")
	assert.Contains(t, cookie, "cf_clearance=CLR")

	// Token is long enough to exercise the log-masking branch.
	// x-statsig-id is a non-canonical key, so index the map directly.
	assert.NotEmpty(t, h["x-statsig-id"])

	// Dynamic statsig: verify supplied id is used verbatim.
	h2 := buildHeaders(longToken, opts, "fixed-statsig")
	assert.Equal(t, "fixed-statsig", h2["x-statsig-id"][0])
}

func TestGenStatsigIDDecodesToTypeErrorShapes(t *testing.T) {
	sawChildren := false
	sawUndefined := false
	for i := 0; i < 100; i++ {
		decoded, err := base64.StdEncoding.DecodeString(genStatsigID())
		if err != nil {
			t.Fatalf("genStatsigID() not base64: %v", err)
		}
		msg := string(decoded)
		switch {
		case strings.Contains(msg, "children['"):
			sawChildren = true
		case strings.Contains(msg, "of undefined"):
			sawUndefined = true
		}
	}
	assert.True(t, sawChildren, "expected children variant to appear")
	assert.True(t, sawUndefined, "expected undefined variant to appear")
}
