package flow

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FormatToolHistory converts assistant messages with tool_calls and tool-role messages
// into text format suitable for Grok's web API which only accepts a single message string.
//
// - assistant + tool_calls → content appended with <tool_call>JSON</tool_call> blocks
// - tool role → user role, content formatted as "tool (name, call_id): content"
func FormatToolHistory(messages []Message) []Message {
	result := make([]Message, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			// Convert assistant tool_calls to text representation
			var parts []string
			if text := contentToString(msg.Content); text != "" {
				parts = append(parts, text)
			}
			for _, tc := range msg.ToolCalls {
				parts = append(parts, toolCallBlock(tc))
			}
			result = append(result, Message{
				Role:    "assistant",
				Content: strings.Join(parts, "\n"),
			})
		} else if msg.Role == "tool" {
			result = append(result, formatToolResultMessage(msg))
		} else {
			result = append(result, msg)
		}
	}
	return result
}

func toolCallBlock(tc ToolCall) string {
	toolName := strings.TrimSpace(tc.Function.Name)
	if toolName == "" {
		toolName = "unknown_tool"
	}
	args := normalizeToolCallArgumentsForPrompt(tc.Function.Arguments)
	return fmt.Sprintf(`<tool_call>{"name":"%s","arguments":%s}</tool_call>`, toolName, args)
}

func normalizeToolCallArgumentsForPrompt(arguments string) string {
	trimmed := strings.TrimSpace(arguments)
	if trimmed == "" {
		return "{}"
	}
	var raw json.RawMessage
	if json.Unmarshal([]byte(trimmed), &raw) == nil {
		return string(raw)
	}
	escaped, err := json.Marshal(trimmed)
	if err != nil {
		return `""`
	}
	return string(escaped)
}

func formatToolResultMessage(msg Message) Message {
	toolName := strings.TrimSpace(msg.Name)
	toolCallID := strings.TrimSpace(msg.ToolCallID)
	content := msg.Content
	if contentMap, ok := msg.Content.(map[string]any); ok {
		if toolName == "" {
			toolName = stringFromAny(contentMap["name"])
		}
		if toolCallID == "" {
			toolCallID = stringFromAny(contentMap["tool_call_id"])
		}
		if mappedContent, ok := contentMap["content"]; ok {
			content = mappedContent
		}
	}
	if toolName == "" {
		toolName = "unknown"
	}
	if toolCallID == "" {
		toolCallID = "unknown_call"
	}
	return Message{
		Role:    "user",
		Content: fmt.Sprintf("tool (%s, %s): %s", toolName, toolCallID, contentToString(content)),
	}
}

// contentToString converts message content to string.
func contentToString(content any) string {
	switch c := content.(type) {
	case string:
		return c
	case nil:
		return ""
	default:
		b, err := json.Marshal(c)
		if err != nil {
			return fmt.Sprintf("%v", c)
		}
		return string(b)
	}
}

func formatStructuredMessage(role string, content map[string]any) (string, bool) {
	rawContent, ok := content["content"]
	if !ok {
		return "", false
	}
	text := contentToString(rawContent)
	if role != "tool" {
		return text, true
	}
	return formatToolMessageContent(
		text,
		stringFromAny(content["name"]),
		stringFromAny(content["tool_call_id"]),
	), true
}

func formatToolMessageContent(content, name, toolCallID string) string {
	toolName := name
	if toolName == "" {
		toolName = "unknown"
	}
	if toolCallID == "" {
		toolCallID = "unknown_call"
	}
	return fmt.Sprintf("tool (%s, %s): %s", toolName, toolCallID, content)
}

func stringFromAny(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}
