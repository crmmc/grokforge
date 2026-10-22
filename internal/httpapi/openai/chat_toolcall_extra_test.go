package openai

import (
	"testing"

	"github.com/crmmc/grokforge/internal/flow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testTools() []flow.Tool {
	return []flow.Tool{
		{Type: "function", Function: flow.Function{Name: "known_tool"}},
	}
}

func TestToolCallStreamParser_PushEmptyChunk(t *testing.T) {
	parser := newToolCallStreamParser(testTools())
	texts, calls := parser.Push("")
	assert.Nil(t, texts)
	assert.Nil(t, calls)
}

func TestToolCallStreamParser_TextWithPrefixCarry(t *testing.T) {
	parser := newToolCallStreamParser(testTools())

	texts, calls := parser.Push("hello <tool")
	require.Nil(t, calls)
	require.Len(t, texts, 1)
	assert.Equal(t, "hello ", texts[0])

	// Flush releases the carried partial tag as plain text.
	texts, calls = parser.Flush()
	require.Nil(t, calls)
	require.Len(t, texts, 1)
	assert.Equal(t, "<tool", texts[0])

	// Flushing an empty text state yields nothing.
	texts, calls = parser.Flush()
	assert.Nil(t, texts)
	assert.Nil(t, calls)
}

func TestToolCallStreamParser_TextBeforeTag(t *testing.T) {
	parser := newToolCallStreamParser(testTools())
	texts, calls := parser.Push(`intro <tool_call>{"name":"known_tool","arguments":{"q":"x"}}</tool_call> tail`)
	require.Len(t, calls, 1)
	assert.Equal(t, "known_tool", calls[0].Function.Name)
	require.Len(t, texts, 2)
	assert.Equal(t, "intro ", texts[0])
	assert.Equal(t, " tail", texts[1])
}

func TestToolCallStreamParser_EmptyBlockYieldsNothing(t *testing.T) {
	parser := newToolCallStreamParser(testTools())
	texts, calls := parser.Push(`<tool_call></tool_call>`)
	assert.Nil(t, texts)
	assert.Empty(t, calls)

	// Buffered empty tool state flushes to nothing.
	texts, calls = parser.Flush()
	assert.Nil(t, texts)
	assert.Nil(t, calls)
}

func TestToolCallStreamParser_FlushValidIncompleteBlock(t *testing.T) {
	parser := newToolCallStreamParser(testTools())
	texts, calls := parser.Push(`<tool_call>{"name":"known_tool","arguments":{"q":1}}`)
	require.Nil(t, texts)
	require.Empty(t, calls)

	texts, calls = parser.Flush()
	require.Nil(t, texts)
	require.Len(t, calls, 1)
	assert.Equal(t, "known_tool", calls[0].Function.Name)
	assert.Equal(t, 0, *calls[0].Index)
}

func TestToolCallStreamParser_FlushInvalidBlockFallsBackToText(t *testing.T) {
	parser := newToolCallStreamParser(testTools())
	raw := `<tool_call>{"name":"mystery_tool"}`
	_, _ = parser.Push(raw)

	texts, calls := parser.Flush()
	require.Nil(t, calls)
	require.Len(t, texts, 1)
	assert.Equal(t, raw, texts[0])
}

func TestToolCallStreamParser_FlushEmptyToolState(t *testing.T) {
	parser := newToolCallStreamParser(testTools())
	_, _ = parser.Push(`<tool_call>`)

	texts, calls := parser.Flush()
	assert.Nil(t, texts)
	assert.Nil(t, calls)
}

func TestToolCallStreamParser_MultipleCallsIncrementIndex(t *testing.T) {
	parser := newToolCallStreamParser(testTools())
	chunk := `<tool_call>{"name":"known_tool"}</tool_call><tool_call>{"name":"known_tool"}</tool_call>`
	texts, calls := parser.Push(chunk)
	require.Nil(t, texts)
	require.Len(t, calls, 2)
	require.NotNil(t, calls[0].Index)
	require.NotNil(t, calls[1].Index)
	assert.Equal(t, 0, *calls[0].Index)
	assert.Equal(t, 1, *calls[1].Index)
}
