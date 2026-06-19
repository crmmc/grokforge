package msgutil

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ContentToString converts message content to string.
func ContentToString(content any) string {
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

// FormatStructuredMessage formats decoded structured message content.
func FormatStructuredMessage(role string, content map[string]any) (string, bool) {
	rawContent, ok := content["content"]
	if !ok {
		return "", false
	}
	text := ContentToString(rawContent)
	if role != "tool" {
		return text, true
	}
	return formatToolMessageContent(
		text,
		StringFromAny(content["name"]),
		StringFromAny(content["tool_call_id"]),
	), true
}

// StringFromAny returns a trimmed string when v is a string.
func StringFromAny(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}
