package flow

import (
	"context"
	"time"

	"github.com/crmmc/grokforge/internal/upstream"
)

func (f *ChatFlow) consumeUpstreamEvents(ctx context.Context, eventCh <-chan upstream.StreamEvent, outCh chan<- StreamEvent, tools []Tool) (bool, *Usage, bool, time.Duration, error) {
	var outputChars int
	var usage *Usage
	var ttft time.Duration
	streamStart := time.Now()
	gotFirstToken := false
	tokenFilter := newStreamTokenFilter(f.filterTags())
	toolParser := newStreamToolCallParser(tools)
	var searchSources []SearchSource
	seenURLs := make(map[string]struct{})
	for {
		select {
		case <-ctx.Done():
			return false, nil, false, 0, ctx.Err()
		case event, ok := <-eventCh:
			if !ok {
				outputChars += flushStreamParsers(outCh, streamStart, &ttft, &gotFirstToken, tokenFilter, toolParser)
				estimated := false
				if usage == nil {
					usage = &Usage{CompletionTokens: estimateTokens(outputChars)}
					usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
					estimated = true
				}
				stop := "stop"
				outCh <- StreamEvent{FinishReason: &stop, Usage: usage, SearchSources: searchSources}
				return true, usage, estimated, ttft, nil
			}
			if event.Error != nil {
				return false, nil, false, 0, event.Error
			}
			if event.Usage != nil {
				usage = event.Usage
				event.Usage = nil
			}
			for _, src := range event.SearchSources {
				if _, seen := seenURLs[src.URL]; !seen {
					seenURLs[src.URL] = struct{}{}
					searchSources = append(searchSources, src)
				}
			}
			event.SearchSources = nil
			event = tokenFilter.Apply(event)
			event.Content, event.ToolCalls = toolParser.Push(event.Content)
			outputChars += emitStreamEvent(outCh, streamStart, &ttft, &gotFirstToken, event)
		}
	}
}

func flushStreamParsers(outCh chan<- StreamEvent, streamStart time.Time, ttft *time.Duration, gotFirstToken *bool, tokenFilter *streamTokenFilter, toolParser *streamToolCallParser) int {
	var outputChars int
	pending := tokenFilter.Flush("")
	if pending != "" {
		text, calls := toolParser.Push(pending)
		outputChars += emitStreamEvent(outCh, streamStart, ttft, gotFirstToken, StreamEvent{
			Content:   text,
			ToolCalls: calls,
		})
	}

	text, calls := toolParser.Flush()
	outputChars += emitStreamEvent(outCh, streamStart, ttft, gotFirstToken, StreamEvent{
		Content:   text,
		ToolCalls: calls,
	})
	return outputChars
}

func emitStreamEvent(outCh chan<- StreamEvent, streamStart time.Time, ttft *time.Duration, gotFirstToken *bool, event StreamEvent) int {
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
