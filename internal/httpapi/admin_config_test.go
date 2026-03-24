package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/config"
)

func TestAdminConfig_GetConfig(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			AppKey:   "app-secret-key-12345",
			Host:     "0.0.0.0",
			Port:     8080,
			LogLevel: "info",
		},
		Retry: config.RetryConfig{
			MaxTokens: 3,
		},
		Token: config.TokenConfig{
			FailThreshold: 5,
		},
	}

	handler := handleGetConfig(cfg)
	req := httptest.NewRequest(http.MethodGet, "/admin/config", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp ConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// App key should be fully masked (never expose chars)
	if resp.App.AppKey != "********" {
		t.Errorf("app_key not masked correctly, got %s", resp.App.AppKey)
	}
	// Non-secrets should be visible
	if resp.App.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %s", resp.App.Host)
	}
	if resp.Retry.MaxTokens != 3 {
		t.Errorf("expected max_tokens 3, got %d", resp.Retry.MaxTokens)
	}
}

func TestAdminConfig_PutConfig_HotReloadable(t *testing.T) {
	cfg := &config.Config{
		Retry: config.RetryConfig{
			MaxTokens: 3,
		},
		Token: config.TokenConfig{
			FailThreshold: 5,
		},
	}

	handler := handlePutConfig(cfg, nil)

	body := `{"retry": {"max_tokens": 6}, "token": {"fail_threshold": 10}}`
	req := httptest.NewRequest(http.MethodPut, "/admin/config", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify config was updated
	if cfg.Retry.MaxTokens != 6 {
		t.Errorf("expected max_tokens 6, got %d", cfg.Retry.MaxTokens)
	}
	if cfg.Token.FailThreshold != 10 {
		t.Errorf("expected fail_threshold 10, got %d", cfg.Token.FailThreshold)
	}
}

func TestAdminConfig_PutConfig_IgnoresUnknownFields(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
	}

	handler := handlePutConfig(cfg, nil)

	// Unknown fields in JSON are silently ignored
	body := `{"app": {"host": "127.0.0.1"}, "retry": {"max_tokens": 5}}`
	req := httptest.NewRequest(http.MethodPut, "/admin/config", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should succeed (unknown "app" field is ignored, retry is applied)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	// App config should NOT have changed (not in ConfigUpdateRequest)
	if cfg.App.Host != "0.0.0.0" {
		t.Errorf("host should not have changed, got %s", cfg.App.Host)
	}
	// Retry should have been updated
	if cfg.Retry.MaxTokens != 5 {
		t.Errorf("expected max_tokens 5, got %d", cfg.Retry.MaxTokens)
	}
}

func TestAdminConfig_PutConfig_InvalidJSON(t *testing.T) {
	cfg := &config.Config{}

	handler := handlePutConfig(cfg, nil)

	body := `{invalid json}`
	req := httptest.NewRequest(http.MethodPut, "/admin/config", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", rec.Code)
	}
}

func TestAdminConfig_PutConfig_ReturnsUpdatedConfig(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			AppKey: "app-secret-key-12345",
		},
		Retry: config.RetryConfig{
			MaxTokens: 3,
		},
	}

	handler := handlePutConfig(cfg, nil)

	body := `{"retry": {"max_tokens": 5}}`
	req := httptest.NewRequest(http.MethodPut, "/admin/config", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp ConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should return updated value
	if resp.Retry.MaxTokens != 5 {
		t.Errorf("expected max_tokens 5 in response, got %d", resp.Retry.MaxTokens)
	}

	// App key should be fully masked in response
	if resp.App.AppKey != "********" {
		t.Errorf("app_key not masked in response, got %s", resp.App.AppKey)
	}
}

func TestAdminConfig_PutConfig_RejectsEmptyAppKey(t *testing.T) {
	cfg := &config.Config{
		App: config.AppConfig{
			AppKey: "old-app-key",
		},
	}

	handler := handlePutConfig(cfg, nil)

	body := `{"app": {"app_key": ""}}`
	req := httptest.NewRequest(http.MethodPut, "/admin/config", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	// Config should not have changed
	if cfg.App.AppKey != "old-app-key" {
		t.Errorf("app key should not have changed, got %q", cfg.App.AppKey)
	}
}

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"abc", "***"},
		{"abcd1234", "abcd****1234"},
		{"sk-1234567890abcdef", "sk-1****cdef"},
		{"short", "shor****hort"},
	}

	for _, tt := range tests {
		got := maskSecret(tt.input)
		if got != tt.expected {
			t.Errorf("maskSecret(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestConfig_SelectionAlgorithm_Valid(t *testing.T) {
	cfg := &config.Config{
		Token: config.TokenConfig{
			SelectionAlgorithm: "high_quota_first",
		},
	}

	handler := handlePutConfig(cfg, nil)

	body := `{"token": {"selection_algorithm": "round_robin"}}`
	req := httptest.NewRequest(http.MethodPut, "/admin/config", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if cfg.Token.SelectionAlgorithm != "round_robin" {
		t.Errorf("expected selection_algorithm round_robin, got %s", cfg.Token.SelectionAlgorithm)
	}

	// Verify it appears in response
	var resp ConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Token.SelectionAlgorithm != "round_robin" {
		t.Errorf("expected round_robin in response, got %s", resp.Token.SelectionAlgorithm)
	}
}

func TestConfig_SelectionAlgorithm_Invalid(t *testing.T) {
	cfg := &config.Config{
		Token: config.TokenConfig{
			SelectionAlgorithm: "high_quota_first",
		},
	}

	handler := handlePutConfig(cfg, nil)

	body := `{"token": {"selection_algorithm": "invalid_algo"}}`
	req := httptest.NewRequest(http.MethodPut, "/admin/config", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid algorithm, got %d: %s", rec.Code, rec.Body.String())
	}

	// Config should not have changed
	if cfg.Token.SelectionAlgorithm != "high_quota_first" {
		t.Errorf("config should not have changed, got %s", cfg.Token.SelectionAlgorithm)
	}
}

func TestAdminConfig_PutConfig_IgnoresMaskedProxySecrets(t *testing.T) {
	cfg := &config.Config{
		Proxy: config.ProxyConfig{
			CFCookies:   "real-cookie",
			CFClearance: "real-clearance",
		},
	}

	handler := handlePutConfig(cfg, nil)

	body := `{"proxy":{"cf_cookies":"********","cf_clearance":"********"}}`
	req := httptest.NewRequest(http.MethodPut, "/admin/config", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if cfg.Proxy.CFCookies != "real-cookie" {
		t.Fatalf("cf_cookies should not be overwritten by masked placeholder, got %q", cfg.Proxy.CFCookies)
	}
	if cfg.Proxy.CFClearance != "real-clearance" {
		t.Fatalf("cf_clearance should not be overwritten by masked placeholder, got %q", cfg.Proxy.CFClearance)
	}
}
