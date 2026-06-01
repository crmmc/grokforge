package xai

import (
	"encoding/json"
	"testing"
)

func TestBuildConsoleBody_ReasoningAndWebSearch(t *testing.T) {
	temp := 0.7
	topP := 0.9
	maxTokens := 128
	body, err := buildConsoleBody(&ConsoleRequest{
		Model: "grok-4.20",
		Input: []ConsoleInputItem{{
			Role:    "user",
			Content: []ConsoleContent{{Type: "input_text", Text: "hello"}},
		}},
		Instructions:            "be concise",
		Temperature:             &temp,
		TopP:                    &topP,
		MaxTokens:               &maxTokens,
		ReasoningEffort:         "xhigh",
		SupportsReasoningEffort: true,
		WebSearch:               true,
	})
	if err != nil {
		t.Fatalf("buildConsoleBody() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if payload["model"] != "grok-4.20" {
		t.Fatalf("model = %v, want grok-4.20", payload["model"])
	}
	if payload["instructions"] != "be concise" {
		t.Fatalf("instructions = %v, want be concise", payload["instructions"])
	}
	if payload["stream"] != true {
		t.Fatalf("stream = %v, want true", payload["stream"])
	}
	if payload["temperature"] != temp || payload["top_p"] != topP || payload["max_output_tokens"] != float64(maxTokens) {
		t.Fatalf("sampling/max token fields not propagated: %#v", payload)
	}
	reasoning, ok := payload["reasoning"].(map[string]any)
	if !ok || reasoning["effort"] != "high" {
		t.Fatalf("reasoning = %#v, want effort high", payload["reasoning"])
	}
	tools, ok := payload["tools"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("tools = %#v, want one web_search tool", payload["tools"])
	}
	tool, ok := tools[0].(map[string]any)
	if !ok || tool["type"] != "web_search" {
		t.Fatalf("tool = %#v, want web_search", tools[0])
	}
}

func TestBuildConsoleBody_OmitsUnsupportedReasoningAndWebSearch(t *testing.T) {
	body, err := buildConsoleBody(&ConsoleRequest{
		Model:           "grok-4.20",
		Input:           []ConsoleInputItem{{Role: "user", Content: []ConsoleContent{{Type: "input_text", Text: "hello"}}}},
		ReasoningEffort: "high",
	})
	if err != nil {
		t.Fatalf("buildConsoleBody() error = %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if _, ok := payload["reasoning"]; ok {
		t.Fatalf("reasoning should be omitted when unsupported: %#v", payload["reasoning"])
	}
	if _, ok := payload["tools"]; ok {
		t.Fatalf("tools should be omitted when web search disabled: %#v", payload["tools"])
	}
}
