package openai

import (
	"testing"

	"github.com/crmmc/grokforge/internal/flow"
)

func TestToolCallStreamParser_InvalidBlockFallsBackToText(t *testing.T) {
	parser := newToolCallStreamParser([]flow.Tool{
		{Type: "function", Function: flow.Function{Name: "known_tool"}},
	})
	chunk := `<tool_call>{"name":"unknown_tool","arguments":{"q":"x"}}</tool_call>`

	texts, calls := parser.Push(chunk)
	if len(calls) != 0 {
		t.Fatalf("expected 0 tool calls, got %d", len(calls))
	}
	if len(texts) != 1 {
		t.Fatalf("expected 1 text chunk fallback, got %d", len(texts))
	}
	if texts[0] != chunk {
		t.Fatalf("invalid block should be preserved as text, got %q", texts[0])
	}
}

func TestToolCallStreamParser_PartialValidBlock(t *testing.T) {
	parser := newToolCallStreamParser([]flow.Tool{
		{Type: "function", Function: flow.Function{Name: "known_tool"}},
	})

	texts1, calls1 := parser.Push(`<tool_call>{"name":"known_tool","arguments":{"q":"x"}}`)
	if len(texts1) != 0 || len(calls1) != 0 {
		t.Fatalf("expected no output for partial block, got texts=%d calls=%d", len(texts1), len(calls1))
	}

	texts2, calls2 := parser.Push(`</tool_call>`)
	if len(texts2) != 0 {
		t.Fatalf("expected no text output when block completes, got %d chunks", len(texts2))
	}
	if len(calls2) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(calls2))
	}
	if calls2[0].Function.Name != "known_tool" {
		t.Fatalf("unexpected tool name: %q", calls2[0].Function.Name)
	}
}
