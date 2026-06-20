package grok

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/crmmc/grokforge/internal/upstream"
)

const (
	assetsBaseURL        = "https://assets.grok.com/"
	maxAssetDownloadSize = 200 << 20
)

func (g *GrokUpstream) downloadFunc(token string) upstream.DownloadFunc {
	return func(ctx context.Context, rawURL string) ([]byte, error) {
		return g.downloadURL(ctx, token, rawURL)
	}
}

func (g *GrokUpstream) downloadURL(ctx context.Context, token, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, normalizeAssetURL(rawURL), nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header = g.buildHeaders(token)

	resp, err := g.doer.Do(req)
	if err != nil {
		return nil, g.mapError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, g.mapHTTPError(resp)
	}

	var buf bytes.Buffer
	written, err := io.Copy(&buf, io.LimitReader(resp.Body, maxAssetDownloadSize+1))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if written > maxAssetDownloadSize {
		return nil, fmt.Errorf("asset body exceeds %d bytes", maxAssetDownloadSize)
	}
	return buf.Bytes(), nil
}

func normalizeAssetURL(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if strings.HasPrefix(trimmed, "//") {
		return canonicalAssetURL("https:" + trimmed)
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return canonicalAssetURL(trimmed)
	}
	return assetsBaseURL + strings.TrimPrefix(trimmed, "/")
}

func canonicalAssetURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return rawURL
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "assets.grok.com" && host != "grok.com" {
		return rawURL
	}
	copied := *parsed
	copied.Scheme = "https"
	return copied.String()
}
