package grok

import (
	"strings"
	"testing"

	fhttp "github.com/bogdanfinn/fhttp"
)

func TestBuildHeaders_Chromium(t *testing.T) {
	g := New("", nil, Options{
		BuildCookieString: func(tok string) string { return "sso=" + tok + "; cf_clearance=xyz" },
		HeaderOrder:       func() []string { return []string{"user-agent", "cookie"} },
		BrowserProfile:    func() string { return "chrome136" },
		UserAgent: func() string {
			return "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36"
		},
		StatsigID: func() string { return "statsig-test" },
	})

	h := g.buildHeaders("tok-abc")

	for _, key := range []string{
		"Accept", "Accept-Encoding", "Accept-Language", "Baggage", "Content-Type", "Cookie", "Origin",
		"Priority", "Referer", "Sec-Ch-Ua", "Sec-Ch-Ua-Mobile", "Sec-Ch-Ua-Platform", "Sec-Fetch-Dest",
		"Sec-Fetch-Mode", "Sec-Fetch-Site", "User-Agent", "x-statsig-id", "x-xai-request-id",
	} {
		if h.Get(key) == "" {
			t.Errorf("missing header %s", key)
		}
	}
	if h.Get("Origin") != "https://grok.com" {
		t.Errorf("Origin = %q", h.Get("Origin"))
	}
	if h.Get("Referer") != "https://grok.com/" {
		t.Errorf("Referer = %q", h.Get("Referer"))
	}
	if !strings.Contains(h.Get("Cookie"), "sso=tok-abc") || !strings.Contains(h.Get("Cookie"), "cf_clearance=xyz") {
		t.Errorf("Cookie missing expected values: %q", h.Get("Cookie"))
	}
	if h.Get("Sec-Ch-Ua") == "" {
		t.Error("expected Sec-Ch-Ua for chromium")
	}
	if got := h[fhttp.HeaderOrderKey]; len(got) != 2 || got[0] != "user-agent" || got[1] != "cookie" {
		t.Errorf("HeaderOrderKey = %v", got)
	}
}

func TestBuildHeaders_FirefoxSkipsClientHints(t *testing.T) {
	g := New("", nil, Options{
		BuildCookieString: func(tok string) string { return BuildCookie(tok, "", "") },
		BrowserProfile:    func() string { return "firefox135" },
		UserAgent:         func() string { return "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:135.0) Gecko/20100101 Firefox/135.0" },
	})

	h := g.buildHeaders("tok-abc")

	if h.Get("Sec-Ch-Ua") != "" {
		t.Errorf("Sec-Ch-Ua = %q, want empty for firefox", h.Get("Sec-Ch-Ua"))
	}
	if got := h[fhttp.HeaderOrderKey]; len(got) == 0 {
		t.Fatal("HeaderOrderKey is empty")
	}
	if h.Get("Cookie") != "sso=tok-abc; sso-rw=tok-abc" {
		t.Errorf("Cookie = %q", h.Get("Cookie"))
	}
}

