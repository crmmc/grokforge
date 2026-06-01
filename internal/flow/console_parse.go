package flow

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/crmmc/grokforge/internal/xai"
)

type consoleWrappedEvent struct {
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

func (f *ChatFlow) parseConsoleEvent(event xai.StreamEvent) StreamEvent {
	var wrapped consoleWrappedEvent
	if err := json.Unmarshal(event.Data, &wrapped); err != nil {
		return StreamEvent{Error: err}
	}
	data := wrapped.Data
	if len(data) == 0 {
		data = event.Data
	}
	eventName := strings.TrimSpace(wrapped.Event)
	if eventName == "" {
		eventName = consoleEventType(data)
	}

	switch eventName {
	case "response.output_text.delta":
		return StreamEvent{Content: consoleDeltaText(data)}
	case "response.reasoning_summary_text.delta", "response.reasoning_summary.delta":
		return StreamEvent{ReasoningContent: consoleDeltaText(data), IsThinking: true}
	case "response.output_text.annotation.added", "response.output_item.done":
		return StreamEvent{SearchSources: consoleSearchSources(data)}
	case "response.completed":
		if usage := consoleUsage(data); usage != nil {
			return StreamEvent{Usage: usage}
		}
		return StreamEvent{}
	case "response.failed", "response.error", "error":
		return StreamEvent{Error: consoleError(data, eventName)}
	default:
		return StreamEvent{}
	}
}

func consoleEventType(raw json.RawMessage) string {
	var payload struct {
		Type string `json:"type"`
	}
	_ = json.Unmarshal(raw, &payload)
	return strings.TrimSpace(payload.Type)
}

func consoleDeltaText(raw json.RawMessage) string {
	var payload struct {
		Delta string `json:"delta"`
		Text  string `json:"text"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if payload.Delta != "" {
		return payload.Delta
	}
	return payload.Text
}

func consoleUsage(raw json.RawMessage) *Usage {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	usageMap, ok := findConsoleUsageMap(payload)
	if !ok {
		return nil
	}
	prompt := intFromConsoleMap(usageMap, "input_tokens", "prompt_tokens", "inputTokens", "promptTokens")
	completion := intFromConsoleMap(usageMap, "output_tokens", "completion_tokens", "outputTokens", "completionTokens")
	total := intFromConsoleMap(usageMap, "total_tokens", "totalTokens")
	if total == 0 {
		total = prompt + completion
	}
	return &Usage{PromptTokens: prompt, CompletionTokens: completion, TotalTokens: total}
}

func findConsoleUsageMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	if usage, ok := m["usage"].(map[string]any); ok {
		return usage, true
	}
	if response, ok := m["response"].(map[string]any); ok {
		if usage, ok := response["usage"].(map[string]any); ok {
			return usage, true
		}
	}
	return nil, false
}

func intFromConsoleMap(m map[string]any, keys ...string) int {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			return consoleNumber(value)
		}
	}
	return 0
}

func consoleNumber(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		value, _ := n.Int64()
		return int(value)
	default:
		return 0
	}
}

func consoleSearchSources(raw json.RawMessage) []SearchSource {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	var sources []SearchSource
	collectConsoleSearchSources(payload, &sources)
	return sources
}

func collectConsoleSearchSources(v any, sources *[]SearchSource) {
	switch value := v.(type) {
	case map[string]any:
		if url := stringFromAny(value["url"]); url != "" {
			*sources = append(*sources, SearchSource{
				URL:   url,
				Title: consoleSourceTitle(value),
				Type:  consoleSourceType(url),
			})
		}
		for _, child := range value {
			collectConsoleSearchSources(child, sources)
		}
	case []any:
		for _, child := range value {
			collectConsoleSearchSources(child, sources)
		}
	}
}

func consoleSourceTitle(m map[string]any) string {
	for _, key := range []string{"title", "text", "name", "query"} {
		if value := stringFromAny(m[key]); value != "" {
			return value
		}
	}
	return ""
}

func consoleSourceType(rawURL string) string {
	lower := strings.ToLower(rawURL)
	if strings.Contains(lower, "x.com/") || strings.Contains(lower, "twitter.com/") {
		return "x_post"
	}
	return "web"
}

func consoleError(raw json.RawMessage, eventName string) error {
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil && strings.TrimSpace(asString) != "" {
		return errors.New(asString)
	}

	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("console %s", eventName)
	}
	if msg := findConsoleErrorMessage(payload); msg != "" {
		return errors.New(msg)
	}
	return fmt.Errorf("console %s", eventName)
}

func findConsoleErrorMessage(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	if msg := stringFromAny(m["message"]); msg != "" {
		return msg
	}
	if errMap, ok := m["error"].(map[string]any); ok {
		if msg := findConsoleErrorMessage(errMap); msg != "" {
			return msg
		}
	}
	if response, ok := m["response"].(map[string]any); ok {
		if msg := findConsoleErrorMessage(response); msg != "" {
			return msg
		}
	}
	return ""
}
