package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	tokenPkg "github.com/crmmc/grokforge/internal/token"
	"github.com/go-chi/chi/v5"
)

// mockTokenStore implements a minimal token store for testing
type mockTokenStore struct {
	tokens map[uint]*store.Token
	nextID uint
}

func newMockTokenStore() *mockTokenStore {
	return &mockTokenStore{
		tokens: make(map[uint]*store.Token),
		nextID: 1,
	}
}

func (m *mockTokenStore) ListTokens(ctx context.Context) ([]*store.Token, error) {
	result := make([]*store.Token, 0, len(m.tokens))
	for _, t := range m.tokens {
		result = append(result, t)
	}
	return result, nil
}

func (m *mockTokenStore) GetToken(ctx context.Context, id uint) (*store.Token, error) {
	t, ok := m.tokens[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return t, nil
}

func (m *mockTokenStore) CreateToken(ctx context.Context, token *store.Token) error {
	token.ID = m.nextID
	token.CreatedAt = time.Now()
	token.UpdatedAt = time.Now()
	m.tokens[m.nextID] = token
	m.nextID++
	return nil
}

func (m *mockTokenStore) UpdateToken(ctx context.Context, token *store.Token) error {
	if _, ok := m.tokens[token.ID]; !ok {
		return store.ErrNotFound
	}
	token.UpdatedAt = time.Now()
	m.tokens[token.ID] = token
	return nil
}

func (m *mockTokenStore) DeleteToken(ctx context.Context, id uint) error {
	if _, ok := m.tokens[id]; !ok {
		return store.ErrNotFound
	}
	delete(m.tokens, id)
	return nil
}

func (m *mockTokenStore) ListTokensFiltered(ctx context.Context, filter store.TokenFilter) ([]*store.Token, error) {
	result := make([]*store.Token, 0)
	for _, t := range m.tokens {
		if filter.Status != nil && t.Status != *filter.Status {
			continue
		}
		if filter.NsfwEnabled != nil && t.NsfwEnabled != *filter.NsfwEnabled {
			continue
		}
		result = append(result, t)
	}
	return result, nil
}

func (m *mockTokenStore) BatchUpdateTokens(ctx context.Context, req store.BatchUpdateRequest) (int, error) {
	count := 0
	for _, id := range req.IDs {
		t, ok := m.tokens[id]
		if !ok {
			continue
		}
		if req.Status != nil {
			t.Status = *req.Status
		}
		if req.NsfwEnabled != nil {
			t.NsfwEnabled = *req.NsfwEnabled
		}
		count++
	}
	return count, nil
}

func (m *mockTokenStore) ListTokenIDs(ctx context.Context, filter store.TokenFilter) ([]uint, error) {
	var ids []uint
	for _, t := range m.tokens {
		if filter.Status != nil && t.Status != *filter.Status {
			continue
		}
		if filter.NsfwEnabled != nil && t.NsfwEnabled != *filter.NsfwEnabled {
			continue
		}
		ids = append(ids, t.ID)
	}
	return ids, nil
}

func TestAdminToken_ListTokens(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{
		ID:     1,
		Token:  "token1_secret_value_here",
		Pool:   "ssoBasic",
		Status: store.TokenStatusActive,
	}
	mockStore.tokens[2] = &store.Token{
		ID:     2,
		Token:  "token2_secret_value_here",
		Pool:   "ssoSuper",
		Status: store.TokenStatusCooling,
	}

	handler := handleListTokens(mockStore)
	req := httptest.NewRequest(http.MethodGet, "/admin/tokens", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp PaginatedTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Data) != 2 {
		t.Errorf("expected 2 tokens, got %d", len(resp.Data))
	}

	// Token values should be masked
	for _, token := range resp.Data {
		if token.Token != "" && len(token.Token) > 12 {
			t.Errorf("token should be masked, got %s", token.Token)
		}
	}
}

func TestAdminToken_GetToken(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{
		ID:     1,
		Token:  "token_secret_value",
		Pool:   "ssoBasic",
		Status: store.TokenStatusActive,
	}

	handler := handleGetToken(mockStore)

	// Create request with chi URL param
	req := httptest.NewRequest(http.MethodGet, "/admin/tokens/1", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp TokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.ID != 1 {
		t.Errorf("expected ID 1, got %d", resp.ID)
	}
}

func TestAdminToken_GetToken_NotFound(t *testing.T) {
	mockStore := newMockTokenStore()

	handler := handleGetToken(mockStore)

	req := httptest.NewRequest(http.MethodGet, "/admin/tokens/999", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "999")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestAdminToken_BatchImport_UsesLatestConfig(t *testing.T) {
	mockStore := newMockTokenStore()
	current := &config.TokenConfig{
		DefaultChatQuota:  50,
		DefaultImageQuota: 20,
		DefaultVideoQuota: 10,
	}
	handler := handleBatchTokensFromProvider(mockStore, nil, func() *config.TokenConfig { return current })

	current = &config.TokenConfig{
		DefaultChatQuota:  70,
		DefaultImageQuota: 30,
		DefaultVideoQuota: 15,
	}

	body := `{"operation":"import","tokens":["token_value_with_sufficient_length_12345"],"pool":"ssoBasic"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tokens/batch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	token := mockStore.tokens[1]
	if token.ChatQuota != 70 || token.ImageQuota != 30 || token.VideoQuota != 15 {
		t.Fatalf("expected runtime quotas 70/30/15, got %d/%d/%d", token.ChatQuota, token.ImageQuota, token.VideoQuota)
	}
}

func TestAdminToken_UpdateToken(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{
		ID:     1,
		Token:  "old_token",
		Status: store.TokenStatusActive,
	}

	handler := handleUpdateToken(mockStore, nil)

	body := `{"status": "disabled"}`
	req := httptest.NewRequest(http.MethodPut, "/admin/tokens/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify status was updated
	if mockStore.tokens[1].Status != store.TokenStatusDisabled {
		t.Errorf("expected status disabled, got %s", mockStore.tokens[1].Status)
	}
}

func TestAdminToken_UpdateToken_NotFound(t *testing.T) {
	mockStore := newMockTokenStore()

	handler := handleUpdateToken(mockStore, nil)

	body := `{"status": "disabled"}`
	req := httptest.NewRequest(http.MethodPut, "/admin/tokens/999", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "999")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestAdminToken_DeleteToken(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{
		ID:    1,
		Token: "to_delete",
	}

	handler := handleDeleteToken(mockStore, nil)

	req := httptest.NewRequest(http.MethodDelete, "/admin/tokens/1", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}

	// Verify token was deleted
	if len(mockStore.tokens) != 0 {
		t.Errorf("expected 0 tokens, got %d", len(mockStore.tokens))
	}
}

func TestAdminToken_DeleteToken_NotFound(t *testing.T) {
	mockStore := newMockTokenStore()

	handler := handleDeleteToken(mockStore, nil)

	req := httptest.NewRequest(http.MethodDelete, "/admin/tokens/999", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "999")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rec.Code)
	}
}

func TestAdminToken_BatchImport(t *testing.T) {
	mockStore := newMockTokenStore()
	handler := handleBatchTokens(mockStore, nil, nil)

	body := `{"operation":"import","tokens":["token1_long_enough_for_test","token2_long_enough_for_test","token3_long_enough_for_test"],"pool":"default","quota":100}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tokens/batch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp BatchTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Operation != BatchOpImport {
		t.Errorf("expected operation import, got %s", resp.Operation)
	}
	if resp.Success != 3 {
		t.Errorf("expected 3 success, got %d", resp.Success)
	}
	if resp.Failed != 0 {
		t.Errorf("expected 0 failed, got %d", resp.Failed)
	}
	if len(mockStore.tokens) != 3 {
		t.Errorf("expected 3 tokens in store, got %d", len(mockStore.tokens))
	}
}

func TestAdminToken_BatchExport(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{ID: 1, Token: "token1", Status: store.TokenStatusActive}
	mockStore.tokens[2] = &store.Token{ID: 2, Token: "token2", Status: store.TokenStatusActive}

	handler := handleBatchTokens(mockStore, nil, nil)

	body := `{"operation":"export"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tokens/batch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp BatchTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Operation != BatchOpExport {
		t.Errorf("expected operation export, got %s", resp.Operation)
	}
	if resp.Success != 2 {
		t.Errorf("expected 2 success, got %d", resp.Success)
	}
	if len(resp.Tokens) != 2 {
		t.Errorf("expected 2 tokens, got %d", len(resp.Tokens))
	}
}

func TestAdminToken_BatchExportRaw(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{ID: 1, Token: "token1", Status: store.TokenStatusActive}
	mockStore.tokens[2] = &store.Token{ID: 2, Token: "token2", Status: store.TokenStatusActive}

	handler := handleBatchTokens(mockStore, nil, nil)

	body := `{"operation":"export"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tokens/batch?raw=true", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp BatchTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.RawTokens) != 2 {
		t.Errorf("expected 2 raw tokens, got %d", len(resp.RawTokens))
	}
	// Raw tokens should not be masked
	for _, tok := range resp.RawTokens {
		if tok != "token1" && tok != "token2" {
			t.Errorf("unexpected raw token: %s", tok)
		}
	}
}

func TestAdminToken_BatchDelete(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{ID: 1, Token: "token1"}
	mockStore.tokens[2] = &store.Token{ID: 2, Token: "token2"}
	mockStore.tokens[3] = &store.Token{ID: 3, Token: "token3"}

	handler := handleBatchTokens(mockStore, nil, nil)

	body := `{"operation":"delete","ids":[1,3]}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tokens/batch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp BatchTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Operation != BatchOpDelete {
		t.Errorf("expected operation delete, got %s", resp.Operation)
	}
	if resp.Success != 2 {
		t.Errorf("expected 2 success, got %d", resp.Success)
	}
	if len(mockStore.tokens) != 1 {
		t.Errorf("expected 1 token remaining, got %d", len(mockStore.tokens))
	}
	if _, ok := mockStore.tokens[2]; !ok {
		t.Error("expected token 2 to remain")
	}
}

func TestAdminToken_BatchInvalidOperation(t *testing.T) {
	mockStore := newMockTokenStore()
	handler := handleBatchTokens(mockStore, nil, nil)

	body := `{"operation":"invalid"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tokens/batch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// Test 1: GET /admin/tokens returns all tokens (existing behavior)
func TestAdminToken_ListTokensWithFilter_NoFilter(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{ID: 1, Token: "token1", Pool: "ssoBasic", Status: store.TokenStatusActive, NsfwEnabled: false}
	mockStore.tokens[2] = &store.Token{ID: 2, Token: "token2", Pool: "ssoSuper", Status: store.TokenStatusCooling, NsfwEnabled: true}

	handler := handleListTokens(mockStore)
	req := httptest.NewRequest(http.MethodGet, "/admin/tokens", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp PaginatedTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Data) != 2 {
		t.Errorf("expected 2 tokens, got %d", len(resp.Data))
	}
}

// Test 2: GET /admin/tokens?status=active returns only active tokens
func TestAdminToken_ListTokensWithFilter_StatusActive(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{ID: 1, Token: "token1", Pool: "ssoBasic", Status: store.TokenStatusActive, NsfwEnabled: false}
	mockStore.tokens[2] = &store.Token{ID: 2, Token: "token2", Pool: "ssoSuper", Status: store.TokenStatusCooling, NsfwEnabled: true}
	mockStore.tokens[3] = &store.Token{ID: 3, Token: "token3", Pool: "ssoBasic", Status: store.TokenStatusActive, NsfwEnabled: true}

	handler := handleListTokens(mockStore)
	req := httptest.NewRequest(http.MethodGet, "/admin/tokens?status=active", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp PaginatedTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Data) != 2 {
		t.Errorf("expected 2 active tokens, got %d", len(resp.Data))
	}
	for _, tok := range resp.Data {
		if tok.Status != store.TokenStatusActive {
			t.Errorf("expected status active, got %s", tok.Status)
		}
	}
}

// Test 3: GET /admin/tokens?nsfw=true returns only NSFW-enabled tokens
func TestAdminToken_ListTokensWithFilter_NsfwTrue(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{ID: 1, Token: "token1", Pool: "ssoBasic", Status: store.TokenStatusActive, NsfwEnabled: false}
	mockStore.tokens[2] = &store.Token{ID: 2, Token: "token2", Pool: "ssoSuper", Status: store.TokenStatusActive, NsfwEnabled: true}
	mockStore.tokens[3] = &store.Token{ID: 3, Token: "token3", Pool: "ssoBasic", Status: store.TokenStatusCooling, NsfwEnabled: true}

	handler := handleListTokens(mockStore)
	req := httptest.NewRequest(http.MethodGet, "/admin/tokens?nsfw=true", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp PaginatedTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Data) != 2 {
		t.Errorf("expected 2 NSFW-enabled tokens, got %d", len(resp.Data))
	}
	for _, tok := range resp.Data {
		if !tok.NsfwEnabled {
			t.Error("expected NsfwEnabled to be true")
		}
	}
}

// Test 4: GET /admin/tokens?status=active&nsfw=false combines both filters
func TestAdminToken_ListTokensWithFilter_Combined(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{ID: 1, Token: "token1", Pool: "ssoBasic", Status: store.TokenStatusActive, NsfwEnabled: false}
	mockStore.tokens[2] = &store.Token{ID: 2, Token: "token2", Pool: "ssoSuper", Status: store.TokenStatusActive, NsfwEnabled: true}
	mockStore.tokens[3] = &store.Token{ID: 3, Token: "token3", Pool: "ssoBasic", Status: store.TokenStatusCooling, NsfwEnabled: false}

	handler := handleListTokens(mockStore)
	req := httptest.NewRequest(http.MethodGet, "/admin/tokens?status=active&nsfw=false", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}

	var resp PaginatedTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Data) != 1 {
		t.Errorf("expected 1 token with status=active AND nsfw=false, got %d", len(resp.Data))
	}
	if len(resp.Data) > 0 {
		if resp.Data[0].Status != store.TokenStatusActive {
			t.Errorf("expected status active, got %s", resp.Data[0].Status)
		}
		if resp.Data[0].NsfwEnabled {
			t.Error("expected NsfwEnabled to be false")
		}
	}
}

// Test 5: Response includes remark and nsfw_enabled fields
func TestAdminToken_ListTokensWithFilter_ResponseIncludesNewFields(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{
		ID:                1,
		Token:             "token1",
		Pool:              "ssoBasic",
		Status:            store.TokenStatusActive,
		ChatQuota:         30,
		InitialChatQuota:  50,
		ImageQuota:        6,
		InitialImageQuota: 10,
		VideoQuota:        2,
		InitialVideoQuota: 4,
		NsfwEnabled:       true,
		Remark:            "Test remark",
	}

	handler := handleListTokens(mockStore)
	req := httptest.NewRequest(http.MethodGet, "/admin/tokens", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var resp PaginatedTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 token, got %d", len(resp.Data))
	}

	if resp.Data[0].NsfwEnabled != true {
		t.Error("expected NsfwEnabled to be true in response")
	}
	if resp.Data[0].Remark != "Test remark" {
		t.Errorf("expected Remark 'Test remark', got '%s'", resp.Data[0].Remark)
	}
	if resp.Data[0].TotalChatQuota != 50 || resp.Data[0].TotalImageQuota != 10 || resp.Data[0].TotalVideoQuota != 4 {
		t.Errorf("expected total quotas 50/10/4, got %d/%d/%d", resp.Data[0].TotalChatQuota, resp.Data[0].TotalImageQuota, resp.Data[0].TotalVideoQuota)
	}
}

// Test: Invalid nsfw param returns 400 error
func TestAdminToken_ListTokensWithFilter_InvalidNsfw(t *testing.T) {
	mockStore := newMockTokenStore()
	handler := handleListTokens(mockStore)

	req := httptest.NewRequest(http.MethodGet, "/admin/tokens?nsfw=invalid", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

// Test 1: POST /admin/tokens/batch with enable sets status=active
func TestAdminToken_BatchEnable(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{ID: 1, Token: "token1", Status: store.TokenStatusDisabled}
	mockStore.tokens[2] = &store.Token{ID: 2, Token: "token2", Status: store.TokenStatusDisabled}

	handler := handleBatchTokens(mockStore, nil, nil)

	body := `{"operation":"enable","ids":[1,2]}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tokens/batch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp BatchTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Operation != BatchOpEnable {
		t.Errorf("expected operation enable, got %s", resp.Operation)
	}
	if resp.Success != 2 {
		t.Errorf("expected 2 success, got %d", resp.Success)
	}
	if mockStore.tokens[1].Status != store.TokenStatusActive {
		t.Errorf("expected token 1 status active, got %s", mockStore.tokens[1].Status)
	}
	if mockStore.tokens[2].Status != store.TokenStatusActive {
		t.Errorf("expected token 2 status active, got %s", mockStore.tokens[2].Status)
	}
}

// Test 2: POST /admin/tokens/batch with disable sets status=disabled
func TestAdminToken_BatchDisable(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{ID: 1, Token: "token1", Status: store.TokenStatusActive}
	mockStore.tokens[2] = &store.Token{ID: 2, Token: "token2", Status: store.TokenStatusActive}

	handler := handleBatchTokens(mockStore, nil, nil)

	body := `{"operation":"disable","ids":[1,2]}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tokens/batch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp BatchTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Operation != BatchOpDisable {
		t.Errorf("expected operation disable, got %s", resp.Operation)
	}
	if resp.Success != 2 {
		t.Errorf("expected 2 success, got %d", resp.Success)
	}
	if mockStore.tokens[1].Status != store.TokenStatusDisabled {
		t.Errorf("expected token 1 status disabled, got %s", mockStore.tokens[1].Status)
	}
}

// Test 3: POST /admin/tokens/batch with enable_nsfw sets nsfw_enabled=true
func TestAdminToken_BatchEnableNsfw(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{ID: 1, Token: "token1", NsfwEnabled: false}
	mockStore.tokens[2] = &store.Token{ID: 2, Token: "token2", NsfwEnabled: false}

	handler := handleBatchTokens(mockStore, nil, nil)

	body := `{"operation":"enable_nsfw","ids":[1,2]}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tokens/batch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp BatchTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Operation != BatchOpEnableNsfw {
		t.Errorf("expected operation enable_nsfw, got %s", resp.Operation)
	}
	if resp.Success != 2 {
		t.Errorf("expected 2 success, got %d", resp.Success)
	}
	if !mockStore.tokens[1].NsfwEnabled {
		t.Error("expected token 1 NsfwEnabled to be true")
	}
	if !mockStore.tokens[2].NsfwEnabled {
		t.Error("expected token 2 NsfwEnabled to be true")
	}
}

// Test 4: POST /admin/tokens/batch with disable_nsfw sets nsfw_enabled=false
func TestAdminToken_BatchDisableNsfw(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{ID: 1, Token: "token1", NsfwEnabled: true}
	mockStore.tokens[2] = &store.Token{ID: 2, Token: "token2", NsfwEnabled: true}

	handler := handleBatchTokens(mockStore, nil, nil)

	body := `{"operation":"disable_nsfw","ids":[1,2]}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tokens/batch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp BatchTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Operation != BatchOpDisableNsfw {
		t.Errorf("expected operation disable_nsfw, got %s", resp.Operation)
	}
	if resp.Success != 2 {
		t.Errorf("expected 2 success, got %d", resp.Success)
	}
	if mockStore.tokens[1].NsfwEnabled {
		t.Error("expected token 1 NsfwEnabled to be false")
	}
	if mockStore.tokens[2].NsfwEnabled {
		t.Error("expected token 2 NsfwEnabled to be false")
	}
}

// Test 5: Batch update returns affected count
func TestAdminToken_BatchUpdate_ReturnsCount(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{ID: 1, Token: "token1", Status: store.TokenStatusActive}
	mockStore.tokens[2] = &store.Token{ID: 2, Token: "token2", Status: store.TokenStatusActive}
	// Note: ID 3 doesn't exist

	handler := handleBatchTokens(mockStore, nil, nil)

	body := `{"operation":"disable","ids":[1,2,3]}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tokens/batch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var resp BatchTokenResponse
	json.NewDecoder(rec.Body).Decode(&resp)

	// Mock implementation returns count of found IDs
	if resp.Success != 2 {
		t.Errorf("expected 2 success (only existing tokens), got %d", resp.Success)
	}
}

// Test 7: Import rejects short tokens (< 20 characters)
func TestAdminToken_BatchImport_ShortTokenRejected(t *testing.T) {
	mockStore := newMockTokenStore()
	handler := handleBatchTokens(mockStore, nil, nil)

	body := `{"operation":"import","tokens":["short","also_short_token","valid_token_long_enough_here"],"pool":"default","quota":100}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tokens/batch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp BatchTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Success != 1 {
		t.Errorf("expected 1 success (only valid token), got %d", resp.Success)
	}
	if resp.Failed != 2 {
		t.Errorf("expected 2 failed (short tokens), got %d", resp.Failed)
	}
	if len(resp.Errors) != 2 {
		t.Fatalf("expected 2 errors, got %d", len(resp.Errors))
	}
	for _, e := range resp.Errors {
		if e.Message != "token too short (minimum 20 characters)" {
			t.Errorf("expected 'token too short' message, got '%s'", e.Message)
		}
	}
}

// Test 8: Import operation supports remark and nsfw_enabled
func TestAdminToken_BatchImport_WithRemarkAndNsfw(t *testing.T) {
	mockStore := newMockTokenStore()

	handler := handleBatchTokens(mockStore, nil, nil)

	body := `{"operation":"import","tokens":["token1_long_enough_for_test","token2_long_enough_for_test"],"pool":"ssoBasic","quota":100,"remark":"Imported tokens","nsfw_enabled":true}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tokens/batch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp BatchTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Success != 2 {
		t.Errorf("expected 2 success, got %d", resp.Success)
	}

	// Verify tokens have remark and nsfw_enabled
	for id, token := range mockStore.tokens {
		if token.Remark != "Imported tokens" {
			t.Errorf("token %d: expected remark 'Imported tokens', got '%s'", id, token.Remark)
		}
		if !token.NsfwEnabled {
			t.Errorf("token %d: expected NsfwEnabled true", id)
		}
	}
}

// Test 8: Import with pool and remark applies to all tokens
func TestAdminToken_BatchImport_PoolAndRemark(t *testing.T) {
	mockStore := newMockTokenStore()

	handler := handleBatchTokens(mockStore, nil, nil)

	body := `{"operation":"import","tokens":["token_a_long_enough_for_test","token_b_long_enough_for_test"],"pool":"ssoSuper","remark":"Super pool tokens"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tokens/batch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify all tokens have correct pool and remark
	for id, token := range mockStore.tokens {
		if token.Pool != "ssoSuper" {
			t.Errorf("token %d: expected pool 'ssoSuper', got '%s'", id, token.Pool)
		}
		if token.Remark != "Super pool tokens" {
			t.Errorf("token %d: expected remark 'Super pool tokens', got '%s'", id, token.Remark)
		}
	}
}

// Test 1: PUT /admin/tokens/:id with remark updates remark field
func TestAdminToken_UpdateToken_Remark(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{
		ID:     1,
		Token:  "token1",
		Status: store.TokenStatusActive,
		Remark: "Original remark",
	}

	handler := handleUpdateToken(mockStore, nil)

	body := `{"remark": "Updated remark"}`
	req := httptest.NewRequest(http.MethodPut, "/admin/tokens/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify remark was updated
	if mockStore.tokens[1].Remark != "Updated remark" {
		t.Errorf("expected remark 'Updated remark', got '%s'", mockStore.tokens[1].Remark)
	}

	// Verify response includes updated remark
	var resp TokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Remark != "Updated remark" {
		t.Errorf("expected response remark 'Updated remark', got '%s'", resp.Remark)
	}
}

// Test 2: PUT /admin/tokens/:id with nsfw_enabled updates nsfw field
func TestAdminToken_UpdateToken_NsfwEnabled(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{
		ID:          1,
		Token:       "token1",
		Status:      store.TokenStatusActive,
		NsfwEnabled: false,
	}

	handler := handleUpdateToken(mockStore, nil)

	body := `{"nsfw_enabled": true}`
	req := httptest.NewRequest(http.MethodPut, "/admin/tokens/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify nsfw_enabled was updated
	if !mockStore.tokens[1].NsfwEnabled {
		t.Error("expected NsfwEnabled to be true")
	}

	// Verify response includes updated nsfw_enabled
	var resp TokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.NsfwEnabled {
		t.Error("expected response NsfwEnabled to be true")
	}
}

// Test 3: PUT validates input (remark max length 500)
func TestAdminToken_UpdateToken_RemarkMaxLength(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{
		ID:     1,
		Token:  "token1",
		Status: store.TokenStatusActive,
	}

	handler := handleUpdateToken(mockStore, nil)

	// Create a remark that's too long (501 characters)
	longRemark := make([]byte, 501)
	for i := range longRemark {
		longRemark[i] = 'a'
	}

	body := `{"remark": "` + string(longRemark) + `"}`
	req := httptest.NewRequest(http.MethodPut, "/admin/tokens/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for remark too long, got %d", rec.Code)
	}
}

// Test 4: Partial update only changes specified fields
func TestAdminToken_UpdateToken_PartialUpdate(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{
		ID:          1,
		Token:       "token1",
		Pool:        "ssoBasic",
		Status:      store.TokenStatusActive,
		ChatQuota:   100,
		Remark:      "Original remark",
		NsfwEnabled: false,
	}

	handler := handleUpdateToken(mockStore, nil)

	// Only update remark, leave other fields unchanged
	body := `{"remark": "Only remark changed"}`
	req := httptest.NewRequest(http.MethodPut, "/admin/tokens/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	// Verify only remark was changed
	if mockStore.tokens[1].Remark != "Only remark changed" {
		t.Errorf("expected remark updated, got '%s'", mockStore.tokens[1].Remark)
	}
	if mockStore.tokens[1].Pool != "ssoBasic" {
		t.Errorf("expected pool unchanged, got '%s'", mockStore.tokens[1].Pool)
	}
	if mockStore.tokens[1].Status != store.TokenStatusActive {
		t.Errorf("expected status unchanged, got '%s'", mockStore.tokens[1].Status)
	}
	if mockStore.tokens[1].ChatQuota != 100 {
		t.Errorf("expected quota unchanged, got %d", mockStore.tokens[1].ChatQuota)
	}
	if mockStore.tokens[1].NsfwEnabled {
		t.Error("expected NsfwEnabled unchanged (false)")
	}
}

// --- Token Refresh Handler Tests ---

// mockTokenRefresher implements TokenRefresher for testing.
type mockTokenRefresher struct {
	token *store.Token
	err   error
}

func (m *mockTokenRefresher) RefreshToken(ctx context.Context, id uint) (*store.Token, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.token != nil && m.token.ID == id {
		return m.token, nil
	}
	return nil, tokenPkg.ErrTokenNotFound
}

func TestHandleRefreshToken_Success(t *testing.T) {
	refresher := &mockTokenRefresher{
		token: &store.Token{
			ID:        1,
			Token:     "token_secret_value",
			Pool:      "ssoBasic",
			Status:    store.TokenStatusActive,
			ChatQuota: 80,
		},
	}

	handler := handleRefreshToken(refresher)

	req := httptest.NewRequest(http.MethodPost, "/admin/tokens/1/refresh", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp TokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.ID != 1 {
		t.Errorf("expected ID 1, got %d", resp.ID)
	}
	if resp.ChatQuota != 80 {
		t.Errorf("expected ChatQuota 80, got %d", resp.ChatQuota)
	}
}

func TestHandleRefreshToken_NotFound(t *testing.T) {
	refresher := &mockTokenRefresher{}

	handler := handleRefreshToken(refresher)

	req := httptest.NewRequest(http.MethodPost, "/admin/tokens/999/refresh", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "999")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleRefreshToken_InvalidID(t *testing.T) {
	refresher := &mockTokenRefresher{}

	handler := handleRefreshToken(refresher)

	req := httptest.NewRequest(http.MethodPost, "/admin/tokens/abc/refresh", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "abc")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- resolveImportQuota Tests ---

func TestResolveImportQuota_ExplicitQuota(t *testing.T) {
	cfg := &config.TokenConfig{}
	q := 50
	got := resolveImportQuota(&q, cfg)
	if got != 50 {
		t.Errorf("expected 50, got %d", got)
	}
}

func TestResolveImportQuota_NilQuota_SsoBasic_DefaultFromConfig(t *testing.T) {
	cfg := &config.TokenConfig{DefaultChatQuota: 80}
	got := resolveImportQuota(nil, cfg)
	if got != 80 {
		t.Errorf("expected 80, got %d", got)
	}
}

func TestResolveImportQuota_NilQuota_SsoSuper_DefaultFromConfig(t *testing.T) {
	cfg := &config.TokenConfig{DefaultChatQuota: 200}
	got := resolveImportQuota(nil, cfg)
	if got != 200 {
		t.Errorf("expected 200, got %d", got)
	}
}

func TestResolveImportQuota_NilQuota_SsoBasic_FallbackHardcoded(t *testing.T) {
	cfg := &config.TokenConfig{} // DefaultChatQuota = 0
	got := resolveImportQuota(nil, cfg)
	if got != 50 {
		t.Errorf("expected fallback 50, got %d", got)
	}
}

func TestResolveImportQuota_NilQuota_SsoSuper_FallbackHardcoded(t *testing.T) {
	cfg := &config.TokenConfig{} // DefaultChatQuota = 0
	got := resolveImportQuota(nil, cfg)
	if got != 50 {
		t.Errorf("expected fallback 50, got %d", got)
	}
}

func TestResolveImportQuota_ZeroQuota_AutoResolves(t *testing.T) {
	cfg := &config.TokenConfig{DefaultChatQuota: 80}
	q := 0
	got := resolveImportQuota(&q, cfg)
	if got != 80 {
		t.Errorf("expected auto-resolved 80, got %d", got)
	}
}

// --- Import Priority Tests ---

func TestHandleBatchImport_Priority(t *testing.T) {
	mockStore := newMockTokenStore()
	cfg := &config.TokenConfig{DefaultChatQuota: 80}

	req := BatchTokenRequest{
		Operation: BatchOpImport,
		Tokens:    []string{"token_long_enough_for_priority_test"},
		Pool:      "ssoBasic",
		Priority:  3,
	}

	resp := handleBatchImport(context.Background(), mockStore, nil, req, cfg)
	if resp.Success != 1 {
		t.Fatalf("expected 1 success, got %d", resp.Success)
	}

	// Verify token has priority=3
	for _, tok := range mockStore.tokens {
		if tok.Priority != 3 {
			t.Errorf("expected priority 3, got %d", tok.Priority)
		}
	}
}

func TestHandleBatchImport_DefaultPriority(t *testing.T) {
	mockStore := newMockTokenStore()
	cfg := &config.TokenConfig{DefaultChatQuota: 80}

	req := BatchTokenRequest{
		Operation: BatchOpImport,
		Tokens:    []string{"token_long_enough_for_priority_test"},
		Pool:      "ssoBasic",
	}

	resp := handleBatchImport(context.Background(), mockStore, nil, req, cfg)
	if resp.Success != 1 {
		t.Fatalf("expected 1 success, got %d", resp.Success)
	}

	// Verify token has priority=0 (default)
	for _, tok := range mockStore.tokens {
		if tok.Priority != 0 {
			t.Errorf("expected priority 0, got %d", tok.Priority)
		}
	}
}

func TestHandleBatchImport_AutoQuota(t *testing.T) {
	mockStore := newMockTokenStore()
	cfg := &config.TokenConfig{DefaultChatQuota: 200}

	req := BatchTokenRequest{
		Operation: BatchOpImport,
		Tokens:    []string{"token_long_enough_for_auto_quota_test"},
		Pool:      "ssoSuper",
		// Quota is nil (*int), should auto-resolve to 200
	}

	resp := handleBatchImport(context.Background(), mockStore, nil, req, cfg)
	if resp.Success != 1 {
		t.Fatalf("expected 1 success, got %d", resp.Success)
	}

	for _, tok := range mockStore.tokens {
		if tok.ChatQuota != 200 {
			t.Errorf("expected auto-resolved quota 200, got %d", tok.ChatQuota)
		}
	}
}

// --- TokenResponse Priority Tests ---

func TestTokenResponse_IncludesPriority(t *testing.T) {
	tok := &store.Token{
		ID:                1,
		Token:             "secret_token_value",
		Pool:              "ssoBasic",
		Status:            store.TokenStatusActive,
		ChatQuota:         60,
		InitialChatQuota:  100,
		ImageQuota:        10,
		InitialImageQuota: 20,
		VideoQuota:        3,
		InitialVideoQuota: 5,
		Priority:          5,
	}

	resp := tokenToResponse(tok)
	if resp.Priority != 5 {
		t.Errorf("expected priority 5 in response, got %d", resp.Priority)
	}
	if resp.TotalChatQuota != 100 || resp.TotalImageQuota != 20 || resp.TotalVideoQuota != 5 {
		t.Errorf("expected total quotas 100/20/5, got %d/%d/%d", resp.TotalChatQuota, resp.TotalImageQuota, resp.TotalVideoQuota)
	}
}

func TestTokenResponse_DefaultPriority(t *testing.T) {
	tok := &store.Token{
		ID:        1,
		Token:     "secret_token_value",
		Pool:      "ssoBasic",
		Status:    store.TokenStatusActive,
		ChatQuota: 40,
		// Priority = 0 (default)
	}

	resp := tokenToResponse(tok)
	if resp.Priority != 0 {
		t.Errorf("expected priority 0 in response, got %d", resp.Priority)
	}
	if resp.TotalChatQuota != 40 {
		t.Errorf("expected total chat quota fallback to current value, got %d", resp.TotalChatQuota)
	}
}

// Test 5: Response includes updated remark and nsfw_enabled
func TestAdminToken_UpdateToken_ResponseIncludesNewFields(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{
		ID:          1,
		Token:       "token1",
		Status:      store.TokenStatusActive,
		Remark:      "",
		NsfwEnabled: false,
	}

	handler := handleUpdateToken(mockStore, nil)

	body := `{"remark": "New remark", "nsfw_enabled": true}`
	req := httptest.NewRequest(http.MethodPut, "/admin/tokens/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "1")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var resp TokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Remark != "New remark" {
		t.Errorf("expected response remark 'New remark', got '%s'", resp.Remark)
	}
	if !resp.NsfwEnabled {
		t.Error("expected response NsfwEnabled to be true")
	}
}

// --- Import Status Tests ---

func TestHandleBatchImport_StatusDisabled(t *testing.T) {
	mockStore := newMockTokenStore()
	cfg := &config.TokenConfig{DefaultChatQuota: 80}

	req := BatchTokenRequest{
		Operation: BatchOpImport,
		Tokens:    []string{"token_long_enough_for_status_test"},
		Pool:      "ssoBasic",
		Status:    "disabled",
	}

	resp := handleBatchImport(context.Background(), mockStore, nil, req, cfg)
	if resp.Success != 1 {
		t.Fatalf("expected 1 success, got %d", resp.Success)
	}

	for _, tok := range mockStore.tokens {
		if tok.Status != store.TokenStatusDisabled {
			t.Errorf("expected status 'disabled', got '%s'", tok.Status)
		}
	}
}

func TestHandleBatchImport_StatusDefaultsToActive(t *testing.T) {
	mockStore := newMockTokenStore()
	cfg := &config.TokenConfig{DefaultChatQuota: 80}

	req := BatchTokenRequest{
		Operation: BatchOpImport,
		Tokens:    []string{"token_long_enough_for_default_status"},
		Pool:      "ssoBasic",
		// Status is empty string, should default to "active"
	}

	resp := handleBatchImport(context.Background(), mockStore, nil, req, cfg)
	if resp.Success != 1 {
		t.Fatalf("expected 1 success, got %d", resp.Success)
	}

	for _, tok := range mockStore.tokens {
		if tok.Status != store.TokenStatusActive {
			t.Errorf("expected status 'active', got '%s'", tok.Status)
		}
	}
}

func TestHandleBatchImport_InvalidStatusDefaultsToActive(t *testing.T) {
	mockStore := newMockTokenStore()
	cfg := &config.TokenConfig{DefaultChatQuota: 80}

	req := BatchTokenRequest{
		Operation: BatchOpImport,
		Tokens:    []string{"token_long_enough_for_invalid_status"},
		Pool:      "ssoBasic",
		Status:    "expired", // invalid for import
	}

	resp := handleBatchImport(context.Background(), mockStore, nil, req, cfg)
	if resp.Success != 1 {
		t.Fatalf("expected 1 success, got %d", resp.Success)
	}

	for _, tok := range mockStore.tokens {
		if tok.Status != store.TokenStatusActive {
			t.Errorf("expected status 'active' for invalid import status, got '%s'", tok.Status)
		}
	}
}

// --- ListTokenIDs Tests ---

func TestListTokenIDs_FilterByStatus(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{ID: 1, Token: "token1", Status: store.TokenStatusActive}
	mockStore.tokens[2] = &store.Token{ID: 2, Token: "token2", Status: store.TokenStatusCooling}
	mockStore.tokens[3] = &store.Token{ID: 3, Token: "token3", Status: store.TokenStatusActive}
	mockStore.tokens[4] = &store.Token{ID: 4, Token: "token4", Status: store.TokenStatusDisabled}

	handler := handleListTokenIDs(mockStore)

	req := httptest.NewRequest(http.MethodGet, "/admin/tokens/ids?status=active", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		IDs []uint `json:"ids"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if len(resp.IDs) != 2 {
		t.Errorf("expected 2 active IDs, got %d", len(resp.IDs))
	}
}

func TestListTokenIDs_NoFilter(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{ID: 1, Token: "token1", Status: store.TokenStatusActive}
	mockStore.tokens[2] = &store.Token{ID: 2, Token: "token2", Status: store.TokenStatusCooling}

	handler := handleListTokenIDs(mockStore)

	req := httptest.NewRequest(http.MethodGet, "/admin/tokens/ids", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp struct {
		IDs []uint `json:"ids"`
	}
	json.NewDecoder(rec.Body).Decode(&resp)

	if len(resp.IDs) != 2 {
		t.Errorf("expected 2 IDs (all), got %d", len(resp.IDs))
	}
}

// --- BatchExport with IDs Tests ---

func TestBatchExport_WithIDs(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{ID: 1, Token: "token1"}
	mockStore.tokens[2] = &store.Token{ID: 2, Token: "token2"}
	mockStore.tokens[3] = &store.Token{ID: 3, Token: "token3"}

	handler := handleBatchTokens(mockStore, nil, nil)

	body := `{"operation":"export","ids":[1,3]}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tokens/batch?raw=true", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp BatchTokenResponse
	json.NewDecoder(rec.Body).Decode(&resp)

	if len(resp.RawTokens) != 2 {
		t.Errorf("expected 2 raw tokens (filtered by IDs), got %d", len(resp.RawTokens))
	}
	if resp.Success != 2 {
		t.Errorf("expected success=2, got %d", resp.Success)
	}
}

func TestBatchExport_WithoutIDs_ExportsAll(t *testing.T) {
	mockStore := newMockTokenStore()
	mockStore.tokens[1] = &store.Token{ID: 1, Token: "token1"}
	mockStore.tokens[2] = &store.Token{ID: 2, Token: "token2"}
	mockStore.tokens[3] = &store.Token{ID: 3, Token: "token3"}

	handler := handleBatchTokens(mockStore, nil, nil)

	body := `{"operation":"export"}`
	req := httptest.NewRequest(http.MethodPost, "/admin/tokens/batch?raw=true", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var resp BatchTokenResponse
	json.NewDecoder(rec.Body).Decode(&resp)

	if len(resp.RawTokens) != 3 {
		t.Errorf("expected 3 raw tokens (all), got %d", len(resp.RawTokens))
	}
}
