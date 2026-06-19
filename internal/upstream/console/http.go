package console

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"math/rand"
	"regexp"
	"strings"

	fhttp "github.com/bogdanfinn/fhttp"
	"github.com/google/uuid"
)

var cfClearanceRE = regexp.MustCompile(`(^|;\s*)cf_clearance=[^;]*`)

var reDigits = regexp.MustCompile(`\d{2,3}`)
var reChromeVer = regexp.MustCompile(`(?:Chrome|Chromium|Edg)/(\d+)`)

func extractMajorVersion(browser string) string {
	if browser == "" {
		return ""
	}
	return reDigits.FindString(browser)
}

func extractMajorVersionFromUA(ua string) string {
	m := reChromeVer.FindStringSubmatch(ua)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

type clientHints struct {
	SecChUa         string
	SecChUaMobile   string
	SecChUaPlatform string
	SecChUaArch     string
	SecChUaBitness  string
	SecChUaModel    string
}

func buildClientHints(browser, userAgent string) clientHints {
	browserLower := strings.ToLower(browser)
	ua := strings.ToLower(userAgent)

	isChromium := containsAny(browserLower, "chrome", "chromium", "edge", "brave") ||
		containsAny(ua, "chrome", "chromium", "edg")
	isFirefox := strings.Contains(ua, "firefox") || strings.Contains(browserLower, "firefox")
	isSafari := (strings.Contains(ua, "safari") && !strings.Contains(ua, "chrome") &&
		!strings.Contains(ua, "chromium") && !strings.Contains(ua, "edg")) ||
		strings.Contains(browserLower, "safari")

	if !isChromium || isFirefox || isSafari {
		return clientHints{}
	}

	version := extractMajorVersion(browser)
	if version == "" {
		version = extractMajorVersionFromUA(userAgent)
	}
	if version == "" {
		version = "146"
	}

	secChUa := fmt.Sprintf(`"Google Chrome";v="%s", "Chromium";v="%s", "Not(A:Brand";v="24"`, version, version)

	var platform string
	switch {
	case strings.Contains(ua, "windows"):
		platform = `"Windows"`
	case strings.Contains(ua, "android"):
		platform = `"Android"`
	case strings.Contains(ua, "linux"):
		platform = `"Linux"`
	default:
		platform = `"macOS"`
	}

	var arch, bitness string
	switch {
	case strings.Contains(ua, "aarch64") || strings.Contains(ua, "arm"):
		arch = "arm"
		bitness = "64"
	case strings.Contains(ua, "x86_64") || strings.Contains(ua, "x64") ||
		strings.Contains(ua, "win64") || strings.Contains(ua, "intel"):
		arch = "x86"
		bitness = "64"
	}

	mobile := "?0"
	if strings.Contains(ua, "mobile") || platform == `"Android"` {
		mobile = "?1"
	}

	return clientHints{
		SecChUa:         secChUa,
		SecChUaMobile:   mobile,
		SecChUaPlatform: platform,
		SecChUaArch:     arch,
		SecChUaBitness:  bitness,
		SecChUaModel:    "",
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func BuildCookie(token, cfCookies, cfClearance string) string {
	cookie := "sso=" + token + "; sso-rw=" + token

	extra := strings.TrimSpace(cfCookies)
	if cfClearance != "" {
		if extra == "" {
			extra = "cf_clearance=" + cfClearance
		} else if strings.Contains(extra, "cf_clearance=") {
			extra = cfClearanceRE.ReplaceAllString(extra, "${1}cf_clearance="+cfClearance)
		} else {
			extra = strings.TrimRight(extra, "; ")
			extra += "; cf_clearance=" + cfClearance
		}
	}

	if extra != "" {
		cookie += "; " + extra
	}
	return cookie
}

const staticStatsigID = "ZTpUeXBlRXJyb3I6IENhbm5vdCByZWFkIHByb3BlcnRpZXMgb2YgdW5kZWZpbmVkIChyZWFkaW5nICdjaGlsZE5vZGVzJyk="

func genStatsigID() string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	const alphaNum = "abcdefghijklmnopqrstuvwxyz0123456789"

	var msg string
	if rand.Intn(2) == 0 {
		b := make([]byte, 5)
		for i := range b {
			b[i] = alphaNum[rand.Intn(len(alphaNum))]
		}
		msg = fmt.Sprintf("e:TypeError: Cannot read properties of null (reading 'children['%s']')", string(b))
	} else {
		b := make([]byte, 10)
		for i := range b {
			b[i] = letters[rand.Intn(len(letters))]
		}
		msg = fmt.Sprintf("e:TypeError: Cannot read properties of undefined (reading '%s')", string(b))
	}
	return base64.StdEncoding.EncodeToString([]byte(msg))
}

func (c *ConsoleUpstream) buildHeaders(token string) fhttp.Header {
	statsigID := c.statsigID()
	if statsigID == "" {
		statsigID = genStatsigID()
	}

	hints := buildClientHints(c.browserProfile(), c.userAgent())
	h := fhttp.Header{
		"Accept":           {"*/*"},
		"Accept-Encoding":  {"gzip, deflate, br, zstd"},
		"Accept-Language":  {"zh-CN,zh;q=0.9,en;q=0.8"},
		"Baggage":          {"sentry-environment=production,sentry-release=d6add6fb0460641fd482d767a335ef72b9b6abb8,sentry-public_key=b311e0f2690c81f25e2c4cf6d4f7ce1c"},
		"Content-Type":     {"application/json"},
		"Cookie":           {c.cookieString(token)},
		"Origin":           {"https://console.x.ai"},
		"Priority":         {"u=1, i"},
		"Referer":          {"https://console.x.ai/"},
		"Sec-Fetch-Dest":   {"empty"},
		"Sec-Fetch-Mode":   {"cors"},
		"Sec-Fetch-Site":   {"same-origin"},
		"User-Agent":       {c.userAgent()},
		"X-Statsig-Id":     {statsigID},
		"X-Xai-Request-Id": {uuid.New().String()},
	}

	order := []string{
		"accept", "accept-encoding", "accept-language",
		"baggage", "content-type", "cookie", "origin",
		"priority", "referer",
	}

	if hints.SecChUa != "" {
		h.Set("Sec-Ch-Ua", hints.SecChUa)
		h.Set("Sec-Ch-Ua-Mobile", hints.SecChUaMobile)
		h.Set("Sec-Ch-Ua-Platform", hints.SecChUaPlatform)
		order = append(order, "sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform")

		if hints.SecChUaArch != "" {
			h.Set("Sec-Ch-Ua-Arch", hints.SecChUaArch)
			h.Set("Sec-Ch-Ua-Bitness", hints.SecChUaBitness)
			order = append(order, "sec-ch-ua-arch", "sec-ch-ua-bitness")
		}
		h.Set("Sec-Ch-Ua-Model", hints.SecChUaModel)
		order = append(order, "sec-ch-ua-model")
	}

	order = append(order,
		"sec-fetch-dest", "sec-fetch-mode", "sec-fetch-site",
		"user-agent", "x-statsig-id", "x-xai-request-id",
	)

	if override := c.headerOrder(); len(override) > 0 {
		h[fhttp.HeaderOrderKey] = override
	} else {
		h[fhttp.HeaderOrderKey] = order
	}
	return h
}

func (c *ConsoleUpstream) buildRequest(ctx context.Context, token string, body []byte) (*fhttp.Request, error) {
	req, err := fhttp.NewRequestWithContext(ctx, fhttp.MethodPost, c.baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header = c.buildHeaders(token)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}
