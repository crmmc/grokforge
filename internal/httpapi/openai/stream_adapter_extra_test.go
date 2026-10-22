package openai

import (
	"testing"

	"github.com/crmmc/grokforge/internal/flow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func toolReq() *ChatRequest {
	return &ChatRequest{
		Model: "grok-3",
		Tools: []flow.Tool{{Type: "function", Function: flow.Function{Name: "known_tool"}}},
	}
}

func TestNewChatStreamAdapter_ToolCallsOn(t *testing.T) {
	a := newChatStreamAdapter(toolReq(), nil)
	assert.True(t, a.toolCallsOn)
	require.NotNil(t, a.toolParser)
	assert.False(t, a.toolCallsSeen)
	assert.Equal(t, defaultToolCallIndexBase, a.nextToolIndex)
}

func TestNewChatStreamAdapter_ToolCallsOff(t *testing.T) {
	a := newChatStreamAdapter(&ChatRequest{Model: "grok-3"}, nil)
	assert.False(t, a.toolCallsOn)
	assert.Nil(t, a.toolParser)
}

func TestStreamAdapter_HandleEvent_SearchSources(t *testing.T) {
	a := newChatStreamAdapter(&ChatRequest{Model: "grok-3"}, nil)
	sources := []flow.SearchSource{{Title: "result", URL: "https://example.com"}}
	chunks := a.HandleEvent(flow.StreamEvent{SearchSources: sources})
	assert.Empty(t, chunks, "search sources alone emit no content chunks")

	finish := a.FinishChunks()
	require.Len(t, finish, 1)
	assert.Equal(t, sources, finish[0].SearchSources)
}

func TestStreamAdapter_HandleEvent_ToolCallsOff_TextAndCallsAsText(t *testing.T) {
	a := newChatStreamAdapter(&ChatRequest{Model: "grok-3"}, nil)

	chunks := a.HandleEvent(flow.StreamEvent{Content: "hello"})
	require.Len(t, chunks, 1)
	assert.Equal(t, "hello", chunks[0].Choices[0].Delta.Content)

	chunks = a.HandleEvent(flow.StreamEvent{
		ToolCalls: []flow.ToolCall{{Function: flow.FunctionCall{Name: "lookup", Arguments: `{"q":"x"}`}}},
	})
	require.Len(t, chunks, 1)
	assert.Contains(t, chunks[0].Choices[0].Delta.Content, `"name":"lookup"`)

	finish := a.FinishChunks()
	require.NotEmpty(t, finish)
	assert.Equal(t, "stop", *finish[len(finish)-1].Choices[0].FinishReason)
}

func TestStreamAdapter_HandleEvent_ToolCallsOn_Text(t *testing.T) {
	a := newChatStreamAdapter(toolReq(), nil)
	chunks := a.HandleEvent(flow.StreamEvent{Content: "plain text"})
	require.Len(t, chunks, 1)
	assert.Equal(t, "plain text", chunks[0].Choices[0].Delta.Content)
	assert.Empty(t, chunks[0].Choices[0].Delta.ToolCalls)
}

func TestStreamAdapter_HandleEvent_ToolCallsOn_ToolCallBlockInContent(t *testing.T) {
	a := newChatStreamAdapter(toolReq(), nil)
	chunks := a.HandleEvent(flow.StreamEvent{
		Content: `answer <tool_call>{"name":"known_tool","arguments":{"q":"x"}}</tool_call>`,
	})
	// text chunk + tool call chunk
	require.Len(t, chunks, 2)
	assert.Equal(t, "answer ", chunks[0].Choices[0].Delta.Content)
	require.Len(t, chunks[1].Choices[0].Delta.ToolCalls, 1)
	assert.Equal(t, "known_tool", chunks[1].Choices[0].Delta.ToolCalls[0].Function.Name)
	require.NotNil(t, chunks[1].Choices[0].Delta.ToolCalls[0].Index)
	assert.Equal(t, 0, *chunks[1].Choices[0].Delta.ToolCalls[0].Index)
	assert.True(t, a.toolCallsSeen)
}

func TestStreamAdapter_HandleEvent_StructuredToolCalls_Filtered(t *testing.T) {
	a := newChatStreamAdapter(toolReq(), nil)
	idx := 7
	chunks := a.HandleEvent(flow.StreamEvent{ToolCalls: []flow.ToolCall{
		{Function: flow.FunctionCall{Name: "known_tool", Arguments: "{}"}},
		{Function: flow.FunctionCall{Name: "unknown_tool", Arguments: "{}"}},
		{Index: &idx, Function: flow.FunctionCall{Name: "known_tool", Arguments: "{}"}},
	}})
	require.Len(t, chunks, 1)
	calls := chunks[0].Choices[0].Delta.ToolCalls
	require.Len(t, calls, 2)
	// Call without index gets nextToolIndex assigned.
	assert.Equal(t, 0, *calls[0].Index)
	// Call with an existing index keeps it.
	assert.Equal(t, 7, *calls[1].Index)
	assert.Equal(t, 1, a.nextToolIndex)
}

func TestStreamAdapter_HandleEvent_StructuredToolCalls_AllFilteredOut(t *testing.T) {
	a := newChatStreamAdapter(toolReq(), nil)
	chunks := a.HandleEvent(flow.StreamEvent{ToolCalls: []flow.ToolCall{
		{Function: flow.FunctionCall{Name: "unknown_tool", Arguments: "{}"}},
	}})
	assert.Empty(t, chunks)
	assert.False(t, a.toolCallsSeen)
}

func TestStreamAdapter_ReasoningContent(t *testing.T) {
	a := newChatStreamAdapter(&ChatRequest{Model: "grok-3", ReasoningEffort: "high"}, nil)
	chunks := a.HandleEvent(flow.StreamEvent{ReasoningContent: "thinking"})
	require.Len(t, chunks, 1)
	assert.Equal(t, thinkOpenTag+"thinking", chunks[0].Choices[0].Delta.Content)

	chunks = a.HandleEvent(flow.StreamEvent{Content: "answer"})
	require.Len(t, chunks, 1)
	assert.Equal(t, thinkCloseTag+"answer", chunks[0].Choices[0].Delta.Content)

	finish := a.FinishChunks()
	assert.Equal(t, "stop", *finish[len(finish)-1].Choices[0].FinishReason)
	assert.Len(t, finish, 1)
}

func TestStreamAdapter_FinishChunks_ToolCallsSeen(t *testing.T) {
	a := newChatStreamAdapter(toolReq(), nil)
	a.HandleEvent(flow.StreamEvent{ToolCalls: []flow.ToolCall{
		{Function: flow.FunctionCall{Name: "known_tool", Arguments: "{}"}},
	}})
	finish := a.FinishChunks()
	require.Len(t, finish, 1)
	assert.Equal(t, "tool_calls", *finish[0].Choices[0].FinishReason)
	assert.Empty(t, finish[0].Choices[0].Delta.Content)
}

func TestStreamAdapter_WithIndex(t *testing.T) {
	a := newChatStreamAdapter(&ChatRequest{Model: "grok-3"}, nil)
	idx := 3
	call := flow.ToolCall{Index: &idx}
	assert.Equal(t, idx, *a.withIndex(call).Index)

	assigned := a.withIndex(flow.ToolCall{})
	require.NotNil(t, assigned.Index)
	assert.Equal(t, 0, *assigned.Index)
	assert.Equal(t, 1, a.nextToolIndex)
}

func TestStreamAdapter_RoleChunk(t *testing.T) {
	a := newChatStreamAdapter(&ChatRequest{Model: "grok-3"}, nil)
	chunk := a.RoleChunk()
	assert.Equal(t, "assistant", chunk.Choices[0].Delta.Role)
	assert.Equal(t, chatChunkObject, chunk.Object)
	assert.Equal(t, "grok-3", chunk.Model)
	assert.Equal(t, 0, chunk.Choices[0].Index)
	assert.Nil(t, chunk.Choices[0].FinishReason)
}

func TestStreamAdapter_HandleEvent_ToolCallsOff_EmptyNameCallRendersNothing(t *testing.T) {
	a := newChatStreamAdapter(&ChatRequest{Model: "grok-3"}, nil)
	chunks := a.HandleEvent(flow.StreamEvent{ToolCalls: []flow.ToolCall{
		{Function: flow.FunctionCall{Name: "", Arguments: "{}"}},
	}})
	assert.Empty(t, chunks)
}
