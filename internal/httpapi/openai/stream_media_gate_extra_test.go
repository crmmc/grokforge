package openai

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingDownload is a download func that always errors.
func failingDownload(context.Context, string) ([]byte, error) {
	return nil, errors.New("download unavailable")
}

// pngDownload returns valid PNG bytes for any URL.
func pngDownload(context.Context, string) ([]byte, error) {
	return testPNGBytes(), nil
}

func TestStreamMediaGate_PushEmpty(t *testing.T) {
	g := &streamMediaGate{}
	out, err := g.push(context.Background(), nil, "")
	require.Nil(t, err)
	assert.Equal(t, "", out)
}

func TestStreamMediaGate_PushPlainTextFlushedImmediately(t *testing.T) {
	g := &streamMediaGate{}
	out, err := g.push(context.Background(), nil, "hello world")
	require.Nil(t, err)
	assert.Equal(t, "hello world", out)
	assert.Equal(t, "", g.pending)
}

func TestStreamMediaGate_PushHoldsIncompleteMarkdown(t *testing.T) {
	g := &streamMediaGate{}
	out, err := g.push(context.Background(), nil, "see ![img")
	require.Nil(t, err)
	assert.Equal(t, "see ", out)
	assert.Equal(t, "![img", g.pending)

	out, err = g.push(context.Background(), nil, "](https://example.com/a.png)")
	require.Nil(t, err)
	assert.Equal(t, "![img](https://example.com/a.png)", out)
	assert.Equal(t, "", g.pending)
}

func TestStreamMediaGate_PushErrorPropagates(t *testing.T) {
	rewriter := newMediaRewriter(failingDownload, nil, "base64", nil)
	g := &streamMediaGate{}
	_, err := g.push(context.Background(), rewriter, "![img](https://assets.grok.com/users/u/g.png)")
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "download unavailable")
}

func TestStreamMediaGate_FlushEmpty(t *testing.T) {
	g := &streamMediaGate{}
	out, err := g.flush(context.Background(), nil)
	require.Nil(t, err)
	assert.Equal(t, "", out)
}

func TestStreamMediaGate_FlushEmitsPending(t *testing.T) {
	g := &streamMediaGate{}
	_, err := g.push(context.Background(), nil, "trailing ![img")
	require.Nil(t, err)

	out, err := g.flush(context.Background(), nil)
	require.Nil(t, err)
	assert.Equal(t, "![img", out)
	assert.Equal(t, "", g.pending)
}

func TestStreamMediaGate_CompletingChunkRewritesGrokImage(t *testing.T) {
	rewriter := newMediaRewriter(pngDownload, nil, "base64", nil)
	g := &streamMediaGate{}
	// The incomplete markdown image is held by push...
	out, err := g.push(context.Background(), rewriter, "![img](https://assets.grok.com/users/u/generated/g.png")
	require.Nil(t, err)
	assert.Equal(t, "", out)

	// ...then the closing parenthesis completes it and push rewrites in place.
	out, err = g.push(context.Background(), rewriter, ")")
	require.Nil(t, err)
	assert.Contains(t, out, "data:image/png;base64,")
	assert.NotContains(t, out, "assets.grok.com")
	assert.Equal(t, "", g.pending)
}

func TestStreamMediaGate_FlushErrorPropagates(t *testing.T) {
	rewriter := newMediaRewriter(pngDownload, nil, "base64", nil)
	g := &streamMediaGate{}
	// Unmatched markdown with a Grok media URL: Rewrite leaves it intact but the
	// leak detection inside flush fails the flush.
	_, err := g.push(context.Background(), rewriter, "![img](https://assets.grok.com/users/u/generated/g.png")
	require.Nil(t, err)

	_, err = g.flush(context.Background(), rewriter)
	require.NotNil(t, err)
}

func TestStreamMediaGate_RewriteReplacesPending(t *testing.T) {
	rewriter := newMediaRewriter(pngDownload, nil, "base64", nil)
	g := &streamMediaGate{pending: "![img](https://assets.grok.com/users/u/g.png)"}
	require.Nil(t, g.rewrite(context.Background(), rewriter))
	assert.Contains(t, g.pending, "data:image/png;base64,")
}

func TestStreamingSafeFlushIndex(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{name: "plain text", text: "hello", want: 5},
		{name: "incomplete markdown", text: "see ![img", want: 4},
		{name: "trailing bang", text: "wow!", want: 3},
		{name: "complete markdown", text: "![a](b) tail", want: 12},
		{name: "trailing https prefix", text: "go to https:", want: 6},
		{name: "trailing users slash", text: "look at users/", want: 8},
		{name: "active absolute url", text: "see https://example.com/a.png", want: 4},
		{name: "active relative path", text: "users/u/generated/x/content", want: 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, streamingSafeFlushIndex(tc.text))
		})
	}
}

func TestMarkdownImageHoldStart(t *testing.T) {
	assert.Equal(t, 4, markdownImageHoldStart("see ![img"))
	assert.Equal(t, -1, markdownImageHoldStart("no images here"))
	assert.Equal(t, 3, markdownImageHoldStart("abc!"))
}

func TestIncompleteMarkdownImageStart(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{name: "no image", text: "plain text", want: -1},
		{name: "unclosed alt", text: "abc ![img", want: 4},
		{name: "closed alt no paren", text: "![img] tail", want: -1},
		{name: "alt at end", text: "![img]", want: 0},
		{name: "unclosed url", text: "![img](https://x", want: 0},
		{name: "complete", text: "![img](https://x) tail", want: -1},
		{name: "second incomplete", text: "![a](b) ok then ![c", want: 16},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, incompleteMarkdownImageStart(tc.text))
		})
	}
}

func TestActiveDelimitedStart(t *testing.T) {
	t.Run("absolute url held at end", func(t *testing.T) {
		assert.Equal(t, 4, activeAbsoluteURLStart("see https://example.com/a.png"))
	})

	t.Run("relative path markers", func(t *testing.T) {
		// "users/" occurrence at index 1 is held, then "/users/" at 0 wins (smaller).
		assert.Equal(t, 0, activeRelativePathStart("/users/x/content"))
	})

	t.Run("later occurrence does not move hold", func(t *testing.T) {
		// Two complete active URLs: the first (earliest) wins.
		assert.Equal(t, 0, activeAbsoluteURLStart("https://a.com/xhttps://b.com/y"))
	})
}

func TestPartialSuffixStart(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		marker string
		want   int
	}{
		{name: "full prefix match", text: "http", marker: "https://", want: 0},
		{name: "partial prefix match", text: "see ht", marker: "https://", want: 4},
		{name: "no match", text: "hello", marker: "https://", want: -1},
		{name: "shorter than marker", text: "h", marker: "https://", want: 0},
		{name: "longer than marker", text: "xhttps", marker: "https://", want: 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, partialSuffixStart(tc.text, tc.marker))
		})
	}
}

func TestMinNonNegative(t *testing.T) {
	assert.Equal(t, 5, minNonNegative(-1, 5))
	assert.Equal(t, 2, minNonNegative(5, 2))
	assert.Equal(t, 5, minNonNegative(5, 7))
}

func TestHasDelimiter(t *testing.T) {
	assert.True(t, hasDelimiter("a b"))
	assert.True(t, hasDelimiter(strings.Repeat("x", 3)+"\n"))
	assert.False(t, hasDelimiter("nodelimiter"))
}
