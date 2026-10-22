package openai

import (
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/flow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewChatResponseCollector_Flags(t *testing.T) {
	c := newChatResponseCollector(toolReq(), nil)
	assert.True(t, c.toolCallsOn)
	require.NotNil(t, c.toolParser)

	c = newChatResponseCollector(&ChatRequest{Model: "grok-3"}, nil)
	assert.False(t, c.toolCallsOn)
	assert.Nil(t, c.toolParser)
}

func TestCollector_AddEvent_UsageAndSources(t *testing.T) {
	c := newChatResponseCollector(&ChatRequest{Model: "grok-3"}, nil)

	c.AddEvent(flow.StreamEvent{Usage: &flow.Usage{PromptTokens: 1, TotalTokens: 1}})
	c.AddEvent(flow.StreamEvent{Usage: &flow.Usage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5}})
	c.AddEvent(flow.StreamEvent{SearchSources: []flow.SearchSource{{URL: "https://example.com"}}})
	c.AddEvent(flow.StreamEvent{Content: "hi"})

	resp := c.Build()
	require.NotNil(t, resp.Usage)
	assert.Equal(t, 5, resp.Usage.TotalTokens)
	require.Len(t, resp.SearchSources, 1)
	assert.Equal(t, "chat.completion", resp.Object)
	assert.Equal(t, "stop", resp.Choices[0].FinishReason)
	assert.Equal(t, "grok-3", resp.Model)
	assert.True(t, strings.HasPrefix(resp.ID, "chatcmpl-"))
}

func TestCollector_AddEvent_ReasoningHiddenAndShown(t *testing.T) {
	hidden := newChatResponseCollector(&ChatRequest{Model: "grok-3"}, nil)
	hidden.AddEvent(flow.StreamEvent{ReasoningContent: "secret"})
	assert.Equal(t, "", hidden.Build().Choices[0].Message.Content)

	shown := newChatResponseCollector(&ChatRequest{Model: "grok-3", ReasoningEffort: "low"}, nil)
	shown.AddEvent(flow.StreamEvent{ReasoningContent: "thought"})
	assert.Equal(t, thinkOpenTag+"thought"+thinkCloseTag, shown.Build().Choices[0].Message.Content)
}

func TestCollector_AddContent_ToolCallsOn_ParsesBlocks(t *testing.T) {
	c := newChatResponseCollector(toolReq(), nil)
	c.AddEvent(flow.StreamEvent{
		Content: `<tool_call>{"name":"known_tool","arguments":{"q":"x"}}</tool_call> tail`,
	})
	resp := c.Build()
	assert.Equal(t, " tail", resp.Choices[0].Message.Content)
	require.Len(t, resp.Choices[0].Message.ToolCalls, 1)
	assert.Equal(t, "known_tool", resp.Choices[0].Message.ToolCalls[0].Function.Name)
	assert.Equal(t, "tool_calls", resp.Choices[0].FinishReason)
}

func TestCollector_AddContent_ToolCallsOff(t *testing.T) {
	c := newChatResponseCollector(&ChatRequest{Model: "grok-3"}, nil)
	c.AddEvent(flow.StreamEvent{Content: "plain"})
	resp := c.Build()
	assert.Equal(t, "plain", resp.Choices[0].Message.Content)
	assert.Equal(t, "stop", resp.Choices[0].FinishReason)
}

func TestCollector_AddToolCalls_ToolCallsOff_RendersText(t *testing.T) {
	c := newChatResponseCollector(&ChatRequest{Model: "grok-3"}, nil)
	c.AddEvent(flow.StreamEvent{ToolCalls: []flow.ToolCall{
		{Function: flow.FunctionCall{Name: "lookup", Arguments: `{"q":"x"}`}},
	}})
	resp := c.Build()
	assert.Contains(t, resp.Choices[0].Message.Content, `"name":"lookup"`)
	assert.Empty(t, resp.Choices[0].Message.ToolCalls)
}

func TestCollector_AddToolCalls_ToolCallsOn_Filters(t *testing.T) {
	c := newChatResponseCollector(toolReq(), nil)
	c.AddEvent(flow.StreamEvent{ToolCalls: []flow.ToolCall{
		{Function: flow.FunctionCall{Name: "known_tool", Arguments: "{}"}},
		{Function: flow.FunctionCall{Name: "unknown_tool", Arguments: "{}"}},
	}})
	resp := c.Build()
	require.Len(t, resp.Choices[0].Message.ToolCalls, 1)
	assert.Equal(t, "known_tool", resp.Choices[0].Message.ToolCalls[0].Function.Name)
}

func TestCollector_Build_FlushesPartialToolBlock(t *testing.T) {
	c := newChatResponseCollector(toolReq(), nil)
	c.AddEvent(flow.StreamEvent{Content: `<tool_call>{"name":"known_tool","arguments":{"q":1}}`})
	resp := c.Build()
	require.Len(t, resp.Choices[0].Message.ToolCalls, 1)
	assert.Equal(t, "known_tool", resp.Choices[0].Message.ToolCalls[0].Function.Name)
	assert.Equal(t, "", resp.Choices[0].Message.Content)
	assert.Equal(t, "tool_calls", resp.Choices[0].FinishReason)
}

func TestCollector_Build_FlushesPartialText(t *testing.T) {
	c := newChatResponseCollector(toolReq(), nil)
	c.AddEvent(flow.StreamEvent{Content: `trailing <tool`})
	resp := c.Build()
	assert.Equal(t, "trailing <tool", resp.Choices[0].Message.Content)
	assert.Empty(t, resp.Choices[0].Message.ToolCalls)
}

func TestCollector_Build_ChoiceShape(t *testing.T) {
	c := newChatResponseCollector(&ChatRequest{Model: "grok-3"}, nil)
	resp := c.Build()
	require.Len(t, resp.Choices, 1)
	assert.Equal(t, defaultChoiceIndex, resp.Choices[0].Index)
	assert.Equal(t, "assistant", resp.Choices[0].Message.Role)
	assert.Empty(t, resp.Choices[0].Message.ToolCalls)
	assert.Nil(t, resp.Usage)
}
