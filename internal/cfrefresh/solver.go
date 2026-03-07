// Package cfrefresh provides automatic Cloudflare clearance refresh via FlareSolverr.
package cfrefresh

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const grokURL = "https://grok.com"

const maxSolverResponseSize = 5 << 20 // 5MB — FlareSolverr HTML responses

// SolveResult holds the extracted values from a successful FlareSolverr solve.
type SolveResult struct {
	Cookies     string // all cookies joined as "name=value; name=value"
	CFClearance string // cf_clearance cookie value
	UserAgent   string // browser user-agent string
	Browser     string // fingerprint like "chrome133"
}

// flaresolverrRequest is the JSON body sent to FlareSolverr /v1.
type flaresolverrRequest struct {
	Cmd        string                  `json:"cmd"`
	URL        string                  `json:"url"`
	MaxTimeout int                     `json:"maxTimeout"`
	Proxy      *flaresolverrProxyField `json:"proxy,omitempty"`
}

type flaresolverrProxyField struct {
	URL string `json:"url"`
}

// flaresolverrResponse is the JSON response from FlareSolverr.
type flaresolverrResponse struct {
	Status   string                `json:"status"`
	Message  string                `json:"message"`
	Solution flaresolverrSolution  `json:"solution"`
}

type flaresolverrSolution struct {
	UserAgent string              `json:"userAgent"`
	Cookies   []flaresolverrCookie `json:"cookies"`
}

type flaresolverrCookie struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// SolveCFChallenge calls FlareSolverr to solve the Cloudflare challenge on grok.com.
// flaresolverrURL is the base URL of the FlareSolverr service (e.g. "http://localhost:8191").
// timeout is the CF challenge timeout in seconds. proxyURL is optional.
func SolveCFChallenge(flaresolverrURL string, timeout int, proxyURL string) (*SolveResult, error) {
	reqBody := flaresolverrRequest{
		Cmd:        "request.get",
		URL:        grokURL,
		MaxTimeout: timeout * 1000,
	}
	if proxyURL != "" {
		reqBody.Proxy = &flaresolverrProxyField{URL: proxyURL}
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	url := strings.TrimRight(flaresolverrURL, "/") + "/v1"
	httpTimeout := time.Duration(timeout+30) * time.Second

	client := &http.Client{Timeout: httpTimeout}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("POST %s: %w", url, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxSolverResponseSize))
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		preview := string(respBody)
		if len(preview) > 300 {
			preview = preview[:300]
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, preview)
	}

	var result flaresolverrResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if result.Status != "ok" {
		return nil, fmt.Errorf("FlareSolverr error: %s - %s", result.Status, result.Message)
	}

	cookies := result.Solution.Cookies
	if len(cookies) == 0 {
		return nil, fmt.Errorf("FlareSolverr returned no cookies")
	}

	return &SolveResult{
		Cookies:     extractAllCookies(cookies),
		CFClearance: extractCookieValue(cookies, "cf_clearance"),
		UserAgent:   result.Solution.UserAgent,
		Browser:     extractBrowserProfile(result.Solution.UserAgent),
	}, nil
}

// extractAllCookies joins cookie list into "name=value; name=value" format.
func extractAllCookies(cookies []flaresolverrCookie) string {
	parts := make([]string, len(cookies))
	for i, c := range cookies {
		parts[i] = c.Name + "=" + c.Value
	}
	return strings.Join(parts, "; ")
}

// extractCookieValue finds a cookie by name and returns its value.
func extractCookieValue(cookies []flaresolverrCookie, name string) string {
	for _, c := range cookies {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

var chromeVersionRe = regexp.MustCompile(`Chrome/(\d+)`)

// extractBrowserProfile extracts "chromeNNN" from a user-agent string.
func extractBrowserProfile(userAgent string) string {
	m := chromeVersionRe.FindStringSubmatch(userAgent)
	if len(m) >= 2 {
		return "chrome" + m[1]
	}
	return "chrome120"
}
