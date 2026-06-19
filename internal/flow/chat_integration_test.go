package flow

import (
	"context"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream"
)

// Named uniquely because chat_test.go already defines package-level mockUpstream.
type integrationMockUpstream struct {
	name   string
	events []upstream.StreamEvent
	err    error
}

func (m *integrationMockUpstream) Name() string { return m.name }

func (m *integrationMockUpstream) Chat(ctx context.Context, token string, req *upstream.ChatRequest) (<-chan upstream.StreamEvent, error) {
	if m.err != nil {
		return nil, m.err
	}
	ch := make(chan upstream.StreamEvent, len(m.events))
	for _, e := range m.events {
		ch <- e
	}
	close(ch)
	return ch, nil
}

func TestConsume_ToolCallAggregation(t *testing.T) {
	up := &integrationMockUpstream{name: "grok", events: []upstream.StreamEvent{
		{Content: `<tool_call>{"name":"f","arguments":{}}</tool_call>`},
	}}
	f := &ChatFlow{cfg: DefaultChatFlowConfig()}
	out := make(chan StreamEvent, 16)
	ch, err := up.Chat(context.Background(), "t", &upstream.ChatRequest{})
	if err != nil {
		t.Fatalf("mock Chat: %v", err)
	}

	success, _, _, _, err := f.consumeUpstreamEvents(context.Background(), ch, out, []Tool{{Type: "function", Function: Function{Name: "f"}}})
	if err != nil || !success {
		t.Fatalf("consume: success=%v err=%v", success, err)
	}
	close(out)

	var calls []ToolCall
	for ev := range out {
		calls = append(calls, ev.ToolCalls...)
	}
	if len(calls) == 0 {
		t.Fatal("expected tool calls aggregated by flow")
	}
	if calls[0].Function.Name != "f" || calls[0].Function.Arguments != "{}" {
		t.Fatalf("tool call = %#v, want function f with empty arguments", calls[0])
	}
}

func TestResolveUpstream_Unknown(t *testing.T) {
	f := &ChatFlow{upstreams: map[string]upstream.Upstream{"grok": &integrationMockUpstream{name: "grok"}}}
	if up, _, err := f.resolveUpstream(&ChatRequest{UpstreamName: "nope"}); up != nil || err == nil {
		t.Fatalf("unknown upstream: up=%v err=%v, want nil upstream and error", up, err)
	}
	if up, _, err := f.resolveUpstream(&ChatRequest{UpstreamName: "console"}); up != nil || err == nil {
		t.Fatalf("unregistered console upstream: up=%v err=%v, want nil upstream and error", up, err)
	}
}
