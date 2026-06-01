package flow

import (
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/config"
)

func TestBuildConsoleRequest_MapsInstructionsAndContent(t *testing.T) {
	temp := 0.2
	maxTokens := 64
	flow := NewChatFlow(nil, nil, &ChatFlowConfig{
		RetryConfig: DefaultRetryConfig(),
		AppConfig:   &config.AppConfig{CustomInstruction: "app instruction"},
	})

	built, err := flow.buildConsoleRequest(&ChatRequest{
		Model:         "public-model",
		UpstreamModel: "grok-4.20",
		Messages: []Message{
			{Role: "system", Content: "system instruction"},
			{Role: "developer", Content: []any{map[string]any{"type": "text", "text": "developer instruction"}}},
			{Role: "user", Content: []any{
				map[string]any{"type": "text", "text": "look"},
				map[string]any{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/image.png"}},
				map[string]any{"type": "input_audio", "audio": map[string]any{"id": "a1"}},
			}},
			{Role: "assistant", Content: "previous answer"},
		},
		Temperature:                    &temp,
		MaxTokens:                      &maxTokens,
		ForceThinking:                  true,
		ConsoleSupportsReasoningEffort: true,
		ConsoleWebSearch:               true,
	})
	if err != nil {
		t.Fatalf("buildConsoleRequest() error = %v", err)
	}

	if built.Model != "grok-4.20" {
		t.Fatalf("Model = %q, want grok-4.20", built.Model)
	}
	for _, want := range []string{"app instruction", "system instruction", "developer instruction"} {
		if !strings.Contains(built.Instructions, want) {
			t.Fatalf("Instructions %q missing %q", built.Instructions, want)
		}
	}
	if built.Temperature != &temp || built.MaxTokens != &maxTokens {
		t.Fatalf("sampling/max token pointers not propagated")
	}
	if built.ReasoningEffort != "high" || !built.SupportsReasoningEffort || !built.WebSearch {
		t.Fatalf("console route flags not propagated: %#v", built)
	}
	if len(built.Input) != 2 {
		t.Fatalf("Input length = %d, want 2", len(built.Input))
	}
	user := built.Input[0]
	if user.Role != "user" || len(user.Content) != 3 {
		t.Fatalf("user input = %#v, want three content blocks", user)
	}
	if user.Content[0].Type != "input_text" || user.Content[0].Text != "look" {
		t.Fatalf("first user block = %#v, want input_text look", user.Content[0])
	}
	if user.Content[1].Type != "input_image" || user.Content[1].ImageURL != "https://example.com/image.png" {
		t.Fatalf("second user block = %#v, want input_image", user.Content[1])
	}
	if user.Content[2].Type != "input_text" || !strings.Contains(user.Content[2].Text, "input_audio") {
		t.Fatalf("third user block = %#v, want audio JSON fallback", user.Content[2])
	}
	assistant := built.Input[1]
	if assistant.Role != "assistant" || len(assistant.Content) != 1 || assistant.Content[0].Type != "output_text" || assistant.Content[0].Text != "previous answer" {
		t.Fatalf("assistant input = %#v, want output_text", assistant)
	}
}

func TestBuildConsoleRequest_ExplicitReasoningOverridesForceThinking(t *testing.T) {
	flow := NewChatFlow(nil, nil, &ChatFlowConfig{RetryConfig: DefaultRetryConfig()})
	built, err := flow.buildConsoleRequest(&ChatRequest{
		Model:           "grok-4.20",
		Messages:        []Message{{Role: "user", Content: "hi"}},
		ReasoningEffort: "low",
		ForceThinking:   true,
	})
	if err != nil {
		t.Fatalf("buildConsoleRequest() error = %v", err)
	}
	if built.ReasoningEffort != "low" {
		t.Fatalf("ReasoningEffort = %q, want low", built.ReasoningEffort)
	}
}
