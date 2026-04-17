package token

import (
	"testing"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
)

// mockResolver implements ModelResolver for testing.
type mockResolver struct {
	data map[string]string
}

func (m *mockResolver) ResolvePoolFloor(requestName string) (floor string, ok bool) {
	if m == nil {
		return "", false
	}
	floor, found := m.data[requestName]
	if !found {
		return "", false
	}
	return floor, true
}

func newMockResolver(entries map[string]string) *mockResolver {
	m := &mockResolver{data: make(map[string]string)}
	for name, floor := range entries {
		m.data[name] = floor
	}
	return m
}

func TestGetPoolForModel(t *testing.T) {
	resolver := newMockResolver(map[string]string{
		"grok-3":       "basic",
		"grok-3-super": "super",
		"grok-heavy":   "heavy",
	})

	tests := []struct {
		name      string
		model     string
		wantPools []string
		wantOK    bool
	}{
		{
			"basic floor returns all three pools",
			"grok-3",
			[]string{PoolBasic, PoolSuper, PoolHeavy},
			true,
		},
		{
			"super floor returns super and heavy",
			"grok-3-super",
			[]string{PoolSuper, PoolHeavy},
			true,
		},
		{
			"heavy floor returns only heavy",
			"grok-heavy",
			[]string{PoolHeavy},
			true,
		},
		{
			"unknown model returns nil false",
			"unknown-model",
			nil,
			false,
		},
		{
			"empty model returns nil false",
			"",
			nil,
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pools, ok := GetPoolForModel(tt.model, resolver)
			if ok != tt.wantOK {
				t.Errorf("GetPoolForModel(%q) ok = %v, want %v", tt.model, ok, tt.wantOK)
				return
			}
			if !ok {
				if pools != nil {
					t.Errorf("GetPoolForModel(%q) pools = %v, want nil", tt.model, pools)
				}
				return
			}
			if len(pools) != len(tt.wantPools) {
				t.Errorf("GetPoolForModel(%q) = %v, want %v", tt.model, pools, tt.wantPools)
				return
			}
			for i, p := range pools {
				if p != tt.wantPools[i] {
					t.Errorf("GetPoolForModel(%q)[%d] = %q, want %q", tt.model, i, p, tt.wantPools[i])
				}
			}
		})
	}
}

func TestPickForModel_ThreePool(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	resolver := newMockResolver(map[string]string{
		"grok-basic": "basic",
		"grok-super": "super",
	})

	t.Run("picks from first available pool in order", func(t *testing.T) {
		m := NewTokenManager(cfg)
		basicToken := &store.Token{ID: 1, Token: "basic-tok", Pool: PoolBasic, Status: string(StatusActive), ChatQuota: 80}
		superToken := &store.Token{ID: 2, Token: "super-tok", Pool: PoolSuper, Status: string(StatusActive), ChatQuota: 140}
		m.AddToken(basicToken)
		m.AddToken(superToken)

		// basic floor model should pick from basic pool first
		tok, err := m.PickForModel("grok-basic", resolver, CategoryChat)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tok.Pool != PoolBasic {
			t.Errorf("expected pool %q, got %q", PoolBasic, tok.Pool)
		}
	})

	t.Run("falls back to next pool when first is empty", func(t *testing.T) {
		m := NewTokenManager(cfg)
		// Only super token, no basic token
		superToken := &store.Token{ID: 2, Token: "super-tok", Pool: PoolSuper, Status: string(StatusActive), ChatQuota: 140}
		m.AddToken(superToken)

		// basic floor model: basic pool empty -> try super pool
		tok, err := m.PickForModel("grok-basic", resolver, CategoryChat)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tok.Pool != PoolSuper {
			t.Errorf("expected pool %q, got %q", PoolSuper, tok.Pool)
		}
	})

	t.Run("all pools empty returns ErrNoTokenAvailable", func(t *testing.T) {
		m := NewTokenManager(cfg)
		// No tokens at all

		_, err := m.PickForModel("grok-basic", resolver, CategoryChat)
		if err != ErrNoTokenAvailable {
			t.Errorf("expected ErrNoTokenAvailable, got %v", err)
		}
	})

	t.Run("unknown model returns ErrModelNotFound", func(t *testing.T) {
		m := NewTokenManager(cfg)
		_, err := m.PickForModel("unknown", resolver, CategoryChat)
		if err != ErrModelNotFound {
			t.Errorf("expected ErrModelNotFound, got %v", err)
		}
	})

	t.Run("super floor skips basic pool", func(t *testing.T) {
		m := NewTokenManager(cfg)
		basicToken := &store.Token{ID: 1, Token: "basic-tok", Pool: PoolBasic, Status: string(StatusActive), ChatQuota: 80}
		superToken := &store.Token{ID: 2, Token: "super-tok", Pool: PoolSuper, Status: string(StatusActive), ChatQuota: 140}
		m.AddToken(basicToken)
		m.AddToken(superToken)

		// super floor model should NOT pick from basic pool
		tok, err := m.PickForModel("grok-super", resolver, CategoryChat)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if tok.Pool != PoolSuper {
			t.Errorf("expected pool %q, got %q", PoolSuper, tok.Pool)
		}
	})

	t.Run("super floor with only basic token returns error", func(t *testing.T) {
		m := NewTokenManager(cfg)
		basicToken := &store.Token{ID: 1, Token: "basic-tok", Pool: PoolBasic, Status: string(StatusActive), ChatQuota: 80}
		m.AddToken(basicToken)

		// super floor model: only basic pool has tokens, but super floor can't use basic
		_, err := m.PickForModel("grok-super", resolver, CategoryChat)
		if err != ErrNoTokenAvailable {
			t.Errorf("expected ErrNoTokenAvailable, got %v", err)
		}
	})
}
