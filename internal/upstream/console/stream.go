package console

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/crmmc/grokforge/internal/upstream/msgutil"
)

const maxConsoleStreamFrameSize = 8 * 1024 * 1024

type consoleSSEBlock struct {
	event string
	data  []string
}

func (c *ConsoleUpstream) parseStream(ctx context.Context, body io.Reader, ch chan<- upstream.StreamEvent) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), maxConsoleStreamFrameSize)

	var block consoleSSEBlock
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			done, err := emitConsoleSSEBlock(ctx, block, ch)
			if done || err != nil {
				return err
			}
			block = consoleSSEBlock{}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if value, ok := strings.CutPrefix(line, "event:"); ok {
			block.event = strings.TrimSpace(value)
			continue
		}
		if value, ok := strings.CutPrefix(line, "data:"); ok {
			block.data = append(block.data, strings.TrimPrefix(value, " "))
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("%w: %v", upstream.ErrNetwork, err)
	}
	_, err := emitConsoleSSEBlock(ctx, block, ch)
	return err
}

func emitConsoleSSEBlock(ctx context.Context, block consoleSSEBlock, ch chan<- upstream.StreamEvent) (bool, error) {
	if len(block.data) == 0 {
		return false, nil
	}
	data := strings.Join(block.data, "\n")
	if data == "[DONE]" {
		return true, nil
	}

	ev := parseConsoleEvent(strings.TrimSpace(block.event), json.RawMessage(data))
	if isEmptyConsoleEvent(ev) {
		return false, nil
	}
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case ch <- ev:
		return false, nil
	}
}

func parseConsoleEvent(eventName string, data json.RawMessage) upstream.StreamEvent {
	if strings.TrimSpace(eventName) == "" {
		eventName = consoleEventType(data)
	}

	switch eventName {
	case "response.output_text.delta":
		return upstream.StreamEvent{Content: consoleDeltaText(data)}
	case "response.reasoning_summary_text.delta", "response.reasoning_summary.delta":
		return upstream.StreamEvent{ReasoningContent: consoleDeltaText(data), IsThinking: true}
	case "response.output_text.annotation.added", "response.output_item.done":
		return upstream.StreamEvent{SearchSources: consoleSearchSources(data)}
	case "response.completed":
		if usage := consoleUsage(data); usage != nil {
			return upstream.StreamEvent{Usage: usage}
		}
		return upstream.StreamEvent{}
	case "response.failed", "response.error", "error":
		return upstream.StreamEvent{Error: consoleError(data, eventName)}
	default:
		return upstream.StreamEvent{}
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

func consoleUsage(raw json.RawMessage) *upstream.Usage {
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
	return &upstream.Usage{PromptTokens: prompt, CompletionTokens: completion, TotalTokens: total}
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

func consoleSearchSources(raw json.RawMessage) []upstream.SearchSource {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	var sources []upstream.SearchSource
	collectConsoleSearchSources(payload, &sources)
	return sources
}

func collectConsoleSearchSources(v any, sources *[]upstream.SearchSource) {
	switch value := v.(type) {
	case map[string]any:
		if url := msgutil.StringFromAny(value["url"]); url != "" {
			*sources = append(*sources, upstream.SearchSource{
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
		if value := msgutil.StringFromAny(m[key]); value != "" {
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
		return classifyConsoleError(asString)
	}

	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("%w: console %s", upstream.ErrServerError, eventName)
	}
	if msg := findConsoleErrorMessage(payload); msg != "" {
		return classifyConsoleError(msg)
	}
	return fmt.Errorf("%w: console %s", upstream.ErrServerError, eventName)
}

func classifyConsoleError(msg string) error {
	trimmed := strings.TrimSpace(msg)
	lower := strings.ToLower(trimmed)
	switch {
	case strings.Contains(lower, "rate") || strings.Contains(lower, "too many") || strings.Contains(lower, "429"):
		return fmt.Errorf("%w: %s", upstream.ErrRateLimited, trimmed)
	case strings.Contains(lower, "credit") || strings.Contains(lower, "quota") || strings.Contains(lower, "payment required") || strings.Contains(lower, "402"):
		return fmt.Errorf("%w: %s", upstream.ErrCreditExhausted, trimmed)
	default:
		return fmt.Errorf("%w: %s", upstream.ErrServerError, trimmed)
	}
}

func findConsoleErrorMessage(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return ""
	}
	if msg := msgutil.StringFromAny(m["message"]); msg != "" {
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

func isEmptyConsoleEvent(ev upstream.StreamEvent) bool {
	return ev.Content == "" && ev.ReasoningContent == "" && ev.FinishReason == nil &&
		ev.Usage == nil && len(ev.ToolCalls) == 0 && ev.Error == nil &&
		!ev.IsThinking && ev.RolloutID == "" && len(ev.SearchSources) == 0 &&
		ev.Downloader == nil
}
