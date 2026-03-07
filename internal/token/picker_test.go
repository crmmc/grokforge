package token

import (
	"testing"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
)

// testTokenConfig returns a config fixture for picker tests.
func testTokenConfig() *config.TokenConfig {
	return &config.TokenConfig{
		BasicModels:   []string{"grok-2", "grok-2-mini", "grok-2-imageGen"},
		SuperModels:   []string{"grok-3", "grok-3-mini", "grok-3-reasoning", "grok-3-deepsearch", "grok-4"},
		PreferredPool: "ssoSuper",
		FailThreshold: 3,
	}
}

func TestGetPoolForModel_ConfigDriven(t *testing.T) {
	cfg := testTokenConfig()

	tests := []struct {
		name     string
		model    string
		wantPool string
		wantOK   bool
	}{
		{"basic only model", "grok-2", PoolBasic, true},
		{"basic only model mini", "grok-2-mini", PoolBasic, true},
		{"super only model", "grok-3", PoolSuper, true},
		{"super only model mini", "grok-3-mini", PoolSuper, true},
		{"super only model grok-4", "grok-4", PoolSuper, true},
		{"unknown model", "unknown-model", "", false},
		{"empty model", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, ok := GetPoolForModel(tt.model, cfg)
			if pool != tt.wantPool || ok != tt.wantOK {
				t.Errorf("GetPoolForModel(%q) = (%q, %v), want (%q, %v)",
					tt.model, pool, ok, tt.wantPool, tt.wantOK)
			}
		})
	}
}

func TestGetPoolForModel_DualMembership(t *testing.T) {
	// Model in both groups: preferred pool wins
	cfg := &config.TokenConfig{
		BasicModels:   []string{"grok-2", "shared-model"},
		SuperModels:   []string{"grok-3", "shared-model"},
		PreferredPool: "ssoSuper",
	}

	pool, ok := GetPoolForModel("shared-model", cfg)
	if !ok || pool != "ssoSuper" {
		t.Errorf("dual membership with PreferredPool=ssoSuper: got (%q, %v), want (ssoSuper, true)", pool, ok)
	}

	// Change preferred pool
	cfg.PreferredPool = "ssoBasic"
	pool, ok = GetPoolForModel("shared-model", cfg)
	if !ok || pool != "ssoBasic" {
		t.Errorf("dual membership with PreferredPool=ssoBasic: got (%q, %v), want (ssoBasic, true)", pool, ok)
	}
}

func TestPickForModel(t *testing.T) {
	cfg := testTokenConfig()
	m := NewTokenManager(cfg)

	// Add tokens to both pools
	basicToken := &store.Token{ID: 1, Token: "basic-token", Pool: PoolBasic, Status: string(StatusActive), ChatQuota: 80}
	superToken := &store.Token{ID: 2, Token: "super-token", Pool: PoolSuper, Status: string(StatusActive), ChatQuota: 140}
	m.AddToken(basicToken)
	m.AddToken(superToken)

	t.Run("grok-2 picks from basic pool", func(t *testing.T) {
		token, err := m.PickForModel("grok-2", cfg, CategoryChat)
		if err != nil {
			t.Fatalf("PickForModel failed: %v", err)
		}
		if token.Pool != PoolBasic {
			t.Errorf("expected pool %q, got %q", PoolBasic, token.Pool)
		}
	})

	t.Run("grok-3 picks from super pool", func(t *testing.T) {
		token, err := m.PickForModel("grok-3", cfg, CategoryChat)
		if err != nil {
			t.Fatalf("PickForModel failed: %v", err)
		}
		if token.Pool != PoolSuper {
			t.Errorf("expected pool %q, got %q", PoolSuper, token.Pool)
		}
	})

	t.Run("unknown model returns ErrModelNotFound", func(t *testing.T) {
		_, err := m.PickForModel("unknown", cfg, CategoryChat)
		if err != ErrModelNotFound {
			t.Errorf("expected ErrModelNotFound, got %v", err)
		}
	})
}

func TestPickForModel_EmptyPool(t *testing.T) {
	cfg := testTokenConfig()
	m := NewTokenManager(cfg)

	// Only add basic token, no super token
	basicToken := &store.Token{ID: 1, Token: "basic-token", Pool: PoolBasic, Status: string(StatusActive), ChatQuota: 80}
	m.AddToken(basicToken)

	t.Run("returns error when pool is empty and no fallback", func(t *testing.T) {
		_, err := m.PickForModel("grok-3", cfg, CategoryChat) // grok-3 only in super pool, no fallback
		if err != ErrNoTokenAvailable {
			t.Errorf("expected ErrNoTokenAvailable, got %v", err)
		}
	})
}

func TestPickForModel_Fallback(t *testing.T) {
	// Model in both pools, preferred pool empty → should fallback to other pool
	cfg := &config.TokenConfig{
		BasicModels:   []string{"shared-model"},
		SuperModels:   []string{"shared-model"},
		PreferredPool: "ssoSuper",
		FailThreshold: 3,
	}
	m := NewTokenManager(cfg)

	// Only add basic token, no super token
	basicToken := &store.Token{ID: 1, Token: "basic-token", Pool: PoolBasic, Status: string(StatusActive), ChatQuota: 80}
	m.AddToken(basicToken)

	t.Run("falls back to basic when super is empty", func(t *testing.T) {
		token, err := m.PickForModel("shared-model", cfg, CategoryChat)
		if err != nil {
			t.Fatalf("expected fallback to basic, got error: %v", err)
		}
		if token.Pool != PoolBasic {
			t.Errorf("expected pool %q, got %q", PoolBasic, token.Pool)
		}
	})

	// Reverse: preferred=basic, only super token
	cfg2 := &config.TokenConfig{
		BasicModels:   []string{"shared-model"},
		SuperModels:   []string{"shared-model"},
		PreferredPool: "ssoBasic",
		FailThreshold: 3,
	}
	m2 := NewTokenManager(cfg2)
	superToken := &store.Token{ID: 2, Token: "super-token", Pool: PoolSuper, Status: string(StatusActive), ChatQuota: 140}
	m2.AddToken(superToken)

	t.Run("falls back to super when basic is empty", func(t *testing.T) {
		token, err := m2.PickForModel("shared-model", cfg2, CategoryChat)
		if err != nil {
			t.Fatalf("expected fallback to super, got error: %v", err)
		}
		if token.Pool != PoolSuper {
			t.Errorf("expected pool %q, got %q", PoolSuper, token.Pool)
		}
	})
}

func TestGetPoolForModel_WithCostSuffix(t *testing.T) {
	cfg := &config.TokenConfig{
		BasicModels:   []string{"grok-2", "grok-2-mini"},
		SuperModels:   []string{"grok-3", "grok-4-heavy#4"},
		PreferredPool: "ssoSuper",
	}

	t.Run("model with cost suffix matches", func(t *testing.T) {
		pool, ok := GetPoolForModel("grok-4-heavy", cfg)
		if !ok || pool != PoolSuper {
			t.Errorf("GetPoolForModel(grok-4-heavy) = (%q, %v), want (ssoSuper, true)", pool, ok)
		}
	})

	t.Run("model without cost suffix still matches", func(t *testing.T) {
		pool, ok := GetPoolForModel("grok-3", cfg)
		if !ok || pool != PoolSuper {
			t.Errorf("GetPoolForModel(grok-3) = (%q, %v), want (ssoSuper, true)", pool, ok)
		}
	})

	t.Run("literal cost suffix does not match", func(t *testing.T) {
		// Searching for "grok-4-heavy#4" as model name should NOT match
		_, ok := GetPoolForModel("grok-4-heavy#4", cfg)
		if ok {
			t.Error("GetPoolForModel(grok-4-heavy#4) should not match")
		}
	})
}
