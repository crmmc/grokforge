package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/crmmc/grokforge/internal/xai"
)

type chatRequestBuildError struct {
	err error
}

func (e chatRequestBuildError) Error() string { return e.err.Error() }
func (e chatRequestBuildError) Unwrap() error { return e.err }

func (f *ChatFlow) startChatRequest(ctx context.Context, req *ChatRequest, client xai.Client) (<-chan xai.StreamEvent, error) {
	if req.UseConsole {
		consoleReq, err := f.buildConsoleRequest(req)
		if err != nil {
			return nil, chatRequestBuildError{err: err}
		}
		return client.ConsoleResponses(ctx, consoleReq)
	}

	xaiReq, err := f.buildXAIRequest(ctx, req, client)
	if err != nil {
		return nil, chatRequestBuildError{err: err}
	}
	return client.Chat(ctx, xaiReq)
}

func (f *ChatFlow) buildConsoleRequest(req *ChatRequest) (*xai.ConsoleRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("chat request is nil")
	}

	formattedMessages := FormatToolHistory(req.Messages)
	toolPrompt := BuildToolPrompt(req.Tools, req.ToolChoice, req.ParallelToolCalls)
	instructionParts := make([]string, 0)
	if strings.TrimSpace(toolPrompt) != "" {
		instructionParts = append(instructionParts, toolPrompt)
	}
	if appCfg := f.appConfig(); appCfg != nil && strings.TrimSpace(appCfg.CustomInstruction) != "" {
		instructionParts = append(instructionParts, appCfg.CustomInstruction)
	}

	input := make([]xai.ConsoleInputItem, 0, len(formattedMessages))
	for _, m := range formattedMessages {
		switch strings.TrimSpace(m.Role) {
		case "system", "developer":
			if text := strings.TrimSpace(consoleInstructionText(m.Content)); text != "" {
				instructionParts = append(instructionParts, text)
			}
			continue
		}

		content := consoleContentForRole(m.Role, m.Content)
		if len(content) == 0 {
			continue
		}
		input = append(input, xai.ConsoleInputItem{
			Role:    consoleInputRole(m.Role),
			Content: content,
		})
	}

	if len(input) == 0 && len(instructionParts) == 0 {
		return nil, fmt.Errorf("all messages have empty content")
	}

	model := strings.TrimSpace(req.UpstreamModel)
	if model == "" {
		model = req.Model
	}
	reasoningEffort := req.ReasoningEffort
	if req.ForceThinking && reasoningEffort == "" {
		reasoningEffort = "high"
	}

	return &xai.ConsoleRequest{
		Model:                   model,
		Input:                   input,
		Instructions:            strings.Join(instructionParts, "\n\n"),
		Stream:                  true,
		Temperature:             req.Temperature,
		TopP:                    req.TopP,
		MaxTokens:               req.MaxTokens,
		ReasoningEffort:         reasoningEffort,
		SupportsReasoningEffort: req.ConsoleSupportsReasoningEffort,
		WebSearch:               req.ConsoleWebSearch,
	}, nil
}

func consoleInstructionText(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case map[string]any:
		if typ, _ := c["type"].(string); typ == "text" {
			return stringFromAny(c["text"])
		}
		if text, ok := formatStructuredMessage("system", c); ok {
			return text
		}
		return consoleJSONFallback(c)
	case []any:
		parts := make([]string, 0, len(c))
		for _, item := range c {
			if block, ok := item.(map[string]any); ok {
				if typ, _ := block["type"].(string); typ == "text" {
					parts = append(parts, stringFromAny(block["text"]))
					continue
				}
			}
			parts = append(parts, consoleJSONFallback(item))
		}
		return strings.Join(parts, "")
	default:
		return contentToString(content)
	}
}

func consoleContentForRole(role string, content any) []xai.ConsoleContent {
	textType := consoleTextType(role)
	switch c := content.(type) {
	case string:
		return consoleTextContent(textType, c)
	case map[string]any:
		if block, ok := consoleContentPart(role, c); ok {
			return []xai.ConsoleContent{block}
		}
		if text, ok := formatStructuredMessage(role, c); ok {
			return consoleTextContent(textType, text)
		}
		return consoleTextContent(textType, consoleJSONFallback(c))
	case []map[string]any:
		items := make([]any, 0, len(c))
		for _, item := range c {
			items = append(items, item)
		}
		return consoleContentForRole(role, items)
	case []any:
		out := make([]xai.ConsoleContent, 0, len(c))
		for _, item := range c {
			blockMap, ok := item.(map[string]any)
			if ok {
				if block, ok := consoleContentPart(role, blockMap); ok {
					out = append(out, block)
					continue
				}
			}
			out = append(out, xai.ConsoleContent{Type: textType, Text: consoleJSONFallback(item)})
		}
		return out
	case []ContentBlock:
		out := make([]xai.ConsoleContent, 0, len(c))
		for _, block := range c {
			switch block.Type {
			case "text":
				out = append(out, xai.ConsoleContent{Type: textType, Text: block.Text})
			case "image_url":
				if block.ImageURL != nil && consoleInputRole(role) == "user" {
					out = append(out, xai.ConsoleContent{Type: "input_image", ImageURL: block.ImageURL.URL})
				}
			default:
				out = append(out, xai.ConsoleContent{Type: textType, Text: consoleJSONFallback(block)})
			}
		}
		return out
	default:
		return consoleTextContent(textType, contentToString(content))
	}
}

func consoleContentPart(role string, block map[string]any) (xai.ConsoleContent, bool) {
	typ, _ := block["type"].(string)
	textType := consoleTextType(role)
	switch typ {
	case "text":
		return xai.ConsoleContent{Type: textType, Text: stringFromAny(block["text"])}, true
	case "image_url":
		if consoleInputRole(role) != "user" {
			return xai.ConsoleContent{Type: textType, Text: consoleJSONFallback(block)}, true
		}
		if imageURL := consoleImageURL(block["image_url"]); imageURL != "" {
			return xai.ConsoleContent{Type: "input_image", ImageURL: imageURL}, true
		}
		return xai.ConsoleContent{Type: textType, Text: consoleJSONFallback(block)}, true
	case "file", "input_file", "input_audio", "audio":
		return xai.ConsoleContent{Type: textType, Text: consoleJSONFallback(block)}, true
	default:
		return xai.ConsoleContent{}, false
	}
}

func consoleImageURL(v any) string {
	switch img := v.(type) {
	case string:
		return strings.TrimSpace(img)
	case map[string]any:
		return stringFromAny(img["url"])
	default:
		return ""
	}
}

func consoleTextContent(typ, text string) []xai.ConsoleContent {
	if text == "" {
		return nil
	}
	return []xai.ConsoleContent{{Type: typ, Text: text}}
}

func consoleTextType(role string) string {
	if strings.TrimSpace(role) == "assistant" {
		return "output_text"
	}
	return "input_text"
}

func consoleInputRole(role string) string {
	if strings.TrimSpace(role) == "assistant" {
		return "assistant"
	}
	return "user"
}

func consoleJSONFallback(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(data)
}
