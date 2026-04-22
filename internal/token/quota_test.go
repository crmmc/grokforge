package token

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
)

func TestSyncModeQuota_UpdatesQuotaFromAPI(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	m := NewTokenManager(cfg)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/rate-limits" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if payload["modelName"] != "auto" {
			t.Fatalf("expected modelName=%q, got %#v", "auto", payload["modelName"])
		}
		resp := RateLimitsResponse{
			RemainingQueries:  50,
			TotalQueries:      80,
			WindowSizeSeconds: 7200,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	token := &store.Token{
		ID:     2,
		Token:  "test-token-2",
		Pool:   PoolBasic,
		Status: string(StatusActive),
		Quotas: store.IntMap{"auto": 10},
	}
	m.AddToken(token)

	ctx := context.Background()
	resp, err := m.SyncModeQuota(ctx, token.ID, token.Token, server.URL, "auto")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.RemainingQueries != 50 {
		t.Errorf("expected RemainingQueries=50, got %d", resp.RemainingQueries)
	}
	if resp.TotalQueries != 80 {
		t.Errorf("expected TotalQueries=80, got %d", resp.TotalQueries)
	}
}

func TestSyncModeQuota_MarksDirtyAfterUpdate(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	m := NewTokenManager(cfg)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RateLimitsResponse{RemainingQueries: 20, TotalQueries: 40}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	token := &store.Token{
		ID:     4,
		Token:  "test-token-4",
		Pool:   PoolBasic,
		Status: string(StatusActive),
		Quotas: store.IntMap{"auto": 10},
	}
	m.AddToken(token)

	// Clear dirty set first
	d := m.GetDirtyTokens()
	clearIDs := make([]uint, len(d))
	for i, s := range d {
		clearIDs[i] = s.ID
	}
	m.ClearDirty(clearIDs)

	ctx := context.Background()
	_, err := m.SyncModeQuota(ctx, token.ID, token.Token, server.URL, "auto")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// After SyncModeQuota, caller (scheduler) calls UpdateModeQuota which marks dirty
	m.UpdateModeQuota(token.ID, "auto", 20, 40)

	dirty := m.GetDirtyTokens()
	found := false
	for _, d := range dirty {
		if d.ID == 4 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected token to be marked dirty after UpdateModeQuota")
	}
}

func TestUpdateModeQuota_SetsQuotaAndLimit(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	m := NewTokenManager(cfg)

	token := &store.Token{
		ID:          20,
		Token:       "test-update-mode",
		Pool:        PoolBasic,
		Status:      string(StatusActive),
		Quotas:      store.IntMap{"auto": 0},
		LimitQuotas: store.IntMap{"auto": 0},
	}
	m.AddToken(token)

	m.UpdateModeQuota(20, "auto", 50, 80)

	tok := m.GetToken(20)
	if tok.Quotas["auto"] != 50 {
		t.Errorf("expected Quotas[auto]=50, got %d", tok.Quotas["auto"])
	}
	if tok.LimitQuotas["auto"] != 80 {
		t.Errorf("expected LimitQuotas[auto]=80, got %d", tok.LimitQuotas["auto"])
	}
}

func TestUpdateModeQuota_MultipleModesIndependent(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	m := NewTokenManager(cfg)

	token := &store.Token{
		ID:          21,
		Token:       "test-multi-mode",
		Pool:        PoolBasic,
		Status:      string(StatusActive),
		Quotas:      store.IntMap{"auto": 10, "fast": 5},
		LimitQuotas: store.IntMap{"auto": 20, "fast": 10},
	}
	m.AddToken(token)

	// Update only "auto" mode
	m.UpdateModeQuota(21, "auto", 50, 80)

	tok := m.GetToken(21)
	if tok.Quotas["auto"] != 50 {
		t.Errorf("expected Quotas[auto]=50, got %d", tok.Quotas["auto"])
	}
	// "fast" should be unchanged
	if tok.Quotas["fast"] != 5 {
		t.Errorf("expected Quotas[fast]=5 (unchanged), got %d", tok.Quotas["fast"])
	}
}
