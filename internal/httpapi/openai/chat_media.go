package openai

import (
	"context"
	"fmt"
	"strings"

	"github.com/crmmc/grokforge/internal/flow"
)

func extractChatPromptAndImages(ctx context.Context, messages []ChatMessage) (string, [][]byte, error) {
	lastText := ""
	images := make([][]byte, 0)

	for _, msg := range messages {
		role := strings.TrimSpace(msg.Role)

		switch content := msg.Content.(type) {
		case string:
			if text := strings.TrimSpace(content); text != "" {
				lastText = text
			}
		default:
			blocks, err := flow.ParseMultimodalContent(content)
			if err != nil {
				continue
			}
			for _, block := range blocks {
				if text := strings.TrimSpace(block.Text); text != "" {
					lastText = text
				}
				if role != "user" || block.Type != "image_url" {
					continue
				}
				rawImage, err := resolveChatImageBlock(ctx, block)
				if err != nil {
					return "", nil, err
				}
				images = append(images, rawImage)
			}
		}
	}

	return lastText, images, nil
}

func resolveChatImageBlock(ctx context.Context, block flow.ContentBlock) ([]byte, error) {
	processed, err := flow.ProcessContent(ctx, []flow.ContentBlock{block})
	if err != nil {
		return nil, err
	}
	if len(processed.Images) == 0 {
		return nil, fmt.Errorf("image_url produced no image")
	}
	return decodeImageDataURI(processed.Images[0])
}
