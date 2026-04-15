package token

import (
	"testing"
)

func TestPoolHeavyConstant(t *testing.T) {
	if PoolHeavy != "ssoHeavy" {
		t.Errorf("PoolHeavy = %q, want %q", PoolHeavy, "ssoHeavy")
	}
}

func TestPoolLevelConstants(t *testing.T) {
	tests := []struct {
		name  string
		level PoolLevel
		want  PoolLevel
	}{
		{"basic", PoolLevelBasic, 1},
		{"super", PoolLevelSuper, 2},
		{"heavy", PoolLevelHeavy, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.level != tt.want {
				t.Errorf("PoolLevel%s = %d, want %d", tt.name, tt.level, tt.want)
			}
		})
	}
}

func TestPoolLevelFor(t *testing.T) {
	tests := []struct {
		input string
		want  PoolLevel
	}{
		// Full pool names
		{"ssoBasic", PoolLevelBasic},
		{"ssoSuper", PoolLevelSuper},
		{"ssoHeavy", PoolLevelHeavy},
		// Short floor names
		{"basic", PoolLevelBasic},
		{"super", PoolLevelSuper},
		{"heavy", PoolLevelHeavy},
		// Unknown
		{"unknown", 0},
		{"", 0},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := PoolLevelFor(tt.input)
			if got != tt.want {
				t.Errorf("PoolLevelFor(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestPoolNameForLevel(t *testing.T) {
	tests := []struct {
		level PoolLevel
		want  string
	}{
		{PoolLevelBasic, "ssoBasic"},
		{PoolLevelSuper, "ssoSuper"},
		{PoolLevelHeavy, "ssoHeavy"},
		{0, ""},
		{99, ""},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := PoolNameForLevel(tt.level)
			if got != tt.want {
				t.Errorf("PoolNameForLevel(%d) = %q, want %q", tt.level, got, tt.want)
			}
		})
	}
}

func TestAllPoolNames(t *testing.T) {
	names := AllPoolNames()
	want := []string{"ssoBasic", "ssoSuper", "ssoHeavy"}
	if len(names) != len(want) {
		t.Fatalf("AllPoolNames() len = %d, want %d", len(names), len(want))
	}
	for i, name := range names {
		if name != want[i] {
			t.Errorf("AllPoolNames()[%d] = %q, want %q", i, name, want[i])
		}
	}
}
