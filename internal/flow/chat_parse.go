package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/crmmc/grokforge/internal/store"
	"github.com/crmmc/grokforge/internal/xai"
)

func (f *ChatFlow) parseEvent(event xai.StreamEvent) StreamEvent {
	// Parse the raw JSON from xai event — field names match xAI's actual API:
	// "token" = text chunk, "isThinking" = boolean flag for reasoning content.
	var result struct {
		Result struct {
			Response struct {
				Token      string `json:"token"`
				IsThinking bool   `json:"isThinking"`
				ModelName  string `json:"modelName"`
				RolloutID  string `json:"rolloutId"`
				// modelResponse contains generated images and final message
				ModelResponse *struct {
					Message            string   `json:"message"`
					GeneratedImageUrls []string `json:"generatedImageUrls"`
				} `json:"modelResponse"`
				// cardAttachment contains external image/link cards
				CardAttachment *struct {
					JSONData string `json:"jsonData"`
				} `json:"cardAttachment"`
			} `json:"response"`
		} `json:"result"`
	}
	if err := json.Unmarshal(event.Data, &result); err != nil {
		return StreamEvent{Error: err}
	}

	resp := result.Result.Response
	token := resp.Token

	if resp.IsThinking {
		slog.Debug("flow: thinking token received", "len", len(token))
	}

	var content, reasoning string

	// Route token based on isThinking flag
	if resp.IsThinking {
		reasoning = token
	} else {
		content = token
	}

	// Extract images from modelResponse
	if mr := resp.ModelResponse; mr != nil {
		for _, imgURL := range mr.GeneratedImageUrls {
			parts := strings.Split(imgURL, "/")
			imgID := "image"
			if len(parts) >= 2 {
				imgID = parts[len(parts)-2]
			}
			content += fmt.Sprintf("\n![%s](%s)", imgID, imgURL)
		}
	}

	// Extract images from cardAttachment
	if ca := resp.CardAttachment; ca != nil && ca.JSONData != "" {
		var card struct {
			Image struct {
				Original string `json:"original"`
				Title    string `json:"title"`
			} `json:"image"`
		}
		if json.Unmarshal([]byte(ca.JSONData), &card) == nil && card.Image.Original != "" {
			title := strings.ReplaceAll(strings.TrimSpace(card.Image.Title), "\n", " ")
			if title == "" {
				title = "image"
			}
			content += fmt.Sprintf("\n![%s](%s)", title, card.Image.Original)
		}
	}

	// Parse tool calls from response content
	return StreamEvent{
		Content:          content,
		ReasoningContent: reasoning,
		IsThinking:       resp.IsThinking,
		RolloutID:        strings.TrimSpace(resp.RolloutID),
		Usage:            extractUsage(event.Data),
	}
}

func extractUsage(data json.RawMessage) *Usage {
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	return findUsage(payload)
}

func findUsage(payload map[string]any) *Usage {
	if usage := parseUsageValue(payload["usage"]); usage != nil {
		return usage
	}
	if usage := parseUsageValue(payload["tokenUsage"]); usage != nil {
		return usage
	}
	if result, ok := payload["result"].(map[string]any); ok {
		if usage := findUsage(result); usage != nil {
			return usage
		}
	}
	if response, ok := payload["response"].(map[string]any); ok {
		if usage := findUsage(response); usage != nil {
			return usage
		}
	}
	return nil
}

func parseUsageValue(value any) *Usage {
	usageMap, ok := value.(map[string]any)
	if !ok {
		return nil
	}

	usage := Usage{
		PromptTokens:     pickUsageInt(usageMap, "prompt_tokens", "promptTokens", "input_tokens", "inputTokens"),
		CompletionTokens: pickUsageInt(usageMap, "completion_tokens", "completionTokens", "output_tokens", "outputTokens"),
		TotalTokens:      pickUsageInt(usageMap, "total_tokens", "totalTokens"),
	}

	if usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0 {
		return nil
	}
	if usage.TotalTokens == 0 {
		usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	}
	return &usage
}

func pickUsageInt(values map[string]any, keys ...string) int {
	for _, key := range keys {
		if raw, ok := values[key]; ok {
			if val, ok := intFromAny(raw); ok {
				return val
			}
		}
	}
	return 0
}

func intFromAny(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	case json.Number:
		i, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	default:
		return 0, false
	}
}

// estimatePromptTokens estimates input token count from request messages.
func (f *ChatFlow) estimatePromptTokens(req *ChatRequest) int {
	var chars int
	for _, m := range req.Messages {
		chars += len(m.Role)
		switch c := m.Content.(type) {
		case string:
			chars += len(c)
		}
	}
	return estimateTokens(chars)
}

// recordUsage records an API usage log entry via the buffer (non-blocking).
func (f *ChatFlow) recordUsage(apiKeyID uint, tokenID uint, model, endpoint string, status int, latency time.Duration, ttft time.Duration, tokensInput, tokensOutput int, estimated bool) {
	if f.usageLog == nil {
		return
	}
	_ = f.usageLog.Record(context.Background(), &store.UsageLog{
		APIKeyID:     apiKeyID,
		TokenID:      tokenID,
		Model:        model,
		Endpoint:     endpoint,
		Status:       status,
		DurationMs:   latency.Milliseconds(),
		TTFTMs:       int(ttft.Milliseconds()),
		CacheTokens:  0,
		TokensInput:  tokensInput,
		TokensOutput: tokensOutput,
		Estimated:    estimated,
		CreatedAt:    time.Now(),
	})
}
