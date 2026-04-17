package token

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
)

func TestQuota_Consume(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	m := NewTokenManager(cfg)

	token := &store.Token{
		ID:        1,
		Token:     "test-token",
		Pool:      PoolBasic,
		Status:    string(StatusActive),
		ChatQuota: 10,
	}
	m.AddToken(token)

	t.Run("deducts quota and returns remaining", func(t *testing.T) {
		remaining, err := m.Consume(1, CategoryChat, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if remaining != 9 {
			t.Errorf("expected remaining=9, got %d", remaining)
		}

		// Verify token state updated
		tok := m.GetToken(1)
		if tok.ChatQuota != 9 {
			t.Errorf("expected token.ChatQuota=9, got %d", tok.ChatQuota)
		}
	})

	t.Run("marks token dirty", func(t *testing.T) {
		// Consume again
		m.Consume(1, CategoryChat, 1)
		dirty := m.GetDirtyTokens()
		found := false
		for _, d := range dirty {
			if d.ID == 1 {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected token to be marked dirty")
		}
	})

	t.Run("returns error when quota is zero", func(t *testing.T) {
		// Set quota to 0
		tok := m.GetToken(1)
		tok.ChatQuota = 0

		_, err := m.Consume(1, CategoryChat, 1)
		if err != ErrNoQuota {
			t.Errorf("expected ErrNoQuota, got %v", err)
		}
	})

	t.Run("marks token cooling when quota reaches zero", func(t *testing.T) {
		tok := m.GetToken(1)
		tok.ChatQuota = 1
		tok.ImageQuota = 0
		tok.VideoQuota = 0
		tok.Status = string(StatusActive)
		tok.CoolUntil = nil

		remaining, err := m.Consume(1, CategoryChat, 1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if remaining != 0 {
			t.Fatalf("expected remaining=0, got %d", remaining)
		}
		if tok.Status != string(StatusCooling) {
			t.Errorf("expected status=cooling, got %s", tok.Status)
		}
		if tok.CoolUntil == nil {
			t.Error("expected CoolUntil to be set")
		}
	})

	t.Run("returns error for non-existent token", func(t *testing.T) {
		_, err := m.Consume(999, CategoryChat, 1)
		if err != ErrTokenNotFound {
			t.Errorf("expected ErrTokenNotFound, got %v", err)
		}
	})
}

func TestQuota_SyncQuota(t *testing.T) {
	cfg := &config.TokenConfig{FailThreshold: 3}
	m := NewTokenManager(cfg)

	t.Run("updates quota from API response", func(t *testing.T) {
		// Mock server
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
			if payload["modelName"] != rateLimitsProbeModeName {
				t.Fatalf("expected modelName=%q, got %#v", rateLimitsProbeModeName, payload["modelName"])
			}
			if _, ok := payload["requestKind"]; ok {
				t.Fatalf("unexpected legacy requestKind in payload: %#v", payload)
			}
			resp := RateLimitsResponse{
				RemainingQueries:  50,
				WindowSizeSeconds: 7200,
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		token := &store.Token{
			ID:        2,
			Token:     "test-token-2",
			Pool:      PoolBasic,
			Status:    string(StatusActive),
			ChatQuota: 10,
		}
		m.AddToken(token)

		ctx := context.Background()
		err := m.SyncQuota(ctx, token.ID, token.Token, server.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		tok := m.GetToken(2)
		if tok.ChatQuota != 50 {
			t.Errorf("expected ChatQuota=50, got %d", tok.ChatQuota)
		}
		if tok.InitialChatQuota != 50 {
			t.Errorf("expected InitialChatQuota=50, got %d", tok.InitialChatQuota)
		}
	})

	t.Run("restores cooling token to active when quota recovered", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := RateLimitsResponse{
				RemainingQueries:  30,
				WindowSizeSeconds: 7200,
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		coolUntil := time.Now().Add(-1 * time.Minute) // already expired
		token := &store.Token{
			ID:        3,
			Token:     "test-token-3",
			Pool:      PoolBasic,
			Status:    string(StatusCooling),
			ChatQuota: 0,
			CoolUntil: &coolUntil,
		}
		m.AddToken(token)

		ctx := context.Background()
		err := m.SyncQuota(ctx, token.ID, token.Token, server.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		tok := m.GetToken(3)
		if tok.Status != string(StatusActive) {
			t.Errorf("expected status=active, got %s", tok.Status)
		}
		if tok.CoolUntil != nil {
			t.Error("expected CoolUntil to be nil")
		}
	})

	t.Run("sets active token to cooling when quota is zero", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := RateLimitsResponse{
				RemainingQueries:  0,
				WindowSizeSeconds: 7200,
			}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		token := &store.Token{
			ID:        10,
			Token:     "test-token-zombie",
			Pool:      PoolBasic,
			Status:    string(StatusActive),
			ChatQuota: 5,
		}
		m.AddToken(token)

		ctx := context.Background()
		err := m.SyncQuota(ctx, token.ID, token.Token, server.URL)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		tok := m.GetToken(10)
		if tok.ChatQuota != 0 {
			t.Errorf("expected ChatQuota=0, got %d", tok.ChatQuota)
		}
		if tok.Status != string(StatusCooling) {
			t.Errorf("expected status=cooling, got %s", tok.Status)
		}
		if tok.CoolUntil == nil {
			t.Error("expected CoolUntil to be set")
		}
	})

	t.Run("marks token dirty after sync", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := RateLimitsResponse{RemainingQueries: 20}
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		token := &store.Token{
			ID:        4,
			Token:     "test-token-4",
			Pool:      PoolBasic,
			Status:    string(StatusActive),
			ChatQuota: 10,
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
		m.SyncQuota(ctx, token.ID, token.Token, server.URL)

		dirty := m.GetDirtyTokens()
		found := false
		for _, d := range dirty {
			if d.ID == 4 {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected token to be marked dirty after sync")
		}
	})
}

func TestSyncQuota_RestoresImageVideoQuotas(t *testing.T) {
	cfg := &config.TokenConfig{
		FailThreshold:     3,
		DefaultImageQuota: 10,
		DefaultVideoQuota: 5,
	}
	m := NewTokenManager(cfg)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := RateLimitsResponse{RemainingQueries: 50}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	token := &store.Token{
		ID:         20,
		Token:      "test-sync-img-vid",
		Pool:       PoolBasic,
		Status:     string(StatusActive),
		ChatQuota:  0,
		ImageQuota: 0,
		VideoQuota: 0,
	}
	m.AddToken(token)

	err := m.SyncQuota(context.Background(), token.ID, token.Token, server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tok := m.GetToken(20)
	if tok.ChatQuota != 50 {
		t.Errorf("expected ChatQuota=50, got %d", tok.ChatQuota)
	}
	if tok.InitialChatQuota != 50 {
		t.Errorf("expected InitialChatQuota=50, got %d", tok.InitialChatQuota)
	}
	if tok.ImageQuota != 10 {
		t.Errorf("expected ImageQuota=10, got %d", tok.ImageQuota)
	}
	if tok.InitialImageQuota != 10 {
		t.Errorf("expected InitialImageQuota=10, got %d", tok.InitialImageQuota)
	}
	if tok.VideoQuota != 5 {
		t.Errorf("expected VideoQuota=5, got %d", tok.VideoQuota)
	}
	if tok.InitialVideoQuota != 5 {
		t.Errorf("expected InitialVideoQuota=5, got %d", tok.InitialVideoQuota)
	}
}
