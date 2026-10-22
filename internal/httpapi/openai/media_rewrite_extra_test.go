package openai

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMediaRewriter_NilDownload(t *testing.T) {
	assert.Nil(t, newMediaRewriter(nil, nil, "base64", nil))
}

func TestNewMediaRewriter_FormatNormalization(t *testing.T) {
	tests := []struct {
		name       string
		format     string
		wantFormat string
	}{
		{name: "empty defaults to base64", format: "", wantFormat: "base64"},
		{name: "padded local url", format: "  LOCAL_URL ", wantFormat: "local_url"},
		{name: "base64 exact", format: "base64", wantFormat: "base64"},
		{name: "unknown kept for error path", format: "weird", wantFormat: "weird"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rewriter := newMediaRewriter(pngDownload, nil, tc.format, nil)
			require.NotNil(t, rewriter)
			assert.Equal(t, tc.wantFormat, rewriter.format)
		})
	}
}

func TestMediaRewriter_Rewrite_EmptyAndNoImages(t *testing.T) {
	rewriter := newMediaRewriter(pngDownload, nil, "base64", nil)

	got, err := rewriter.Rewrite(context.Background(), "")
	require.Nil(t, err)
	assert.Equal(t, "", got)

	got, err = rewriter.Rewrite(context.Background(), "no images in this text")
	require.Nil(t, err)
	assert.Equal(t, "no images in this text", got)
}

func TestMediaRewriter_Rewrite_UnsupportedFormat(t *testing.T) {
	rewriter := newMediaRewriter(pngDownload, nil, "weird", nil)
	_, err := rewriter.Rewrite(context.Background(), "![img](users/u/generated/a.png)")
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "unsupported format")
}

func TestMediaRewriter_Rewrite_NonImageDataRejected(t *testing.T) {
	rewriter := newMediaRewriter(func(context.Context, string) ([]byte, error) {
		return []byte("plain text body"), nil
	}, nil, "base64", nil)
	_, err := rewriter.Rewrite(context.Background(), "![img](users/u/generated/a.png)")
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "not an image")
}

func TestMediaRewriter_Rewrite_LocalURL(t *testing.T) {
	cacheSvc := cache.NewService(t.TempDir(), nil)
	rewriter := newMediaRewriter(pngDownload, cacheSvc, "local_url", func(name string) string {
		return "https://files.example.test/api/files/image/" + name
	})
	got, err := rewriter.Rewrite(context.Background(), "![img](users/u/generated/a.png)")
	require.Nil(t, err)
	assert.Contains(t, got, "https://files.example.test/api/files/image/")
}

func TestMediaRewriter_Rewrite_MixedContent(t *testing.T) {
	rewriter := newMediaRewriter(pngDownload, nil, "base64", nil)
	content := "before ![a](users/u/generated/a.png) middle ![b](https://example.com/b.png) after ![c](data:image/png;base64,QUJD)"
	got, err := rewriter.Rewrite(context.Background(), content)
	require.Nil(t, err)
	assert.True(t, strings.HasPrefix(got, "before ![a](data:image/"))
	assert.Contains(t, got, "![b](https://example.com/b.png)")
	assert.Contains(t, got, "![c](data:image/png;base64,QUJD)")
}

func TestDetectRewriteImageExt(t *testing.T) {
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10}
	gif := []byte("GIF89a....")
	webp := []byte("RIFF\x14\x00\x00\x00WEBPVP8 ")
	png := testPNGBytes()

	assert.Equal(t, ".jpg", detectRewriteImageExt(jpeg))
	assert.Equal(t, ".gif", detectRewriteImageExt(gif))
	assert.Equal(t, ".webp", detectRewriteImageExt(webp))
	assert.Equal(t, ".png", detectRewriteImageExt(png))
	assert.Equal(t, ".png", detectRewriteImageExt([]byte("unknown content")))
}

func TestHasRelativeImagePathBoundary(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{name: "start of content", content: "users/u/generated/x", want: true},
		{name: "after colon", content: "src:users/u/generated/x", want: true},
		{name: "after space", content: "see users/u/generated/x", want: true},
		{name: "after quote", content: `href="users/u/generated/x"`, want: true},
		{name: "mid word", content: "xusers/u/generated/x", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// The marker "users/" occurrence must exist for boundary checks to matter.
			assert.True(t, strings.Contains(tc.content, "users/"))
			lower := strings.ToLower(tc.content)
			start := strings.Index(lower, "users/")
			assert.Equal(t, tc.want, hasRelativeImagePathBoundary(tc.content, start, "users/"))
		})
	}
}

func TestContainsRelativeGrokImagePath(t *testing.T) {
	tests := []struct {
		name    string
		content string
		spanned bool
		want    bool
	}{
		{
			name:    "relative generated path",
			content: "prefix:users/u/generated/img.png",
			want:    true,
		},
		{
			name:    "plain words not matched",
			content: "the users of the system are happy",
			want:    false,
		},
		{
			name:    "candidate inside provided spans skipped",
			content: "see users/u/generated/x.png",
			spanned: true,
			want:    false,
		},
		{
			name:    "same candidate without spans is found",
			content: "see users/u/generated/x.png",
			want:    true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var spans [][]int
			if tc.spanned {
				spans = [][]int{{0, len(tc.content)}}
			}
			assert.Equal(t, tc.want, containsRelativeGrokImagePath(tc.content, spans))
		})
	}
}

func TestContainsGrokImageReference_MediaGateCases(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    bool
	}{
		{name: "clean text", content: "nothing suspicious", want: false},
		{name: "grok host markdown", content: "![a](https://assets.grok.com/users/u/g.png)", want: true},
		{name: "grok.com img path", content: "see https://grok.com/img/abc/def.png", want: true},
		{name: "imagine public host", content: "see https://imagine-public.grok.com/anything", want: true},
		{name: "subdomain host without media path", content: "see https://blog.grok.com/posts/1", want: false},
		{name: "relative asset path", content: "path /users/u/generated/x.png here", want: true},
		{name: "non grok url", content: "see https://example.com/users/u/g.png", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, containsGrokImageReference(tc.content))
		})
	}
}

func TestRenderLocalImage_CacheSaveError(t *testing.T) {
	base := t.TempDir()
	cacheDir := filepath.Join(base, "cache")
	// Service writes into <dataDir>/tmp/image; pre-create and lock it down so the
	// subsequent save fails.
	imageDir := filepath.Join(cacheDir, "tmp", "image")
	require.NoError(t, os.MkdirAll(imageDir, 0755))
	require.NoError(t, os.Chmod(imageDir, 0500))
	t.Cleanup(func() { _ = os.Chmod(imageDir, 0755) })

	cacheSvc := cache.NewService(cacheDir, nil)
	rewriter := newMediaRewriter(pngDownload, cacheSvc, "local_url", func(string) string { return "unused" })

	_, err := rewriteContent(rewriter, context.Background(), "![img](users/u/generated/a.png)")
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "cache save")
}
