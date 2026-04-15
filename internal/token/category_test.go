package token

import (
	"testing"
)

func TestParseModelEntry(t *testing.T) {
	tests := []struct {
		entry    string
		wantName string
		wantCost int
	}{
		{"grok-4", "grok-4", 1},
		{"grok-4-heavy#4", "grok-4-heavy", 4},
		{"grok-4.1-expert#4", "grok-4.1-expert", 4},
		{"grok-2#1", "grok-2", 1},
		{"grok-2#0", "grok-2#0", 1},       // cost=0 invalid, treat as name
		{"grok-2#-1", "grok-2#-1", 1},     // negative invalid
		{"grok-2#abc", "grok-2#abc", 1},   // non-numeric
		{"#4", "#4", 1},                    // no name before #
		{"model#10", "model", 10},
		{"a#b#3", "a#b", 3},               // last # wins
	}

	for _, tt := range tests {
		t.Run(tt.entry, func(t *testing.T) {
			name, cost := ParseModelEntry(tt.entry)
			if name != tt.wantName || cost != tt.wantCost {
				t.Errorf("ParseModelEntry(%q) = (%q, %d), want (%q, %d)",
					tt.entry, name, cost, tt.wantName, tt.wantCost)
			}
		})
	}
}

func TestCostForModel(t *testing.T) {
	resolver := newMockResolver(map[string][2]any{
		"grok-2":       {"basic", 1},
		"grok-3":       {"super", 1},
		"grok-expert":  {"heavy", 4},
	})

	tests := []struct {
		model    string
		wantCost int
	}{
		{"grok-2", 1},
		{"grok-3", 1},
		{"grok-expert", 4},
		{"unknown", 1},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			cost := CostForModel(tt.model, resolver)
			if cost != tt.wantCost {
				t.Errorf("CostForModel(%q) = %d, want %d", tt.model, cost, tt.wantCost)
			}
		})
	}

	t.Run("nil resolver returns 1", func(t *testing.T) {
		if cost := CostForModel("any", nil); cost != 1 {
			t.Errorf("CostForModel with nil resolver = %d, want 1", cost)
		}
	})
}
