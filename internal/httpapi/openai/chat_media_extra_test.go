package openai

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractChatPromptAndImages(t *testing.T) {
	pngB64 := base64.StdEncoding.EncodeToString(testPNGBytes())
	ctx := context.Background()

	t.Run("last string content wins", func(t *testing.T) {
		prompt, images, err := extractChatPromptAndImages(ctx, []ChatMessage{
			{Role: "user", Content: "first"},
			{Role: "assistant", Content: "second"},
			{Role: "user", Content: "  third  "},
		})
		require.Nil(t, err)
		assert.Equal(t, "third", prompt)
		assert.Empty(t, images)
	})

	t.Run("text blocks update prompt", func(t *testing.T) {
		prompt, _, err := extractChatPromptAndImages(ctx, []ChatMessage{{
			Role: "user",
			Content: []any{
				map[string]any{"type": "text", "text": "describe "},
				map[string]any{"type": "text", "text": "this"},
			},
		}})
		require.Nil(t, err)
		assert.Equal(t, "this", prompt)
	})

	t.Run("unsupported content type skipped", func(t *testing.T) {
		prompt, _, err := extractChatPromptAndImages(ctx, []ChatMessage{
			{Role: "user", Content: "kept"},
			{Role: "user", Content: 12345},
		})
		require.Nil(t, err)
		assert.Equal(t, "kept", prompt)
	})

	t.Run("assistant image ignored", func(t *testing.T) {
		_, images, err := extractChatPromptAndImages(ctx, []ChatMessage{{
			Role: "assistant",
			Content: []any{
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + pngB64}},
			},
		}})
		require.Nil(t, err)
		assert.Empty(t, images)
	})

	t.Run("user image collected", func(t *testing.T) {
		_, images, err := extractChatPromptAndImages(ctx, []ChatMessage{{
			Role: "user",
			Content: []any{
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + pngB64}},
			},
		}})
		require.Nil(t, err)
		require.Len(t, images, 1)
		assert.Equal(t, testPNGBytes(), images[0])
	})

	t.Run("invalid image data returns error", func(t *testing.T) {
		_, _, err := extractChatPromptAndImages(ctx, []ChatMessage{{
			Role: "user",
			Content: []any{
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,!!!"}},
			},
		}})
		require.NotNil(t, err)
		assert.Contains(t, err.Error(), "decode image data")
	})
}

func TestResolveChatImageBlock(t *testing.T) {
	pngB64 := base64.StdEncoding.EncodeToString(testPNGBytes())

	t.Run("valid data uri", func(t *testing.T) {
		block := imageBlock("data:image/png;base64," + pngB64)
		got, err := resolveChatImageBlock(context.Background(), block)
		require.Nil(t, err)
		assert.Equal(t, testPNGBytes(), got)
	})

	t.Run("no image produced", func(t *testing.T) {
		block := imageBlock("data:image/png;base64," + pngB64)
		block.ImageURL = nil
		_, err := resolveChatImageBlock(context.Background(), block)
		require.NotNil(t, err)
		assert.Contains(t, err.Error(), "image_url produced no image")
	})

	t.Run("unsupported scheme", func(t *testing.T) {
		block := imageBlock("ftp://example.com/a.png")
		_, err := resolveChatImageBlock(context.Background(), block)
		require.NotNil(t, err)
		assert.Contains(t, err.Error(), "unsupported URL scheme")
	})
}

func imageBlock(url string) upstream.ContentBlock {
	return upstream.ContentBlock{Type: "image_url", ImageURL: &upstream.ImageURLBlock{URL: url}}
}
