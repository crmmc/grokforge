package grok

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream"
)

func TestBuildBody_BasicChat(t *testing.T) {
	g := New("", nil, Options{})
	body, err := g.buildBody(&upstream.ChatRequest{
		Messages:     []upstream.Message{{Role: "user", Content: "hello"}},
		Model:        "grok-3",
		UpstreamMode: "auto",
	}, nil)
	if err != nil {
		t.Fatalf("buildBody: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if payload["message"] != "hello" {
		t.Errorf("message=%v", payload["message"])
	}
	if payload["modeId"] != "auto" {
		t.Errorf("modeId=%v", payload["modeId"])
	}
	if _, ok := payload["fileAttachments"]; !ok {
		t.Error("missing fileAttachments")
	}
}

func TestBuildBody_RequiresMode(t *testing.T) {
	g := New("", nil, Options{})
	if _, err := g.buildBody(&upstream.ChatRequest{
		Messages: []upstream.Message{{Role: "user", Content: "x"}},
		Model:    "grok-3",
	}, nil); err == nil {
		t.Error("expected error for empty UpstreamMode")
	}
}

func TestBuildBody_InsertsToolPromptIntoSystemMessage(t *testing.T) {
	g := New("", nil, Options{})
	body, err := g.buildBody(&upstream.ChatRequest{
		Messages: []upstream.Message{
			{Role: "system", Content: "You are helpful."},
			{Role: "user", Content: "Hello"},
		},
		Model:        "grok-3",
		UpstreamMode: "auto",
		Tools: []upstream.Tool{
			{
				Type: "function",
				Function: upstream.Function{
					Name:        "get_weather",
					Description: "Get weather",
					Parameters:  map[string]any{"type": "object"},
				},
			},
		},
		ParallelToolCalls: true,
	}, nil)
	if err != nil {
		t.Fatalf("buildBody: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	message, ok := payload["message"].(string)
	if !ok {
		t.Fatalf("message has type %T, want string", payload["message"])
	}
	for _, want := range []string{"get_weather", "You are helpful.", "Hello"} {
		if !strings.Contains(message, want) {
			t.Fatalf("message %q missing %q", message, want)
		}
	}
}
