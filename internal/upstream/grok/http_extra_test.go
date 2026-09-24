package grok

import (
	"context"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream/transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractMajorVersion(t *testing.T) {
	tests := []struct {
		name    string
		browser string
		want    string
	}{
		{name: "empty", browser: "", want: ""},
		{name: "chrome136", browser: "chrome136", want: "136"},
		{name: "edge99", browser: "edge99", want: "99"},
		{name: "no digits", browser: "safari", want: ""},
		{name: "single digit ignored", browser: "v1", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractMajorVersion(tt.browser))
		})
	}
}

func TestExtractMajorVersionFromUA(t *testing.T) {
	tests := []struct {
		name string
		ua   string
		want string
	}{
		{name: "chrome", ua: "Mozilla/5.0 AppleWebKit/537.36 Chrome/136.0.0.0 Safari/537.36", want: "136"},
		{name: "chromium", ua: "Chromium/120.0 Safari/537.36", want: "120"},
		{name: "edge", ua: "Mozilla/5.0 (Windows NT 10.0) Edg/99.0", want: "99"},
		{name: "firefox no match", ua: "Mozilla/5.0 Gecko/20100101 Firefox/135.0", want: ""},
		{name: "empty", ua: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractMajorVersionFromUA(tt.ua))
		})
	}
}

func TestBuildClientHints(t *testing.T) {
	tests := []struct {
		name           string
		browser        string
		userAgent      string
		wantSecChUa    string
		wantPlatform   string
		wantMobile     string
		wantArch       string
		wantBitness    string
		wantEmptyHints bool
	}{
		{
			name:           "firefox skipped",
			browser:        "firefox135",
			userAgent:      "Mozilla/5.0 Gecko/20100101 Firefox/135.0",
			wantEmptyHints: true,
		},
		{
			name:           "safari via ua skipped",
			browser:        "",
			userAgent:      "Mozilla/5.0 (Macintosh) Version/17.0 Safari/605.1.15",
			wantEmptyHints: true,
		},
		{
			name:           "safari via browser skipped",
			browser:        "safari17",
			userAgent:      "",
			wantEmptyHints: true,
		},
		{
			name:           "unknown browser skipped",
			browser:        "",
			userAgent:      "",
			wantEmptyHints: true,
		},
		{
			name:         "chromium via ua only",
			browser:      "",
			userAgent:    "Mozilla/5.0 (X11; Linux x86_64) Chrome/124.0.0.0",
			wantSecChUa:  `"Google Chrome";v="124", "Chromium";v="124", "Not(A:Brand";v="24"`,
			wantPlatform: `"Linux"`,
			wantMobile:   "?0",
			wantArch:     "x86",
			wantBitness:  "64",
		},
		{
			name:         "windows x64",
			browser:      "chrome136",
			userAgent:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/136.0.0.0",
			wantSecChUa:  `"Google Chrome";v="136", "Chromium";v="136", "Not(A:Brand";v="24"`,
			wantPlatform: `"Windows"`,
			wantMobile:   "?0",
			wantArch:     "x86",
			wantBitness:  "64",
		},
		{
			name:         "android arm mobile",
			browser:      "chrome",
			userAgent:    "Mozilla/5.0 (Linux; Android 14; aarch64) Chrome/135.0.0.0 Mobile",
			wantSecChUa:  `"Google Chrome";v="135", "Chromium";v="135", "Not(A:Brand";v="24"`,
			wantPlatform: `"Android"`,
			wantMobile:   "?1",
			wantArch:     "arm",
			wantBitness:  "64",
		},
		{
			name:         "mac intel default platform",
			browser:      "chromium121",
			userAgent:    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) Chrome/121.0.0.0",
			wantSecChUa:  `"Google Chrome";v="121", "Chromium";v="121", "Not(A:Brand";v="24"`,
			wantPlatform: `"macOS"`,
			wantMobile:   "?0",
			wantArch:     "x86",
			wantBitness:  "64",
		},
		{
			name:         "edge ua version fallback",
			browser:      "brave",
			userAgent:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Edg/99.0.1150.30",
			wantSecChUa:  `"Google Chrome";v="99", "Chromium";v="99", "Not(A:Brand";v="24"`,
			wantPlatform: `"Windows"`,
			wantMobile:   "?0",
			wantArch:     "x86",
			wantBitness:  "64",
		},
		{
			name:         "no version anywhere defaults to 146",
			browser:      "chrome",
			userAgent:    "Mozilla/5.0 (Windows NT 10.0)",
			wantSecChUa:  `"Google Chrome";v="146", "Chromium";v="146", "Not(A:Brand";v="24"`,
			wantPlatform: `"Windows"`,
			wantMobile:   "?0",
			wantArch:     "",
			wantBitness:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hints := buildClientHints(tt.browser, tt.userAgent)
			if tt.wantEmptyHints {
				assert.Empty(t, hints.SecChUa)
				return
			}
			assert.Equal(t, tt.wantSecChUa, hints.SecChUa)
			assert.Equal(t, tt.wantPlatform, hints.SecChUaPlatform)
			assert.Equal(t, tt.wantMobile, hints.SecChUaMobile)
			assert.Equal(t, tt.wantArch, hints.SecChUaArch)
			assert.Equal(t, tt.wantBitness, hints.SecChUaBitness)
			assert.Empty(t, hints.SecChUaModel)
		})
	}
}

func TestBuildCookie(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		cfCookies   string
		cfClearance string
		want        string
	}{
		{
			name:  "token only",
			token: "tok",
			want:  "sso=tok; sso-rw=tok",
		},
		{
			name:        "clearance only",
			token:       "tok",
			cfClearance: "cf1",
			want:        "sso=tok; sso-rw=tok; cf_clearance=cf1",
		},
		{
			name:        "clearance appended to cookies",
			token:       "tok",
			cfCookies:   "a=b; c=d",
			cfClearance: "cf1",
			want:        "sso=tok; sso-rw=tok; a=b; c=d; cf_clearance=cf1",
		},
		{
			name:        "clearance replaces existing",
			token:       "tok",
			cfCookies:   "a=b; cf_clearance=old; c=d",
			cfClearance: "new",
			want:        "sso=tok; sso-rw=tok; a=b; cf_clearance=new; c=d",
		},
		{
			name:        "clearance replaces existing at start",
			token:       "tok",
			cfCookies:   "cf_clearance=old; a=b",
			cfClearance: "new",
			want:        "sso=tok; sso-rw=tok; cf_clearance=new; a=b",
		},
		{
			name:      "cookies without clearance untouched",
			token:     "tok",
			cfCookies: "a=b",
			want:      "sso=tok; sso-rw=tok; a=b",
		},
		{
			name:      "whitespace cookies trimmed",
			token:     "tok",
			cfCookies: "   ",
			want:      "sso=tok; sso-rw=tok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, BuildCookie(tt.token, tt.cfCookies, tt.cfClearance))
		})
	}
}

func TestGrokBuildRequest_InvalidURL(t *testing.T) {
	g := New("http://[", nil, Options{})
	_, err := g.buildRequest(context.Background(), "tok", []byte("{}"))
	require.Error(t, err)
}

func TestGrokBuildHeaders_DefaultOrderWithoutArch(t *testing.T) {
	g := New("", nil, Options{
		BrowserProfile: func() string { return "chrome" },
		UserAgent:      func() string { return "Mozilla/5.0 (Windows NT 10.0) Chrome/136.0.0.0" },
	})
	h := g.buildHeaders("tok")
	require.NotEmpty(t, h.Get("Sec-Ch-Ua"))
	assert.Empty(t, h.Get("Sec-Ch-Ua-Arch"))
	order := h[transport.HeaderOrderKey]
	require.NotEmpty(t, order)
	assert.NotContains(t, order, "sec-ch-ua-arch")
	assert.Contains(t, order, "sec-ch-ua-model")
	assert.True(t, strings.HasPrefix(order[0], "accept"))
	assert.Equal(t, "x-xai-request-id", order[len(order)-1])
}

func TestGrokBuildHeaders_WithArch(t *testing.T) {
	g := New("", nil, Options{
		BrowserProfile: func() string { return "chrome136" },
		UserAgent:      func() string { return "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/136.0.0.0" },
		StatsigID:      func() string { return "fixed-statsig" },
	})
	h := g.buildHeaders("tok")
	assert.Equal(t, "x86", h.Get("Sec-Ch-Ua-Arch"))
	assert.Equal(t, "64", h.Get("Sec-Ch-Ua-Bitness"))
	assert.Equal(t, "fixed-statsig", h.Get("X-Statsig-Id"))
	order := h[transport.HeaderOrderKey]
	assert.Contains(t, order, "sec-ch-ua-arch")
	assert.Contains(t, order, "sec-ch-ua-bitness")
}
