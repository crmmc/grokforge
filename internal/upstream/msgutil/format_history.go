package msgutil

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/crmmc/grokforge/internal/upstream"
)

// FormatToolHistory converts assistant messages with tool_calls and tool-role messages
// into text format suitable for Grok's web API which only accepts a single message string.
//
// - assistant + tool_calls -> content appended with <tool_call>JSON</tool_call> blocks
// - tool role -> user role, content formatted as "tool (name, call_id): content"
func FormatToolHistory(messages []upstream.Message) []upstream.Message {
	result := make([]upstream.Message, 0, len(messages))
	for _, msg := range messages {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			var parts []string
			if text := ContentToString(msg.Content); text != "" {
				parts = append(parts, text)
			}
			for _, tc := range msg.ToolCalls {
				parts = append(parts, toolCallBlock(tc))
			}
			result = append(result, upstream.Message{
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

func toolCallBlock(tc upstream.ToolCall) string {
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

func formatToolResultMessage(msg upstream.Message) upstream.Message {
	toolName := strings.TrimSpace(msg.Name)
	toolCallID := strings.TrimSpace(msg.ToolCallID)
	content := msg.Content
	if contentMap, ok := msg.Content.(map[string]any); ok {
		if toolName == "" {
			toolName = StringFromAny(contentMap["name"])
		}
		if toolCallID == "" {
			toolCallID = StringFromAny(contentMap["tool_call_id"])
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
	return upstream.Message{
		Role:    "user",
		Content: fmt.Sprintf("tool (%s, %s): %s", toolName, toolCallID, ContentToString(content)),
	}
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
