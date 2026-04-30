package openai

import (
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/flow"
)

type chatCompletionResponse struct {
	ID            string                  `json:"id"`
	Object        string                  `json:"object"`
	Created       int64                   `json:"created"`
	Model         string                  `json:"model"`
	Choices       []chatCompletionChoice  `json:"choices"`
	Usage         *flow.Usage             `json:"usage,omitempty"`
	SearchSources []flow.SearchSource     `json:"search_sources,omitempty"`
}

type chatCompletionChoice struct {
	Index        int                   `json:"index"`
	Message      chatCompletionMessage `json:"message"`
	FinishReason string                `json:"finish_reason"`
}

type chatCompletionMessage struct {
	Role      string          `json:"role"`
	Content   string          `json:"content"`
	ToolCalls []flow.ToolCall `json:"tool_calls,omitempty"`
}

type chatResponseCollector struct {
	req         *ChatRequest
	showThink   bool
	toolCallsOn bool
	tools       []flow.Tool
	think       *thinkCollector
	toolParser  *toolCallStreamParser
	toolCalls   []flow.ToolCall
	lastUsage   *flow.Usage
	searchSources []flow.SearchSource
}

func newChatResponseCollector(req *ChatRequest, cfg *config.Config) *chatResponseCollector {
	showThink := shouldShowThinking(req, cfg)
	toolCallsOn := toolCallsEnabled(req)
	var parser *toolCallStreamParser
	if toolCallsOn {
		parser = newToolCallStreamParser(req.Tools)
	}
	return &chatResponseCollector{
		req:         req,
		showThink:   showThink,
		toolCallsOn: toolCallsOn,
		tools:       req.Tools,
		think:       newThinkCollector(showThink),
		toolParser:  parser,
	}
}

func (c *chatResponseCollector) AddEvent(event flow.StreamEvent) {
	if event.Usage != nil {
		c.lastUsage = event.Usage
	}

	if len(event.SearchSources) > 0 {
		c.searchSources = event.SearchSources
	}

	if event.ReasoningContent != "" {
		c.think.AddReasoning(event.ReasoningContent)
	}

	if event.Content != "" {
		c.addContent(event.Content)
	}

	if len(event.ToolCalls) > 0 {
		c.addToolCalls(event.ToolCalls)
	}
}

func (c *chatResponseCollector) addContent(content string) {
	if !c.toolCallsOn {
		c.think.AddText(content)
		return
	}

	texts, calls := c.toolParser.Push(content)
	for _, text := range texts {
		c.think.AddText(text)
	}
	if len(calls) > 0 {
		c.toolCalls = append(c.toolCalls, calls...)
	}
}

func (c *chatResponseCollector) addToolCalls(calls []flow.ToolCall) {
	if !c.toolCallsOn {
		c.think.AddText(formatToolCallsAsText(calls))
		return
	}
	filtered := filterToolCalls(calls, c.tools)
	c.toolCalls = append(c.toolCalls, filtered...)
}

func (c *chatResponseCollector) Build() *chatCompletionResponse {
	if c.toolCallsOn && c.toolParser != nil {
		texts, calls := c.toolParser.Flush()
		for _, text := range texts {
			c.think.AddText(text)
		}
		c.toolCalls = append(c.toolCalls, calls...)
	}

	content := c.think.Finalize()
	finish := "stop"
	if len(c.toolCalls) > 0 {
		finish = "tool_calls"
	}

	return &chatCompletionResponse{
		ID:      generateChatID(),
		Object:  chatObject,
		Created: time.Now().Unix(),
		Model:   c.req.Model,
		Choices: []chatCompletionChoice{
			{
				Index: defaultChoiceIndex,
				Message: chatCompletionMessage{
					Role:      "assistant",
					Content:   content,
					ToolCalls: c.toolCalls,
				},
				FinishReason: finish,
			},
		},
		Usage:         c.lastUsage,
		SearchSources: c.searchSources,
	}
}
