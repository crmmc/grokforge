package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockUsageLogStoreForUsage implements UsageLogStoreForUsage for testing.
type mockUsageLogStoreForUsage struct {
	result   *store.UsagePeriodResult
	err      error
	logs     []store.UsageLogWithKeyName
	logTotal int64
	logErr   error
	// capture last ListLogs params for assertion
	lastListParams store.UsageLogListParams
}

func (m *mockUsageLogStoreForUsage) PeriodUsage(ctx context.Context, period string) (*store.UsagePeriodResult, error) {
	return m.result, m.err
}

func (m *mockUsageLogStoreForUsage) ListLogs(ctx context.Context, p store.UsageLogListParams) ([]store.UsageLogWithKeyName, int64, error) {
	m.lastListParams = p
	return m.logs, m.logTotal, m.logErr
}

func TestHandleSystemUsage_DefaultPeriod(t *testing.T) {
	mock := &mockUsageLogStoreForUsage{
		result: &store.UsagePeriodResult{
			Requests:     42,
			TokensInput:  1000,
			TokensOutput: 2000,
			Errors:       3,
			ByModel: map[string]store.ModelUsage{
				"grok-3": {Requests: 30, TokensInput: 700, TokensOutput: 1500},
			},
		},
	}

	handler := handleSystemUsage(mock)
	req := httptest.NewRequest(http.MethodGet, "/admin/system/usage", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp SystemUsageResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "day", resp.Period)
	assert.Equal(t, 42, resp.Requests)
	assert.Equal(t, 1000, resp.TokensInput)
	assert.Equal(t, 2000, resp.TokensOutput)
	assert.Equal(t, 3, resp.Errors)
	assert.Contains(t, resp.ByModel, "grok-3")
}

func TestHandleSystemUsage_AllPeriods(t *testing.T) {
	mock := &mockUsageLogStoreForUsage{
		result: &store.UsagePeriodResult{
			Requests: 10,
			ByModel:  make(map[string]store.ModelUsage),
		},
	}

	periods := []string{"hour", "day", "week", "month"}
	for _, p := range periods {
		t.Run(p, func(t *testing.T) {
			handler := handleSystemUsage(mock)
			req := httptest.NewRequest(http.MethodGet, "/admin/system/usage?period="+p, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)

			var resp SystemUsageResponse
			require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
			assert.Equal(t, p, resp.Period)
			assert.Equal(t, 10, resp.Requests)
		})
	}
}

func TestHandleSystemUsage_InvalidPeriod(t *testing.T) {
	mock := &mockUsageLogStoreForUsage{
		result: &store.UsagePeriodResult{},
	}

	handler := handleSystemUsage(mock)
	req := httptest.NewRequest(http.MethodGet, "/admin/system/usage?period=year", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestHandleSystemUsage_ResponseFormat(t *testing.T) {
	mock := &mockUsageLogStoreForUsage{
		result: &store.UsagePeriodResult{
			Requests:     5,
			TokensInput:  100,
			TokensOutput: 200,
			Errors:       1,
			ByModel: map[string]store.ModelUsage{
				"grok-3":      {Requests: 3, TokensInput: 60, TokensOutput: 120},
				"grok-3-mini": {Requests: 2, TokensInput: 40, TokensOutput: 80},
			},
		},
	}

	handler := handleSystemUsage(mock)
	req := httptest.NewRequest(http.MethodGet, "/admin/system/usage?period=day", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	// Parse raw JSON to verify exact field names match frontend UsageStats type
	var raw map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))

	// Verify all expected fields exist
	assert.Contains(t, raw, "period")
	assert.Contains(t, raw, "requests")
	assert.Contains(t, raw, "tokens_input")
	assert.Contains(t, raw, "tokens_output")
	assert.Contains(t, raw, "errors")
	assert.Contains(t, raw, "by_model")

	// Verify by_model structure
	byModel, ok := raw["by_model"].(map[string]any)
	require.True(t, ok)
	assert.Len(t, byModel, 2)

	grok3, ok := byModel["grok-3"].(map[string]any)
	require.True(t, ok)
	assert.Contains(t, grok3, "requests")
	assert.Contains(t, grok3, "tokens_input")
	assert.Contains(t, grok3, "tokens_output")
}

func TestHandleSystemUsage_CacheTokens(t *testing.T) {
	mock := &mockUsageLogStoreForUsage{
		result: &store.UsagePeriodResult{
			Requests:     10,
			TokensInput:  500,
			TokensOutput: 1000,
			CacheTokens:  200,
			Errors:       0,
			ByModel:      make(map[string]store.ModelUsage),
		},
	}

	handler := handleSystemUsage(mock)
	req := httptest.NewRequest(http.MethodGet, "/admin/system/usage?period=day", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var raw map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	assert.Contains(t, raw, "cache_tokens")
	assert.Equal(t, float64(200), raw["cache_tokens"])
}

func TestHandleUsageLogs_Default(t *testing.T) {
	mock := &mockUsageLogStoreForUsage{
		result: &store.UsagePeriodResult{ByModel: make(map[string]store.ModelUsage)},
		logs: []store.UsageLogWithKeyName{
			{UsageLog: store.UsageLog{ID: 1, Model: "grok-3"}, APIKeyName: "key-1"},
			{UsageLog: store.UsageLog{ID: 2, Model: "grok-3-mini"}, APIKeyName: "key-2"},
		},
		logTotal: 2,
	}

	handler := handleUsageLogs(mock)
	req := httptest.NewRequest(http.MethodGet, "/admin/usage/logs", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var raw map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))

	// PaginatedResponse shape
	assert.Contains(t, raw, "data")
	assert.Contains(t, raw, "total")
	assert.Contains(t, raw, "page")
	assert.Contains(t, raw, "page_size")
	assert.Contains(t, raw, "total_pages")

	assert.Equal(t, float64(2), raw["total"])
	assert.Equal(t, float64(1), raw["page"])
	assert.Equal(t, float64(20), raw["page_size"])
	assert.Equal(t, float64(1), raw["total_pages"])

	data := raw["data"].([]any)
	assert.Len(t, data, 2)

	// Verify first entry has api_key_name
	first := data[0].(map[string]any)
	assert.Equal(t, "key-1", first["api_key_name"])
}

func TestHandleUsageLogs_Pagination(t *testing.T) {
	mock := &mockUsageLogStoreForUsage{
		result:   &store.UsagePeriodResult{ByModel: make(map[string]store.ModelUsage)},
		logs:     []store.UsageLogWithKeyName{},
		logTotal: 50,
	}

	handler := handleUsageLogs(mock)
	req := httptest.NewRequest(http.MethodGet, "/admin/usage/logs?page=2&page_size=10", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	// Verify params passed to store
	assert.Equal(t, 2, mock.lastListParams.Page)
	assert.Equal(t, 10, mock.lastListParams.PageSize)

	var raw map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	assert.Equal(t, float64(50), raw["total"])
	assert.Equal(t, float64(2), raw["page"])
	assert.Equal(t, float64(10), raw["page_size"])
	assert.Equal(t, float64(5), raw["total_pages"]) // ceil(50/10) = 5
}

func TestHandleUsageLogs_Filters(t *testing.T) {
	mock := &mockUsageLogStoreForUsage{
		result:   &store.UsagePeriodResult{ByModel: make(map[string]store.ModelUsage)},
		logs:     []store.UsageLogWithKeyName{},
		logTotal: 0,
	}

	handler := handleUsageLogs(mock)
	req := httptest.NewRequest(http.MethodGet, "/admin/usage/logs?model=grok-3&period=day", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "grok-3", mock.lastListParams.Model)
	assert.Equal(t, "day", mock.lastListParams.Period)
}

func TestHandleUsageLogs_Sort(t *testing.T) {
	mock := &mockUsageLogStoreForUsage{
		result:   &store.UsagePeriodResult{ByModel: make(map[string]store.ModelUsage)},
		logs:     []store.UsageLogWithKeyName{},
		logTotal: 0,
	}

	handler := handleUsageLogs(mock)
	req := httptest.NewRequest(http.MethodGet, "/admin/usage/logs?sort_by=ttft&sort_dir=asc", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ttft", mock.lastListParams.SortBy)
	assert.Equal(t, "asc", mock.lastListParams.SortDir)
}

func TestHandleSystemUsage_ByAPIKey(t *testing.T) {
	mock := &mockUsageLogStoreForUsage{
		result: &store.UsagePeriodResult{
			Requests:     15,
			TokensInput:  300,
			TokensOutput: 600,
			Errors:       0,
			ByModel:      make(map[string]store.ModelUsage),
			ByAPIKey: []store.APIKeyUsage{
				{APIKeyName: "key-1", Requests: 10, TokensInput: 200, TokensOutput: 400},
				{APIKeyName: "key-2", Requests: 5, TokensInput: 100, TokensOutput: 200},
			},
		},
	}

	handler := handleSystemUsage(mock)
	req := httptest.NewRequest(http.MethodGet, "/admin/system/usage?period=day", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var raw map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	assert.Contains(t, raw, "by_api_key")

	byAPIKey, ok := raw["by_api_key"].([]any)
	require.True(t, ok)
	assert.Len(t, byAPIKey, 2)

	first := byAPIKey[0].(map[string]any)
	assert.Equal(t, "key-1", first["api_key_name"])
	assert.Equal(t, float64(10), first["requests"])
}

func TestHandleSystemUsage_ByAPIKeyNil(t *testing.T) {
	mock := &mockUsageLogStoreForUsage{
		result: &store.UsagePeriodResult{
			Requests: 0,
			ByModel:  make(map[string]store.ModelUsage),
			// ByAPIKey is nil
		},
	}

	handler := handleSystemUsage(mock)
	req := httptest.NewRequest(http.MethodGet, "/admin/system/usage?period=day", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var raw map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	// Should be empty array, not null
	byAPIKey, ok := raw["by_api_key"].([]any)
	require.True(t, ok)
	assert.Len(t, byAPIKey, 0)
}
