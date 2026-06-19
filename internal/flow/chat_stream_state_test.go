package flow

import (
	"context"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/store"
	"github.com/crmmc/grokforge/internal/upstream"
)

func TestChatFlow_FilterTagsAcrossChunks(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	grokUp := &mockUpstream{
		name: "grok",
		events: []upstream.StreamEvent{
			{Content: `before<xaiarti`},
			{Content: `fact>secret</xaiarti`},
			{Content: `fact>after`},
		},
	}
	flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": grokUp}, &ChatFlowConfig{
		RetryConfig: DefaultRetryConfig(),
		ModelResolver: testModelResolver(),
		FilterTags:  []string{"xaiartifact"},
	})

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "test"}},
		Model:    "grok-2",
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	var content strings.Builder
	for event := range ch {
		content.WriteString(event.Content)
	}
	if content.String() != "beforeafter" {
		t.Fatalf("unexpected filtered content: %q", content.String())
	}
}

func TestChatFlow_ToolCallsAcrossChunks(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	grokUp := &mockUpstream{
		name: "grok",
		events: []upstream.StreamEvent{
			{Content: `I'll check.<tool_`},
			{Content: `call>{"name":"get_weather","arguments":{"location":"Tokyo"}}`},
			{Content: `</tool_call>done`},
		},
	}
	flow := NewChatFlow(tokenSvc, map[string]upstream.Upstream{"grok": grokUp}, &ChatFlowConfig{
		RetryConfig: DefaultRetryConfig(),
		ModelResolver: testModelResolver(),
	})

	ch, err := flow.Complete(context.Background(), &ChatRequest{
		Messages: []Message{{Role: "user", Content: "weather"}},
		Model:    "grok-2",
		Tools: []Tool{{
			Type: "function",
			Function: Function{
				Name:       "get_weather",
				Parameters: map[string]any{"type": "object"},
			},
		}},
		ToolChoice: "auto",
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}

	var content strings.Builder
	var found []ToolCall
	for event := range ch {
		content.WriteString(event.Content)
		if len(event.ToolCalls) > 0 {
			found = append(found, event.ToolCalls...)
		}
	}
	if content.String() != "I'll check.done" {
		t.Fatalf("unexpected streamed content: %q", content.String())
	}
	if len(found) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(found))
	}
	if found[0].Function.Name != "get_weather" {
		t.Fatalf("unexpected tool name: %q", found[0].Function.Name)
	}
	if found[0].Function.Arguments != `{"location":"Tokyo"}` {
		t.Fatalf("unexpected tool arguments: %q", found[0].Function.Arguments)
	}
}
