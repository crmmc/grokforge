package flow

import (
	"context"
	"strings"
	"time"

	"github.com/crmmc/grokforge/internal/store"
)

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

// normalizeXTitle builds a display title for an X/Twitter post.
func normalizeXTitle(username, text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return "𝕏/@" + username
	}
	runes := []rune(text)
	if len(runes) > 50 {
		return string(runes[:50]) + "…"
	}
	return text
}
