package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/store"
)

func TestHandleSystemStatus_APIKeys(t *testing.T) {
	ctx := context.Background()
	ts := newMockTokenStore()
	ts.CreateToken(ctx, &store.Token{Status: "active"})
	ts.CreateToken(ctx, &store.Token{Status: "active"})
	ts.CreateToken(ctx, &store.Token{Status: "disabled"})

	aks := newMockAPIKeyStore()
	// Add keys with specific statuses to get total=3, active=2
	for i := 0; i < 7; i++ {
		aks.Create(ctx, &store.APIKey{Name: "a", Status: "active"})
	}
	for i := 0; i < 3; i++ {
		aks.Create(ctx, &store.APIKey{Name: "i", Status: "inactive"})
	}

	handler := handleSystemStatus(ts, aks, time.Now(), "1.0.0")

	req := httptest.NewRequest(http.MethodGet, "/system/status", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp SystemStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.APIKeys.Total != 10 {
		t.Errorf("expected api_keys.total=10, got %d", resp.APIKeys.Total)
	}
	if resp.APIKeys.Active != 7 {
		t.Errorf("expected api_keys.active=7, got %d", resp.APIKeys.Active)
	}
	if resp.Tokens.Total != 3 {
		t.Errorf("expected tokens.total=3, got %d", resp.Tokens.Total)
	}
	if resp.Tokens.Active != 2 {
		t.Errorf("expected tokens.active=2, got %d", resp.Tokens.Active)
	}
}

func TestHandleSystemStatus_NilAPIKeyStore(t *testing.T) {
	ts := newMockTokenStore()

	handler := handleSystemStatus(ts, nil, time.Now(), "1.0.0")

	req := httptest.NewRequest(http.MethodGet, "/system/status", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp SystemStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// With nil apiKeyStore, should return zeros (no panic)
	if resp.APIKeys.Total != 0 {
		t.Errorf("expected api_keys.total=0, got %d", resp.APIKeys.Total)
	}
	if resp.APIKeys.Active != 0 {
		t.Errorf("expected api_keys.active=0, got %d", resp.APIKeys.Active)
	}
}
