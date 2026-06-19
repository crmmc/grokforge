package msgutil

import (
	"strings"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream"
)

func TestBuildToolPrompt_Empty(t *testing.T) {
	prompt := BuildToolPrompt(nil, "auto", false)
	if prompt != "" {
		t.Errorf("expected empty prompt for nil tools, got %q", prompt)
	}

	prompt = BuildToolPrompt([]upstream.Tool{}, "auto", false)
	if prompt != "" {
		t.Errorf("expected empty prompt for empty tools, got %q", prompt)
	}
}

func TestBuildToolPrompt_None(t *testing.T) {
	tools := []upstream.Tool{
		{Type: "function", Function: upstream.Function{Name: "get_weather", Description: "Get weather"}},
	}
	prompt := BuildToolPrompt(tools, "none", false)
	if prompt != "" {
		t.Errorf("expected empty prompt for tool_choice=none, got %q", prompt)
	}
}

func TestBuildToolPrompt_Auto(t *testing.T) {
	tools := []upstream.Tool{
		{
			Type: "function",
			Function: upstream.Function{
				Name:        "get_weather",
				Description: "Get current weather",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{"type": "string"},
					},
				},
			},
		},
	}

	prompt := BuildToolPrompt(tools, "auto", false)

	if !strings.Contains(prompt, "<tools>") || !strings.Contains(prompt, "</tools>") {
		t.Error("prompt should wrap definitions in <tools> XML tags")
	}
	if !strings.Contains(prompt, "get_weather") {
		t.Error("prompt should contain function name")
	}
	if !strings.Contains(prompt, "Get current weather") {
		t.Error("prompt should contain function description")
	}
	if !strings.Contains(prompt, "<tool_call>") {
		t.Error("prompt should contain tool_call format instruction")
	}
}

func TestBuildToolPrompt_Required(t *testing.T) {
	tools := []upstream.Tool{
		{Type: "function", Function: upstream.Function{Name: "search", Description: "Search"}},
	}

	prompt := BuildToolPrompt(tools, "required", false)

	if !strings.Contains(strings.ToLower(prompt), "must") {
		t.Error("required tool_choice should contain MUST instruction")
	}
}

func TestBuildToolPrompt_SpecificFunction(t *testing.T) {
	tools := []upstream.Tool{
		{Type: "function", Function: upstream.Function{Name: "get_weather"}},
		{Type: "function", Function: upstream.Function{Name: "search"}},
	}

	choice := map[string]any{
		"type": "function",
		"function": map[string]any{
			"name": "get_weather",
		},
	}

	prompt := BuildToolPrompt(tools, choice, false)

	if !strings.Contains(prompt, "get_weather") {
		t.Error("prompt should mention specific function")
	}
}

func TestBuildToolPrompt_ParallelCalls(t *testing.T) {
	tools := []upstream.Tool{
		{Type: "function", Function: upstream.Function{Name: "func1"}},
	}

	prompt := BuildToolPrompt(tools, "auto", true)
	if strings.Contains(strings.ToLower(prompt), "only make one") {
		t.Error("parallel calls enabled should not restrict to one call")
	}

	prompt = BuildToolPrompt(tools, "auto", false)
	if !strings.Contains(strings.ToLower(prompt), "only make one") {
		t.Error("parallel calls disabled should mention one call restriction")
	}
}

func TestBuildToolPrompt_NoMarkdownLayout(t *testing.T) {
	tools := []upstream.Tool{
		{Type: "function", Function: upstream.Function{Name: "search", Description: "search docs"}},
	}
	prompt := BuildToolPrompt(tools, "auto", true)
	if strings.Contains(prompt, "# ") || strings.Contains(prompt, "## ") {
		t.Fatalf("prompt should avoid markdown headings, got %q", prompt)
	}
}
