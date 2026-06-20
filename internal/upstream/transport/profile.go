package transport

import (
	"regexp"
	"sort"
	"strings"
)

const (
	// DefaultProfile matches grok2api's default curl_cffi profile.
	DefaultProfile = "chrome136"
)

// SupportedProfiles mirrors the libcurl-impersonate/curl_cffi profiles this
// build is expected to support. Family profiles are kept for grok2api-style
// fallback when a newer FlareSolverr User-Agent is ahead of this binary.
var SupportedProfiles = []string{
	"chrome131", "chrome133a", "chrome136", "chrome142", "chrome145",
	"firefox133", "firefox135", "firefox144", "firefox147",
	"safari180", "safari184", "safari260",
	"chrome", "firefox", "safari",
}

// ProfileUAMap maps exposed browser profiles to paired User-Agent strings.
// The map is the single source of truth for backend validation and frontend UI.
var ProfileUAMap = map[string]string{
	"chrome131":  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"chrome133a": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36",
	"chrome136":  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36",
	"chrome142":  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36",
	"chrome145":  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36",
	"firefox133": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:133.0) Gecko/20100101 Firefox/133.0",
	"firefox135": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:135.0) Gecko/20100101 Firefox/135.0",
	"firefox144": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:144.0) Gecko/20100101 Firefox/144.0",
	"firefox147": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:147.0) Gecko/20100101 Firefox/147.0",
	"safari180":  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15",
	"safari184":  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.4 Safari/605.1.15",
	"safari260":  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.0 Safari/605.1.15",
	"chrome":     "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/136.0.0.0 Safari/537.36",
	"firefox":    "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:135.0) Gecko/20100101 Firefox/135.0",
	"safari":     "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.4 Safari/605.1.15",
}

var (
	familyPrefixRE = regexp.MustCompile(`^[a-z_]+`)
	firefoxUARE    = regexp.MustCompile(`firefox/(\d+)`)
	edgeUARE       = regexp.MustCompile(`edg/(\d+)`)
	chromeUARE     = regexp.MustCompile(`(?:chrome|chromium|crios)/(\d+)`)
)

func DefaultUserAgent() string { return ProfileUAMap[DefaultProfile] }

func SupportedProfileList() []string {
	out := append([]string(nil), SupportedProfiles...)
	sort.Strings(out)
	return out
}

func IsSupportedProfile(candidate string) bool { return ResolveProfile(candidate) != "" }

func UserAgentForProfile(name string) (string, bool) {
	key := normalizeProfileName(name)
	if ua, ok := ProfileUAMap[key]; ok {
		return ua, true
	}
	resolved := ResolveProfile(key)
	if resolved == "" {
		return "", false
	}
	ua, ok := ProfileUAMap[resolved]
	return ua, ok
}

func ResolveProfile(candidate string) string {
	candidate = normalizeProfileName(candidate)
	if candidate == "" {
		return ""
	}
	set := supportedProfileSet()
	if set[candidate] {
		return candidate
	}
	family := familyPrefixRE.FindString(candidate)
	if family != "" && set[family] {
		return family
	}
	return ""
}

func BrowserFromUserAgent(userAgent string) string {
	lower := strings.ToLower(userAgent)
	if m := firefoxUARE.FindStringSubmatch(lower); len(m) >= 2 {
		if resolved := ResolveProfile("firefox" + m[1]); resolved != "" {
			return resolved
		}
		return ResolveProfile("firefox")
	}
	if m := edgeUARE.FindStringSubmatch(lower); len(m) >= 2 {
		if resolved := ResolveProfile("edge" + m[1]); resolved != "" {
			return resolved
		}
		return ResolveProfile("edge")
	}
	if m := chromeUARE.FindStringSubmatch(lower); len(m) >= 2 {
		suffix := ""
		if strings.Contains(lower, "android") {
			suffix = "_android"
		}
		if resolved := ResolveProfile("chrome" + m[1] + suffix); resolved != "" {
			return resolved
		}
		if suffix != "" {
			return ResolveProfile("chrome_android")
		}
		return ResolveProfile("chrome")
	}
	if strings.Contains(lower, "safari/") && !strings.Contains(lower, "chrome/") && !strings.Contains(lower, "chromium/") {
		if strings.Contains(lower, "iphone") || strings.Contains(lower, "ipad") {
			return ResolveProfile("safari_ios")
		}
		return ResolveProfile("safari")
	}
	return ""
}

func EffectiveProfile(browser, userAgent string) string {
	if fromUA := BrowserFromUserAgent(userAgent); fromUA != "" {
		return fromUA
	}
	if resolved := ResolveProfile(browser); resolved != "" {
		return resolved
	}
	return DefaultProfile
}

func normalizeProfileName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, "_", "")
	return name
}

func supportedProfileSet() map[string]bool {
	set := make(map[string]bool, len(SupportedProfiles))
	for _, profile := range SupportedProfiles {
		set[normalizeProfileName(profile)] = true
	}
	return set
}
