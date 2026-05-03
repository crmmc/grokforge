package openai

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/crmmc/grokforge/internal/cache"
	"github.com/crmmc/grokforge/internal/flow"
)

const assetsGrokBaseURL = "https://assets.grok.com/"
const grokImagePathPrefix = "img/"
const mediaRewriteFormatBase64 = "base64"
const mediaRewriteFormatLocalURL = "local_url"

var markdownImageRe = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]+)\)`)
var absoluteURLRe = regexp.MustCompile(`https?://[^'"\s)<>]+`)

// mediaRewriter rewrites Grok image URLs in chat content.
type mediaRewriter struct {
	download     flow.DownloadFunc
	cacheSvc     *cache.Service
	format       string
	localURLFunc func(filename string) string
}

// newMediaRewriter creates a rewriter. Returns nil if dl is nil.
func newMediaRewriter(
	dl flow.DownloadFunc,
	cacheSvc *cache.Service,
	format string,
	localURLFunc func(filename string) string,
) *mediaRewriter {
	if dl == nil {
		return nil
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = mediaRewriteFormatBase64
	}
	return &mediaRewriter{
		download:     dl,
		cacheSvc:     cacheSvc,
		format:       format,
		localURLFunc: localURLFunc,
	}
}

// rewriteContent is a nil-safe helper that passes content through when rewriter is nil.
func rewriteContent(m *mediaRewriter, ctx context.Context, content string) (string, error) {
	if m == nil {
		return content, nil
	}
	rewritten, err := m.Rewrite(ctx, content)
	if err != nil {
		return "", err
	}
	// Leak detection: ensure no Grok image references remain
	if containsGrokImageReference(rewritten) {
		return "", fmt.Errorf("media rewrite: grok image reference remains")
	}
	return rewritten, nil
}

// Rewrite processes content, downloading and rewriting any Grok image URLs.
func (m *mediaRewriter) Rewrite(ctx context.Context, content string) (string, error) {
	if content == "" {
		return content, nil
	}
	matches := markdownImageRe.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return content, nil
	}

	var b strings.Builder
	last := 0
	for _, loc := range matches {
		b.WriteString(content[last:loc[0]])
		alt := content[loc[2]:loc[3]]
		target := content[loc[4]:loc[5]]

		if !isGrokImageTarget(target) {
			b.WriteString(content[loc[0]:loc[1]])
			last = loc[1]
			continue
		}

		downloadURL := target
		if isRelativeImageTarget(target) {
			rel := strings.TrimPrefix(strings.TrimSpace(target), "/")
			downloadURL = assetsGrokBaseURL + rel
		}

		rewritten, err := m.renderImage(ctx, alt, downloadURL)
		if err != nil {
			return "", fmt.Errorf("rewrite image %q: %w", target, err)
		}
		b.WriteString(rewritten)
		last = loc[1]
	}
	b.WriteString(content[last:])
	return b.String(), nil
}

func (m *mediaRewriter) renderImage(ctx context.Context, alt, assetURL string) (string, error) {
	data, err := m.download(ctx, assetURL)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	mime := http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/") {
		return "", fmt.Errorf("downloaded media is not an image: %s", mime)
	}
	if m.format == mediaRewriteFormatLocalURL {
		return m.renderLocalImage(alt, data)
	}
	if m.format != mediaRewriteFormatBase64 {
		return "", fmt.Errorf("media rewrite: unsupported format %q", m.format)
	}
	dataURI := fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(data))
	return fmt.Sprintf("![%s](%s)", alt, dataURI), nil
}

func (m *mediaRewriter) renderLocalImage(alt string, data []byte) (string, error) {
	if m.cacheSvc == nil {
		return "", fmt.Errorf("media rewrite: local_url requires cache service")
	}
	if m.localURLFunc == nil {
		return "", fmt.Errorf("media rewrite: local_url requires local URL builder")
	}
	name, err := m.cacheSvc.SaveFile("image", data, detectRewriteImageExt(data))
	if err != nil {
		return "", fmt.Errorf("cache save: %w", err)
	}
	return fmt.Sprintf("![%s](%s)", alt, m.localURLFunc(name)), nil
}

// isGrokImageTarget checks if a markdown image target should be rewritten.
func isGrokImageTarget(target string) bool {
	target = strings.TrimSpace(target)
	if strings.HasPrefix(target, "data:") || strings.HasPrefix(target, "/api/files/") {
		return false
	}
	return isRelativeImageTarget(target) || isGrokImageHostByTarget(target)
}

// isAllowedGrokImageHost checks host against strict Grok domain whitelist.
func isAllowedGrokImageHost(host string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	return host == "assets.grok.com" ||
		host == "grok.com" ||
		strings.HasSuffix(host, ".grok.com")
}

func isGrokImageHostByTarget(target string) bool {
	u, err := url.Parse(strings.TrimSpace(target))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	return isAllowedGrokImageHost(u.Hostname())
}

func isRelativeImageTarget(target string) bool {
	t := strings.TrimPrefix(strings.TrimSpace(target), "/")
	if !strings.HasPrefix(t, "users/") {
		return false
	}
	return strings.Contains(t, "/generated/") ||
		strings.HasSuffix(t, "/content") ||
		strings.Contains(t, "/content?")
}

// containsGrokImageReference checks if content still contains Grok image references.
func containsGrokImageReference(content string) bool {
	// 1. markdown image target host
	for _, loc := range markdownImageRe.FindAllStringSubmatchIndex(content, -1) {
		target := content[loc[4]:loc[5]]
		if isRelativeImageTarget(target) || isGrokImageHostByTarget(target) {
			return true
		}
	}
	// 2. plain-text absolute URL with media resource path
	urlSpans := absoluteURLRe.FindAllStringIndex(content, -1)
	for _, span := range urlSpans {
		raw := content[span[0]:span[1]]
		if isGrokMediaURLTarget(raw) {
			return true
		}
	}
	// 3. plain-text relative asset paths
	if containsRelativeGrokImagePath(content, urlSpans) {
		return true
	}
	return false
}

func containsRelativeGrokImagePath(content string, absoluteURLSpans [][]int) bool {
	lower := strings.ToLower(content)
	for _, marker := range []string{"users/", "/users/"} {
		for offset := 0; offset < len(lower); {
			found := strings.Index(lower[offset:], marker)
			if found < 0 {
				break
			}
			start := offset + found
			offset = start + 1
			if indexInSpans(start, absoluteURLSpans) {
				continue
			}
			if !hasRelativeImagePathBoundary(content, start, marker) {
				continue
			}
			candidate := readRelativePathCandidate(content, start)
			if isRelativeImageTarget(strings.ToLower(candidate)) {
				return true
			}
		}
	}
	return false
}

func indexInSpans(index int, spans [][]int) bool {
	for _, span := range spans {
		if index >= span[0] && index < span[1] {
			return true
		}
	}
	return false
}

func hasRelativeImagePathBoundary(content string, start int, marker string) bool {
	if start == 0 {
		return true
	}
	prev := content[start-1]
	if marker == "users/" && prev == ':' {
		return true
	}
	return strings.ContainsRune(" \n\r\t([{\"'<", rune(prev))
}

func readRelativePathCandidate(content string, start int) string {
	end := start
	for end < len(content) && !strings.ContainsRune("'\" )<>\n\r\t", rune(content[end])) {
		end++
	}
	return content[start:end]
}

// isGrokMediaURLTarget checks if a plain-text URL points to a Grok media resource.
func isGrokMediaURLTarget(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if !isAllowedGrokImageHost(host) {
		return false
	}
	// imagine-public.grok.com is a media-only subdomain
	if host == "imagine-public.grok.com" {
		return true
	}
	// Remaining Grok hosts require a media resource path pattern.
	path := strings.TrimPrefix(u.Path, "/")
	return isRelativeImageTarget(path) || isGrokImagePath(host, path)
}

func isGrokImagePath(host, path string) bool {
	return host == "grok.com" && strings.HasPrefix(path, grokImagePathPrefix)
}

func detectRewriteImageExt(data []byte) string {
	switch http.DetectContentType(data) {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}
