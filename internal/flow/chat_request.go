package flow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/crmmc/grokforge/internal/xai"
)

func (f *ChatFlow) buildXAIRequest(ctx context.Context, req *ChatRequest, client xai.Client) (*xai.ChatRequest, error) {
	// Format tool history: convert tool_calls and tool-role messages to text
	formattedMessages := FormatToolHistory(req.Messages)

	messages := make([]xai.Message, 0, len(formattedMessages))
	fileAttachments := make([]string, 0)

	// Build tool prompt if tools present
	toolPrompt := BuildToolPrompt(req.Tools, req.ToolChoice, req.ParallelToolCalls)

	for i, m := range formattedMessages {
		var textContent string

		// Process multimodal content
		switch c := m.Content.(type) {
		case string:
			textContent = c
		case map[string]any:
			if content, ok := formatStructuredMessage(m.Role, c); ok {
				textContent = content
				break
			}
			raw, err := json.Marshal(c)
			if err != nil {
				return nil, err
			}
			textContent = string(raw)
		default:
			content, attachmentIDs, err := processMultimodalForXAI(ctx, client, m.Content)
			if err != nil {
				return nil, err
			}
			textContent = content
			fileAttachments = append(fileAttachments, attachmentIDs...)
		}
		// Prepend tool prompt to first system message
		if i == 0 && m.Role == "system" && toolPrompt != "" {
			textContent = toolPrompt + "\n\n" + textContent
		}

		messages = append(messages, xai.Message{Role: m.Role, Content: textContent})
	}

	// If no system message but tools present, prepend one
	if toolPrompt != "" && (len(messages) == 0 || messages[0].Role != "system") {
		sysMsg := xai.Message{Role: "system", Content: toolPrompt}
		messages = append([]xai.Message{sysMsg}, messages...)
	}

	// Validate: reject if all messages have empty content
	if allMessagesEmpty(messages) {
		return nil, errors.New("all messages have empty content")
	}

	xaiReq := &xai.ChatRequest{
		Messages:        messages,
		Model:           req.Model,
		Stream:          true, // always stream
		Temperature:     req.Temperature,
		TopP:            req.TopP,
		MaxTokens:       req.MaxTokens,
		FileAttachments: fileAttachments,
	}

	// Populate Grok-specific params from app config
	if appCfg := f.cfg.AppConfig; appCfg != nil {
		xaiReq.Temporary = appCfg.Temporary
		xaiReq.CustomInstruction = appCfg.CustomInstruction
		xaiReq.DisableMemory = appCfg.DisableMemory
	}

	xaiReq.ReasoningEffort = req.ReasoningEffort

	return xaiReq, nil
}

func processMultimodalForXAI(ctx context.Context, client xai.Client, content any) (string, []string, error) {
	rawContent := content
	if items, ok := content.([]map[string]any); ok {
		arr := make([]any, 0, len(items))
		for _, item := range items {
			arr = append(arr, item)
		}
		rawContent = arr
	}

	blocks, err := ParseMultimodalContent(rawContent)
	if err != nil {
		return "", nil, err
	}
	processed, err := ProcessContent(ctx, blocks)
	if err != nil {
		return "", nil, err
	}

	attachmentIDs := make([]string, 0, len(processed.Images))
	for i, dataURI := range processed.Images {
		fileID, uploadErr := uploadDataURIAttachment(ctx, client, dataURI, i)
		if uploadErr != nil {
			return "", nil, uploadErr
		}
		attachmentIDs = append(attachmentIDs, fileID)
	}
	return processed.Text, attachmentIDs, nil
}

func uploadDataURIAttachment(ctx context.Context, client xai.Client, dataURI string, index int) (string, error) {
	if client == nil {
		return "", errors.New("xai client is nil")
	}
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
	fileID, _, err := client.UploadFile(ctx, fileName, mimeType, parts[1])
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

// allMessagesEmpty returns true if every message has empty (whitespace-only) content.
func allMessagesEmpty(messages []xai.Message) bool {
	for _, m := range messages {
		if strings.TrimSpace(m.Content) != "" {
			return false
		}
	}
	return true
}
