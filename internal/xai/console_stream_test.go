package xai

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseConsoleSSEStream_WrapsEventAndJSONData(t *testing.T) {
	input := "event: response.output_text.delta\n" +
		"data: {\"delta\":\"Hel\"}\n\n" +
		"event: response.output_text.delta\n" +
		"data: {\"delta\":\"lo\"}\n\n" +
		"data: [DONE]\n\n"
	events := make(chan StreamEvent, 10)
	c := &client{}

	if err := c.parseConsoleSSEStream(strings.NewReader(input), events); err != nil {
		t.Fatalf("parseConsoleSSEStream() error = %v", err)
	}
	close(events)

	var deltas []string
	for ev := range events {
		var wrapped struct {
			Event string `json:"event"`
			Data  struct {
				Delta string `json:"delta"`
			} `json:"data"`
		}
		if err := json.Unmarshal(ev.Data, &wrapped); err != nil {
			t.Fatalf("unmarshal wrapped event: %v", err)
		}
		if wrapped.Event != "response.output_text.delta" {
			t.Fatalf("event = %q, want response.output_text.delta", wrapped.Event)
		}
		deltas = append(deltas, wrapped.Data.Delta)
	}
	if got := strings.Join(deltas, ""); got != "Hello" {
		t.Fatalf("deltas = %q, want Hello", got)
	}
}

func TestParseConsoleSSEStream_MultilineStringData(t *testing.T) {
	input := "event: error\n" +
		"data: first line\n" +
		"data: second line\n\n"
	events := make(chan StreamEvent, 10)
	c := &client{}

	if err := c.parseConsoleSSEStream(strings.NewReader(input), events); err != nil {
		t.Fatalf("parseConsoleSSEStream() error = %v", err)
	}
	close(events)

	ev, ok := <-events
	if !ok {
		t.Fatal("expected one event")
	}
	var wrapped struct {
		Event string `json:"event"`
		Data  string `json:"data"`
	}
	if err := json.Unmarshal(ev.Data, &wrapped); err != nil {
		t.Fatalf("unmarshal wrapped event: %v", err)
	}
	if wrapped.Event != "error" || wrapped.Data != "first line\nsecond line" {
		t.Fatalf("wrapped = %#v, want multiline error string", wrapped)
	}
}
