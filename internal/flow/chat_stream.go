package flow

import (
	"context"
	"time"

	"github.com/crmmc/grokforge/internal/xai"
)

func (f *ChatFlow) streamEvents(ctx context.Context, eventCh <-chan xai.StreamEvent, outCh chan<- StreamEvent, dl DownloadFunc) (bool, *Usage, bool, time.Duration, error) {
	var lastUsage *Usage
	var outputChars int
	var ttft time.Duration
	estimated := false
	streamStart := time.Now()
	gotFirstToken := false
	filterTags := []string{}
	if f.cfg != nil && len(f.cfg.FilterTags) > 0 {
		filterTags = f.cfg.FilterTags
	}
	tokenFilter := newStreamTokenFilter(filterTags)
	for {
		select {
		case <-ctx.Done():
			return false, nil, false, 0, ctx.Err()
		case event, ok := <-eventCh:
			if !ok {
				// Channel closed normally = success. Send finish event.
				// Build estimated usage if upstream didn't provide real counts.
				if lastUsage == nil {
					lastUsage = &Usage{}
				}
				if lastUsage.CompletionTokens == 0 && outputChars > 0 {
					lastUsage.CompletionTokens = estimateTokens(outputChars)
					lastUsage.TotalTokens = lastUsage.PromptTokens + lastUsage.CompletionTokens
					estimated = true
				}
				stop := "stop"
				outCh <- StreamEvent{FinishReason: &stop, Usage: lastUsage}
				return true, lastUsage, estimated, ttft, nil
			}
			if event.Error != nil {
				return false, nil, false, 0, event.Error
			}
			// Parse and forward event
			flowEvent := f.parseEvent(event)
			if flowEvent.Error != nil {
				return false, nil, false, 0, flowEvent.Error
			}
			flowEvent = tokenFilter.Apply(flowEvent)
			flowEvent.Downloader = dl
			contentLen := len(flowEvent.Content) + len(flowEvent.ReasoningContent)
			outputChars += contentLen
			// Record TTFT on first content-bearing token
			if !gotFirstToken && contentLen > 0 {
				ttft = time.Since(streamStart)
				gotFirstToken = true
			}
			if flowEvent.Usage != nil {
				lastUsage = flowEvent.Usage
			}
			outCh <- flowEvent
		}
	}
}

// estimateTokens provides a rough token count from character length.
// Grok web API does not expose real token counts, so we estimate:
// ~4 chars per token for English, ~2 for CJK — use 3 as a balanced average.
func estimateTokens(chars int) int {
	if chars <= 0 {
		return 0
	}
	return (chars + 2) / 3
}
