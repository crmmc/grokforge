package openai

import (
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/flow"
)

type chatStreamChunk struct {
	ID                string             `json:"id"`
	Object            string             `json:"object"`
	Created           int64              `json:"created"`
	Model             string             `json:"model"`
	Choices           []chatStreamChoice `json:"choices"`
	SystemFingerprint string             `json:"system_fingerprint,omitempty"`
}

type chatStreamChoice struct {
	Index        int             `json:"index"`
	Delta        chatStreamDelta `json:"delta"`
	FinishReason *string         `json:"finish_reason,omitempty"`
}

type chatStreamDelta struct {
	Role      string          `json:"role,omitempty"`
	Content   string          `json:"content,omitempty"`
	ToolCalls []flow.ToolCall `json:"tool_calls,omitempty"`
}

type chatStreamAdapter struct {
	id            string
	model         string
	created       int64
	showThink     bool
	toolCallsOn   bool
	tools         []flow.Tool
	nextToolIndex int
	thinkWriter   *thinkStreamWriter
	toolParser    *toolCallStreamParser
	toolCallsSeen bool
}

func newChatStreamAdapter(req *ChatRequest, cfg *config.Config) *chatStreamAdapter {
	showThink := shouldShowThinking(req, cfg)
	toolCallsOn := toolCallsEnabled(req)
	var parser *toolCallStreamParser
	if toolCallsOn {
		parser = newToolCallStreamParser(req.Tools)
	}
	return &chatStreamAdapter{
		id:            generateChatID(),
		model:         req.Model,
		created:       time.Now().Unix(),
		showThink:     showThink,
		toolCallsOn:   toolCallsOn,
		tools:         req.Tools,
		thinkWriter:   newThinkStreamWriter(showThink),
		toolParser:    parser,
		nextToolIndex: defaultToolCallIndexBase,
	}
}

func (a *chatStreamAdapter) RoleChunk() chatStreamChunk {
	delta := chatStreamDelta{Role: "assistant", Content: ""}
	return a.chunk(delta, nil)
}

func (a *chatStreamAdapter) HandleEvent(event flow.StreamEvent) []chatStreamChunk {
	var chunks []chatStreamChunk

	if text := a.thinkWriter.HandleReasoning(event.ReasoningContent); text != "" {
		chunks = append(chunks, a.chunk(chatStreamDelta{Content: text}, nil))
	}

	if event.Content != "" {
		chunks = append(chunks, a.handleContent(event.Content)...)
	}

	if len(event.ToolCalls) > 0 {
		chunks = append(chunks, a.handleToolCalls(event.ToolCalls)...)
	}

	return chunks
}

func (a *chatStreamAdapter) FinishChunks() []chatStreamChunk {
	var chunks []chatStreamChunk
	if tail := a.thinkWriter.Close(); tail != "" {
		chunks = append(chunks, a.chunk(chatStreamDelta{Content: tail}, nil))
	}
	finish := "stop"
	if a.toolCallsSeen {
		finish = "tool_calls"
	}
	chunks = append(chunks, a.chunk(chatStreamDelta{}, &finish))
	return chunks
}

func (a *chatStreamAdapter) handleContent(content string) []chatStreamChunk {
	if !a.toolCallsOn {
		text := a.thinkWriter.HandleText(content)
		if text == "" {
			return nil
		}
		return []chatStreamChunk{a.chunk(chatStreamDelta{Content: text}, nil)}
	}

	texts, calls := a.toolParser.Push(content)
	return a.mergeTextAndCalls(texts, calls)
}

func (a *chatStreamAdapter) handleToolCalls(calls []flow.ToolCall) []chatStreamChunk {
	if !a.toolCallsOn {
		text := a.thinkWriter.HandleText(formatToolCallsAsText(calls))
		if text == "" {
			return nil
		}
		return []chatStreamChunk{a.chunk(chatStreamDelta{Content: text}, nil)}
	}

	filtered := filterToolCalls(calls, a.tools)
	return a.emitToolCalls(filtered)
}

func (a *chatStreamAdapter) mergeTextAndCalls(texts []string, calls []flow.ToolCall) []chatStreamChunk {
	var chunks []chatStreamChunk
	for _, text := range texts {
		chunkText := a.thinkWriter.HandleText(text)
		if chunkText != "" {
			chunks = append(chunks, a.chunk(chatStreamDelta{Content: chunkText}, nil))
		}
	}
	if len(calls) > 0 {
		chunks = append(chunks, a.emitToolCalls(calls)...)
	}
	return chunks
}

func (a *chatStreamAdapter) emitToolCalls(calls []flow.ToolCall) []chatStreamChunk {
	if len(calls) == 0 {
		return nil
	}
	normalized := make([]flow.ToolCall, 0, len(calls))
	for _, call := range calls {
		normalized = append(normalized, a.withIndex(call))
	}
	a.toolCallsSeen = true
	return []chatStreamChunk{
		a.chunk(chatStreamDelta{ToolCalls: normalized}, nil),
	}
}

func (a *chatStreamAdapter) withIndex(call flow.ToolCall) flow.ToolCall {
	if call.Index != nil {
		return call
	}
	idx := a.nextToolIndex
	a.nextToolIndex++
	call.Index = &idx
	return call
}

func (a *chatStreamAdapter) chunk(delta chatStreamDelta, finish *string) chatStreamChunk {
	return chatStreamChunk{
		ID:      a.id,
		Object:  chatChunkObject,
		Created: a.created,
		Model:   a.model,
		Choices: []chatStreamChoice{
			{Index: defaultChoiceIndex, Delta: delta, FinishReason: finish},
		},
	}
}
