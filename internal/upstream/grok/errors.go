package grok

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	fhttp "github.com/bogdanfinn/fhttp"
	"github.com/crmmc/grokforge/internal/upstream"
)

func (g *GrokUpstream) mapError(err error) error {
	urlErr := &url.Error{}
	if errors.As(err, &urlErr) {
		return fmt.Errorf("%w: %v", upstream.ErrNetwork, err)
	}
	return err
}

func (g *GrokUpstream) mapHTTPError(resp *fhttp.Response) error {
	contentType := resp.Header.Get("Content-Type")
	switch resp.StatusCode {
	case fhttp.StatusTooManyRequests:
		return upstream.ErrRateLimited
	case fhttp.StatusForbidden:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if isCFChallenge(contentType, string(body)) {
			return upstream.ErrCFChallenge
		}
		return upstream.ErrForbidden
	case fhttp.StatusUnauthorized:
		return upstream.ErrInvalidToken
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 65536))
		return fmt.Errorf("%w: status %d: %s", upstream.ErrServerError, resp.StatusCode, string(body))
	}
}

func isCFChallenge(contentType, body string) bool {
	if strings.Contains(contentType, "text/html") {
		return true
	}
	lower := strings.ToLower(body)
	return strings.Contains(lower, "cf-") || strings.Contains(lower, "cloudflare") ||
		strings.Contains(lower, "challenge-platform") || strings.Contains(lower, "just a moment")
}
