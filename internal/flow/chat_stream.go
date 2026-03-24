package flow

import (
	"context"
	"time"

	"github.com/crmmc/grokforge/internal/xai"
)

func (f *ChatFlow) streamEvents(ctx context.Context, eventCh <-chan xai.StreamEvent, outCh chan<- StreamEvent, dl DownloadFunc, tools []Tool) (bool, *Usage, bool, time.Duration, error) {
	var lastUsage *Usage
	var outputChars int
	var ttft time.Duration
	estimated := false
	streamStart := time.Now()
	gotFirstToken := false
	filterTags := f.filterTags()
	tokenFilter := newStreamTokenFilter(filterTags)
	toolParser := newStreamToolCallParser(tools)
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
				outputChars += flushStreamParsers(outCh, dl, streamStart, &ttft, &gotFirstToken, tokenFilter, toolParser)
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
			flowEvent.Content, flowEvent.ToolCalls = toolParser.Push(flowEvent.Content)
			flowEvent.Downloader = dl
			if flowEvent.Usage != nil {
				lastUsage = flowEvent.Usage
			}
			outputChars += emitStreamEvent(outCh, dl, streamStart, &ttft, &gotFirstToken, flowEvent)
		}
	}
}

func flushStreamParsers(outCh chan<- StreamEvent, dl DownloadFunc, streamStart time.Time, ttft *time.Duration, gotFirstToken *bool, tokenFilter *streamTokenFilter, toolParser *streamToolCallParser) int {
	var outputChars int
	pending := tokenFilter.Flush("")
	if pending != "" {
		text, calls := toolParser.Push(pending)
		outputChars += emitStreamEvent(outCh, dl, streamStart, ttft, gotFirstToken, StreamEvent{
			Content:   text,
			ToolCalls: calls,
		})
	}

	text, calls := toolParser.Flush()
	outputChars += emitStreamEvent(outCh, dl, streamStart, ttft, gotFirstToken, StreamEvent{
		Content:   text,
		ToolCalls: calls,
	})
	return outputChars
}

func emitStreamEvent(outCh chan<- StreamEvent, dl DownloadFunc, streamStart time.Time, ttft *time.Duration, gotFirstToken *bool, event StreamEvent) int {
	event.Downloader = dl
	contentLen := len(event.Content) + len(event.ReasoningContent)
	if !*gotFirstToken && contentLen > 0 {
		*ttft = time.Since(streamStart)
		*gotFirstToken = true
	}
	if event.Content == "" && event.ReasoningContent == "" && len(event.ToolCalls) == 0 && event.Usage == nil {
		return 0
	}
	outCh <- event
	return contentLen
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
