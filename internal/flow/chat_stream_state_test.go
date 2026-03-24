package flow

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/store"
	"github.com/crmmc/grokforge/internal/xai"
)

func TestChatFlow_FilterTagsAcrossChunks(t *testing.T) {
	tokenSvc := &mockTokenService{
		tokens: []*store.Token{{ID: 1, Token: "tok1", Pool: "basic"}},
	}
	client := &mockXAIClient{
		events: []xai.StreamEvent{
			streamTokenEvent(`before<xaiarti`),
			streamTokenEvent(`fact>secret</xaiarti`),
			streamTokenEvent(`fact>after`),
		},
	}
	flow := NewChatFlow(tokenSvc, func(token string) xai.Client { return client }, &ChatFlowConfig{
		RetryConfig: DefaultRetryConfig(),
		TokenConfig: testFlowTokenConfig(),
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
	client := &mockXAIClient{
		events: []xai.StreamEvent{
			streamTokenEvent(`I'll check.<tool_`),
			streamTokenEvent(`call>{"name":"get_weather","arguments":{"location":"Tokyo"}}`),
			streamTokenEvent(`</tool_call>done`),
		},
	}
	flow := NewChatFlow(tokenSvc, func(token string) xai.Client { return client }, &ChatFlowConfig{
		RetryConfig: DefaultRetryConfig(),
		TokenConfig: testFlowTokenConfig(),
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

func streamTokenEvent(token string) xai.StreamEvent {
	payload, err := json.Marshal(token)
	if err != nil {
		panic(err)
	}
	data := `{"result":{"response":{"token":` + string(payload) + `,"isThinking":false}}}`
	return xai.StreamEvent{Data: json.RawMessage(data)}
}
