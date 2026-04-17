package tokencount

import "testing"

func TestEstimateTokens(t *testing.T) {
	if got := EstimateTokens("hello world"); got <= 0 {
		t.Fatalf("EstimateTokens() = %d, want > 0", got)
	}
}

func TestEstimatePromptTokens(t *testing.T) {
	base := EstimateTokens("hello world")
	got := EstimatePromptTokens("hello world")
	if got <= base {
		t.Fatalf("EstimatePromptTokens() = %d, want > %d", got, base)
	}
}

func TestEstimateTokens_WithStructuredValue(t *testing.T) {
	value := map[string]any{
		"messages": []map[string]any{
			{"role": "user", "content": "hello"},
			{"role": "assistant", "content": "world"},
		},
	}
	if got := EstimateTokens(value); got <= 0 {
		t.Fatalf("EstimateTokens(structured) = %d, want > 0", got)
	}
}
