package openai

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"

	"github.com/crmmc/grokforge/internal/flow"
)

// xaiImagePathRe matches markdown images with xAI relative paths like:
// ![id](users/xxx/generated/xxx/image.jpg)
var xaiImagePathRe = regexp.MustCompile(`!\[([^\]]*)\]\((users/[^\)]+)\)`)

// mediaRewriter rewrites xAI relative image URLs in chat content.
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
func rewriteContent(m *mediaRewriter, ctx context.Context, content string) string {
	if m == nil {
		return content
	}
	return m.Rewrite(ctx, content)
}

// Rewrite processes content, downloading and rewriting any xAI image URLs.
func (m *mediaRewriter) Rewrite(ctx context.Context, content string) string {
	if m == nil || content == "" {
		return content
	}
	if !xaiImagePathRe.MatchString(content) {
		return content
	}

	return xaiImagePathRe.ReplaceAllStringFunc(content, func(match string) string {
		sub := xaiImagePathRe.FindStringSubmatch(match)
		if len(sub) < 3 {
			return match
		}
		imgID := sub[1]
		relPath := sub[2] // e.g. "users/xxx/generated/xxx/image.jpg"

		rendered, err := m.renderImage(ctx, relPath, imgID)
		if err != nil {
			slog.Warn("media_rewrite: failed to process image, keeping original",
				"path", relPath, "error", err)
			return match
		}
		return rendered
	})
}

func (m *mediaRewriter) renderImage(ctx context.Context, relPath, imgID string) (string, error) {
	assetURL := "https://assets.grok.com/" + relPath

	data, err := m.download(ctx, assetURL)
	if err != nil {
		return "", fmt.Errorf("download for base64: %w", err)
	}
	mime := http.DetectContentType(data)
	dataURI := fmt.Sprintf("data:%s;base64,%s", mime, base64.StdEncoding.EncodeToString(data))
	return fmt.Sprintf("![%s](%s)", imgID, dataURI), nil
}
