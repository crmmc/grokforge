package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/config"
)

// TestAdminAPI_IntegrationFlow tests the full admin API flow:
// auth → config → tokens → batch operations
func TestAdminAPI_IntegrationFlow(t *testing.T) {
	// Setup server with all dependencies
	cfg := &config.Config{
		App: config.AppConfig{
			Port: 8080,
		},
	}
	mockStore := newMockTokenStore()

	srv := NewServer(&ServerConfig{
		AppKey:     "test-app-key",
		Config:     cfg,
		TokenStore: mockStore,
	})

	router := srv.Router()

	// Test 1: Verify auth is required
	t.Run("auth_required", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/config", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 without auth, got %d", rec.Code)
		}
	})

	// Test 2: Get config with valid auth
	t.Run("get_config", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/config", nil)
		req.Header.Set("Authorization", "Bearer test-app-key")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// Test 3: Create tokens via batch import
	t.Run("batch_import_tokens", func(t *testing.T) {
		body := `{"operation":"import","tokens":["tok1_long_enough_for_test","tok2_long_enough_for_test","tok3_long_enough_for_test"],"pool":"main"}`
		req := httptest.NewRequest(http.MethodPost, "/admin/tokens/batch", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer test-app-key")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp BatchTokenResponse
		json.NewDecoder(rec.Body).Decode(&resp)
		if resp.Success != 3 {
			t.Errorf("expected 3 imports, got %d", resp.Success)
		}
	})

	// Test 4: List tokens
	t.Run("list_tokens", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/tokens", nil)
		req.Header.Set("Authorization", "Bearer test-app-key")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var listResp PaginatedTokenResponse
		json.NewDecoder(rec.Body).Decode(&listResp)
		if len(listResp.Data) != 3 {
			t.Errorf("expected 3 tokens, got %d", len(listResp.Data))
		}
	})

	// Test 5: Get single token
	t.Run("get_token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/tokens/1", nil)
		req.Header.Set("Authorization", "Bearer test-app-key")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// Test 6: Update token
	t.Run("update_token", func(t *testing.T) {
		body := `{"status":"disabled"}`
		req := httptest.NewRequest(http.MethodPut, "/admin/tokens/1", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer test-app-key")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	// Test 7: Export tokens (raw)
	t.Run("batch_export_raw", func(t *testing.T) {
		body := `{"operation":"export"}`
		req := httptest.NewRequest(http.MethodPost, "/admin/tokens/batch?raw=true", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer test-app-key")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp BatchTokenResponse
		json.NewDecoder(rec.Body).Decode(&resp)
		if len(resp.RawTokens) != 3 {
			t.Errorf("expected 3 raw tokens, got %d", len(resp.RawTokens))
		}
	})

	// Test 8: Batch delete
	t.Run("batch_delete", func(t *testing.T) {
		body := `{"operation":"delete","ids":[1,2]}`
		req := httptest.NewRequest(http.MethodPost, "/admin/tokens/batch", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer test-app-key")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp BatchTokenResponse
		json.NewDecoder(rec.Body).Decode(&resp)
		if resp.Success != 2 {
			t.Errorf("expected 2 deletions, got %d", resp.Success)
		}
	})

	// Test 9: Verify remaining tokens
	t.Run("verify_remaining", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/tokens", nil)
		req.Header.Set("Authorization", "Bearer test-app-key")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		var listResp PaginatedTokenResponse
		json.NewDecoder(rec.Body).Decode(&listResp)
		if len(listResp.Data) != 1 {
			t.Errorf("expected 1 token remaining, got %d", len(listResp.Data))
		}
	})
}
