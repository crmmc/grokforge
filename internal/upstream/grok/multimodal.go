package grok

import (
	"context"
	"fmt"
	"strings"

	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/crmmc/grokforge/internal/upstream/mediautil"
)

func (g *GrokUpstream) processMultimodalContent(ctx context.Context, token string, content any) (string, []string, error) {
	rawContent := content
	if items, ok := content.([]map[string]any); ok {
		arr := make([]any, 0, len(items))
		for _, item := range items {
			arr = append(arr, item)
		}
		rawContent = arr
	}

	blocks, err := mediautil.ParseMultimodalContent(rawContent)
	if err != nil {
		return "", nil, err
	}
	processed, err := mediautil.ProcessContent(ctx, blocks)
	if err != nil {
		return "", nil, err
	}

	attachmentIDs := make([]string, 0, len(processed.Images))
	for i, dataURI := range processed.Images {
		fileID, uploadErr := g.uploadDataURIAttachment(ctx, token, dataURI, i)
		if uploadErr != nil {
			return "", nil, uploadErr
		}
		attachmentIDs = append(attachmentIDs, fileID)
	}
	return processed.Text, attachmentIDs, nil
}

func (g *GrokUpstream) uploadDataURIAttachment(ctx context.Context, token, dataURI string, index int) (string, error) {
	parts := strings.SplitN(dataURI, ",", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid image data URI")
	}

	meta := strings.TrimPrefix(parts[0], "data:")
	mimeType := "application/octet-stream"
	if semicolon := strings.Index(meta, ";"); semicolon > 0 {
		mimeType = meta[:semicolon]
	} else if meta != "" {
		mimeType = meta
	}

	fileName := fmt.Sprintf("image-%d.%s", index, mimeToExtension(mimeType))
	fileID, _, err := g.uploadFile(ctx, token, fileName, mimeType, parts[1])
	if err != nil {
		return "", fmt.Errorf("upload attachment: %w", err)
	}
	return fileID, nil
}

func mimeToExtension(mimeType string) string {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/png":
		return "png"
	case "image/webp":
		return "webp"
	case "image/gif":
		return "gif"
	case "image/jpeg", "image/jpg":
		return "jpg"
	default:
		return "bin"
	}
}

func (g *GrokUpstream) processMultimodal(ctx context.Context, token string, messages []upstream.Message) ([]upstream.Message, []string, error) {
	var allAttachments []string
	out := make([]upstream.Message, 0, len(messages))
	for _, m := range messages {
		switch m.Content.(type) {
		case string, map[string]any, nil:
			out = append(out, m)
		default:
			text, atts, err := g.processMultimodalContent(ctx, token, m.Content)
			if err != nil {
				return nil, nil, err
			}
			allAttachments = append(allAttachments, atts...)
			out = append(out, upstream.Message{
				Role:       m.Role,
				Content:    text,
				ToolCalls:  m.ToolCalls,
				Name:       m.Name,
				ToolCallID: m.ToolCallID,
			})
		}
	}
	return out, allAttachments, nil
}
