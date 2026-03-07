package xai

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestExtractMajorVersion(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"chrome136", "136"},
		{"chrome_133", "133"},
		{"firefox135", "135"},
		{"", ""},
		{"noversion", ""},
		{"edge120", "120"},
	}
	for _, tt := range tests {
		got := extractMajorVersion(tt.input)
		if got != tt.want {
			t.Errorf("extractMajorVersion(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestBuildClientHints(t *testing.T) {
	// chrome136 browser -> v="136"
	hints := buildClientHints("chrome136", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36")
	if !strings.Contains(hints.SecChUa, `v="136"`) {
		t.Errorf("buildClientHints with chrome136: got %q, want v=\"136\"", hints.SecChUa)
	}
	if !strings.Contains(hints.SecChUa, `"Google Chrome"`) {
		t.Errorf("buildClientHints with chrome136: missing Google Chrome brand, got %q", hints.SecChUa)
	}
	if !strings.Contains(hints.SecChUa, `"Chromium"`) {
		t.Errorf("buildClientHints with chrome136: missing Chromium brand, got %q", hints.SecChUa)
	}
	if hints.SecChUaPlatform != `"macOS"` {
		t.Errorf("buildClientHints platform = %q, want \"macOS\"", hints.SecChUaPlatform)
	}
	// macOS + Intel UA should detect x86 arch
	if hints.SecChUaArch != "x86" {
		t.Errorf("buildClientHints arch = %q, want \"x86\"", hints.SecChUaArch)
	}
	if hints.SecChUaBitness != "64" {
		t.Errorf("buildClientHints bitness = %q, want \"64\"", hints.SecChUaBitness)
	}
	if hints.SecChUaModel != "" {
		t.Errorf("buildClientHints model = %q, want empty", hints.SecChUaModel)
	}

	// empty browser -> fallback to UserAgent Chrome/133
	hints2 := buildClientHints("", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36")
	if !strings.Contains(hints2.SecChUa, `v="133"`) {
		t.Errorf("buildClientHints with empty browser: got %q, want v=\"133\"", hints2.SecChUa)
	}

	// Windows UA
	hints3 := buildClientHints("chrome136", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/136.0.0.0")
	if hints3.SecChUaPlatform != `"Windows"` {
		t.Errorf("buildClientHints Windows platform = %q, want \"Windows\"", hints3.SecChUaPlatform)
	}

	// Linux UA
	hints4 := buildClientHints("chrome136", "Mozilla/5.0 (X11; Linux x86_64) Chrome/136.0.0.0")
	if hints4.SecChUaPlatform != `"Linux"` {
		t.Errorf("buildClientHints Linux platform = %q, want \"Linux\"", hints4.SecChUaPlatform)
	}

	// Firefox — should return empty hints (no Client Hints for non-Chromium)
	hintsFF := buildClientHints("firefox135", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:135.0) Gecko/20100101 Firefox/135.0")
	if hintsFF.SecChUa != "" {
		t.Errorf("buildClientHints Firefox: SecChUa = %q, want empty", hintsFF.SecChUa)
	}
	if hintsFF.SecChUaPlatform != "" {
		t.Errorf("buildClientHints Firefox: SecChUaPlatform = %q, want empty", hintsFF.SecChUaPlatform)
	}
}

func TestSSoCookieWithCFClearance(t *testing.T) {
	// Without cf_clearance
	got := ssoCookie("TOKEN", "", "")
	want := "sso=TOKEN; sso-rw=TOKEN"
	if got != want {
		t.Errorf("ssoCookie(TOKEN, \"\") = %q, want %q", got, want)
	}

	// With cf_clearance
	got2 := ssoCookie("TOKEN", "", "CF_VAL")
	want2 := "sso=TOKEN; sso-rw=TOKEN; cf_clearance=CF_VAL"
	if got2 != want2 {
		t.Errorf("ssoCookie(TOKEN, CF_VAL) = %q, want %q", got2, want2)
	}

	// With cf_cookies + cf_clearance override
	got3 := ssoCookie("TOKEN", "foo=bar; cf_clearance=old", "new")
	want3 := "sso=TOKEN; sso-rw=TOKEN; foo=bar; cf_clearance=new"
	if got3 != want3 {
		t.Errorf("ssoCookie with cf_cookies override = %q, want %q", got3, want3)
	}
}

func TestBuildHeaders_DynamicHints(t *testing.T) {
	opts := &Options{
		UserAgent:   "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36",
		Browser:     "chrome136",
		CFClearance: "test_cf_val",
	}
	headers := buildHeaders("mytoken", opts, "test-statsig")

	// Sec-Ch-Ua should use v="136" (not hardcoded 133)
	secChUa := headers.Get("Sec-Ch-Ua")
	if !strings.Contains(secChUa, `v="136"`) {
		t.Errorf("Sec-Ch-Ua = %q, want v=\"136\"", secChUa)
	}

	// Cookie should include cf_clearance
	cookie := headers.Get("Cookie")
	if !strings.Contains(cookie, "cf_clearance=test_cf_val") {
		t.Errorf("Cookie = %q, want cf_clearance=test_cf_val", cookie)
	}
	if !strings.Contains(cookie, "sso=mytoken") {
		t.Errorf("Cookie = %q, want sso=mytoken", cookie)
	}

	// Without cf_clearance
	opts2 := &Options{
		UserAgent: "Mozilla/5.0 Chrome/136.0.0.0",
		Browser:   "chrome136",
	}
	headers2 := buildHeaders("tok2", opts2, "")
	cookie2 := headers2.Get("Cookie")
	if strings.Contains(cookie2, "cf_clearance") {
		t.Errorf("Cookie without CFClearance should not contain cf_clearance: %q", cookie2)
	}
}

func TestGenStatsigID_SingleQuotes(t *testing.T) {
	// Generate multiple IDs and check the children variant uses single quotes (not double)
	for i := 0; i < 100; i++ {
		id := genStatsigID()
		decoded, err := base64.StdEncoding.DecodeString(id)
		if err != nil {
			t.Fatalf("genStatsigID() returned non-base64: %q", id)
		}
		msg := string(decoded)
		// The children variant should have single quotes, not double single quotes
		if strings.Contains(msg, "children[''") {
			t.Errorf("genStatsigID() produced double single quotes: %q", msg)
		}
		if strings.Contains(msg, "children['") {
			// Correct single quote format
			if !strings.Contains(msg, "']')") {
				t.Errorf("genStatsigID() children variant missing closing single quote: %q", msg)
			}
		}
	}
}

func TestStaticStatsigID(t *testing.T) {
	// Verify the static value is valid base64
	decoded, err := base64.StdEncoding.DecodeString(staticStatsigID)
	if err != nil {
		t.Fatalf("staticStatsigID is not valid base64: %v", err)
	}
	msg := string(decoded)
	if !strings.Contains(msg, "TypeError") {
		t.Errorf("staticStatsigID decoded = %q, expected TypeError message", msg)
	}
	if !strings.Contains(msg, "childNodes") {
		t.Errorf("staticStatsigID decoded = %q, expected childNodes reference", msg)
	}
}
