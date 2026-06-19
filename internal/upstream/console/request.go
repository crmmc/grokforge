package console

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/crmmc/grokforge/internal/upstream/msgutil"
)

type ConsoleRequest struct {
	Model                   string             `json:"model"`
	Input                   []ConsoleInputItem `json:"input"`
	Instructions            string             `json:"instructions,omitempty"`
	Stream                  bool               `json:"stream"`
	Temperature             *float64           `json:"temperature,omitempty"`
	TopP                    *float64           `json:"top_p,omitempty"`
	MaxTokens               *int               `json:"max_output_tokens,omitempty"`
	ReasoningEffort         string             `json:"-"`
	SupportsReasoningEffort bool               `json:"-"`
	WebSearch               bool               `json:"-"`
}

type ConsoleInputItem struct {
	Role    string           `json:"role"`
	Content []ConsoleContent `json:"content"`
}

type ConsoleContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

func (c *ConsoleUpstream) buildBody(req *upstream.ChatRequest) ([]byte, error) {
	consoleReq, err := buildConsoleRequest(req)
	if err != nil {
		return nil, err
	}
	return buildConsoleBody(consoleReq)
}

func buildConsoleRequest(req *upstream.ChatRequest) (*ConsoleRequest, error) {
	if req == nil {
		return nil, errors.New("chat request is nil")
	}

	formattedMessages := msgutil.FormatToolHistory(req.Messages)
	toolPrompt := msgutil.BuildToolPrompt(req.Tools, req.ToolChoice, req.ParallelToolCalls)
	instructionParts := make([]string, 0)
	if strings.TrimSpace(toolPrompt) != "" {
		instructionParts = append(instructionParts, toolPrompt)
	}
	if strings.TrimSpace(req.CustomInstruction) != "" {
		instructionParts = append(instructionParts, req.CustomInstruction)
	}

	input := make([]ConsoleInputItem, 0, len(formattedMessages))
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
		input = append(input, ConsoleInputItem{
			Role:    consoleInputRole(m.Role),
			Content: content,
		})
	}

	if len(input) == 0 && len(instructionParts) == 0 {
		return nil, errors.New("all messages have empty content")
	}

	return &ConsoleRequest{
		Model:                   req.Model,
		Input:                   input,
		Instructions:            strings.Join(instructionParts, "\n\n"),
		Stream:                  true,
		Temperature:             req.Temperature,
		TopP:                    req.TopP,
		MaxTokens:               req.MaxTokens,
		ReasoningEffort:         req.ReasoningEffort,
		SupportsReasoningEffort: req.SupportsReasoningEffort,
		WebSearch:               req.WebSearch,
	}, nil
}

func buildConsoleBody(req *ConsoleRequest) ([]byte, error) {
	if req == nil {
		return nil, errors.New("console request is nil")
	}
	if strings.TrimSpace(req.Model) == "" {
		return nil, errors.New("console model is required")
	}

	payload := map[string]any{
		"model":  req.Model,
		"input":  req.Input,
		"stream": true,
	}
	if strings.TrimSpace(req.Instructions) != "" {
		payload["instructions"] = req.Instructions
	}
	if req.Temperature != nil {
		payload["temperature"] = *req.Temperature
	}
	if req.TopP != nil {
		payload["top_p"] = *req.TopP
	}
	if req.MaxTokens != nil {
		payload["max_output_tokens"] = *req.MaxTokens
	}
	if effort := consoleReasoningEffort(req.ReasoningEffort, req.SupportsReasoningEffort); effort != "" {
		payload["reasoning"] = map[string]any{"effort": effort}
	}
	if req.WebSearch {
		payload["tools"] = []map[string]string{{"type": "web_search"}}
	}
	return json.Marshal(payload)
}

func consoleReasoningEffort(effort string, supported bool) string {
	if !supported {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "", "none":
		return ""
	case "xhigh":
		return "high"
	default:
		return strings.ToLower(strings.TrimSpace(effort))
	}
}

func consoleInstructionText(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case map[string]any:
		if typ, _ := c["type"].(string); typ == "text" {
			return msgutil.StringFromAny(c["text"])
		}
		if text, ok := msgutil.FormatStructuredMessage("system", c); ok {
			return text
		}
		return consoleJSONFallback(c)
	case []any:
		parts := make([]string, 0, len(c))
		for _, item := range c {
			if block, ok := item.(map[string]any); ok {
				if typ, _ := block["type"].(string); typ == "text" {
					parts = append(parts, msgutil.StringFromAny(block["text"]))
					continue
				}
			}
			parts = append(parts, consoleJSONFallback(item))
		}
		return strings.Join(parts, "")
	default:
		return msgutil.ContentToString(content)
	}
}

func consoleContentForRole(role string, content any) []ConsoleContent {
	textType := consoleTextType(role)
	switch c := content.(type) {
	case string:
		return consoleTextContent(textType, c)
	case map[string]any:
		if block, ok := consoleContentPart(role, c); ok {
			return []ConsoleContent{block}
		}
		if text, ok := msgutil.FormatStructuredMessage(role, c); ok {
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
		out := make([]ConsoleContent, 0, len(c))
		for _, item := range c {
			blockMap, ok := item.(map[string]any)
			if ok {
				if block, ok := consoleContentPart(role, blockMap); ok {
					out = append(out, block)
					continue
				}
			}
			out = append(out, ConsoleContent{Type: textType, Text: consoleJSONFallback(item)})
		}
		return out
	case []upstream.ContentBlock:
		out := make([]ConsoleContent, 0, len(c))
		for _, block := range c {
			switch block.Type {
			case "text":
				out = append(out, ConsoleContent{Type: textType, Text: block.Text})
			case "image_url":
				if block.ImageURL != nil && consoleInputRole(role) == "user" {
					out = append(out, ConsoleContent{Type: "input_image", ImageURL: block.ImageURL.URL})
				}
			default:
				out = append(out, ConsoleContent{Type: textType, Text: consoleJSONFallback(block)})
			}
		}
		return out
	default:
		return consoleTextContent(textType, msgutil.ContentToString(content))
	}
}

func consoleContentPart(role string, block map[string]any) (ConsoleContent, bool) {
	typ, _ := block["type"].(string)
	textType := consoleTextType(role)
	switch typ {
	case "text":
		return ConsoleContent{Type: textType, Text: msgutil.StringFromAny(block["text"])}, true
	case "image_url":
		if consoleInputRole(role) != "user" {
			return ConsoleContent{Type: textType, Text: consoleJSONFallback(block)}, true
		}
		if imageURL := consoleImageURL(block["image_url"]); imageURL != "" {
			return ConsoleContent{Type: "input_image", ImageURL: imageURL}, true
		}
		return ConsoleContent{Type: textType, Text: consoleJSONFallback(block)}, true
	case "file", "input_file", "input_audio", "audio":
		return ConsoleContent{Type: textType, Text: consoleJSONFallback(block)}, true
	default:
		return ConsoleContent{}, false
	}
}

func consoleImageURL(v any) string {
	switch img := v.(type) {
	case string:
		return strings.TrimSpace(img)
	case map[string]any:
		return msgutil.StringFromAny(img["url"])
	default:
		return ""
	}
}

func consoleTextContent(typ, text string) []ConsoleContent {
	if text == "" {
		return nil
	}
	return []ConsoleContent{{Type: typ, Text: text}}
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
