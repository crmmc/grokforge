package openai

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/crmmc/grokforge/internal/flow"
)

const assetsGrokBaseURL = "https://assets.grok.com/"

var markdownImageRe = regexp.MustCompile(`!\[([^\]]*)\]\(([^)\s]+)([^)]*)\)`)

// mediaRewriter rewrites downloadable Grok media targets in markdown images.
type mediaRewriter struct {
	download flow.DownloadFunc
}

// newMediaRewriter creates a rewriter. Returns nil if dl is nil.
func newMediaRewriter(dl flow.DownloadFunc) *mediaRewriter {
	if dl == nil {
		return nil
	}
	return &mediaRewriter{download: dl}
}

// rewriteContent is a nil-safe helper that passes content through when rewriter is nil.
func rewriteContent(m *mediaRewriter, ctx context.Context, content string) (string, error) {
	if m == nil {
		return content, nil
	}
	return m.Rewrite(ctx, content)
}

// Rewrite processes content, downloading and rewriting eligible markdown images.
func (m *mediaRewriter) Rewrite(ctx context.Context, content string) (string, error) {
	if m == nil || content == "" {
		return content, nil
	}
	matches := markdownImageRe.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return content, nil
	}

	var b strings.Builder
	last := 0
	for _, match := range matches {
		b.WriteString(content[last:match[0]])
		full := content[match[0]:match[1]]
		alt := content[match[2]:match[3]]
		target := content[match[4]:match[5]]
		downloadURL, ok := mediaDownloadURL(target)
		if !ok {
			b.WriteString(full)
			last = match[1]
			continue
		}
		rendered, err := m.renderImage(ctx, downloadURL, alt)
		if err != nil {
			return "", fmt.Errorf("rewrite markdown image %q: %w", target, err)
		}
		b.WriteString(rendered)
		last = match[1]
	}
	b.WriteString(content[last:])
	return b.String(), nil
}

func (m *mediaRewriter) renderImage(ctx context.Context, rawURL, alt string) (string, error) {
	data, err := m.download(ctx, rawURL)
	if err != nil {
		return "", fmt.Errorf("download for base64: %w", err)
	}
	mime := http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/") {
		return "", fmt.Errorf("downloaded media is not an image: %s", mime)
	}
	dataURI := fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(data))
	return fmt.Sprintf("![%s](%s)", alt, dataURI), nil
}

func mediaDownloadURL(target string) (string, bool) {
	trimmed := strings.TrimSpace(target)
	if isRelativeGeneratedAsset(trimmed) {
		return assetsGrokBaseURL + strings.TrimLeft(trimmed, "/"), true
	}

	parsed, err := parseMediaTargetURL(trimmed)
	if err != nil || !isHTTPURL(parsed) {
		return "", false
	}
	host := strings.ToLower(parsed.Hostname())
	path := strings.ToLower(parsed.EscapedPath())
	if host == "assets.grok.com" && isAssetsMediaPath(path) {
		return canonicalMediaURL(parsed), true
	}
	if host == "grok.com" && isGrokImagePath(path) {
		return canonicalMediaURL(parsed), true
	}
	return "", false
}

func parseMediaTargetURL(target string) (*url.URL, error) {
	if strings.HasPrefix(target, "//") {
		return url.Parse("https:" + target)
	}
	return url.Parse(target)
}

func isRelativeGeneratedAsset(target string) bool {
	path := strings.ToLower(strings.TrimLeft(target, "/"))
	return strings.HasPrefix(path, "users/") && isGeneratedMediaPath(path)
}

func isHTTPURL(parsed *url.URL) bool {
	scheme := strings.ToLower(parsed.Scheme)
	return scheme == "http" || scheme == "https"
}

func canonicalMediaURL(parsed *url.URL) string {
	copied := *parsed
	copied.Scheme = "https"
	return copied.String()
}

func isGeneratedMediaPath(path string) bool {
	return strings.Contains(path, "/generated/")
}

func isAssetsMediaPath(path string) bool {
	return strings.HasPrefix(path, "/users/") ||
		strings.HasPrefix(path, "/cards/") ||
		isGeneratedMediaPath(path)
}

func isGrokImagePath(path string) bool {
	return isGeneratedMediaPath(path) ||
		strings.HasPrefix(path, "/img/") ||
		strings.HasPrefix(path, "/images/")
}
