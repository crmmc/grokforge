package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleSystemUsage_PeriodError(t *testing.T) {
	mock := &failingUsageStore{periodErr: errors.New("db")}
	w := httptest.NewRecorder()
	handleSystemUsage(mock)(w, httptest.NewRequest(http.MethodGet, "/admin/system/usage?period=day", nil))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleSystemUsage_NilByModel(t *testing.T) {
	mock := &mockUsageLogStoreForUsage{
		result: &store.UsagePeriodResult{
			Requests: 1,
			ByModel:  nil, // must be coerced to an empty map
		},
	}
	w := httptest.NewRecorder()
	handleSystemUsage(mock)(w, httptest.NewRequest(http.MethodGet, "/admin/system/usage?period=hour", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var raw map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	byModel, ok := raw["by_model"].(map[string]any)
	require.True(t, ok, "by_model must encode as {} not null")
	assert.Empty(t, byModel)
}

func TestHandleUsageLogs_Extra(t *testing.T) {
	t.Run("clamps invalid pagination values", func(t *testing.T) {
		tests := []struct {
			name         string
			query        string
			wantPage     int
			wantPageSize int
		}{
			{name: "page zero clamped to 1", query: "?page=0", wantPage: 1, wantPageSize: 20},
			{name: "negative page clamped to 1", query: "?page=-3", wantPage: 1, wantPageSize: 20},
			{name: "page_size zero clamped to 1", query: "?page_size=0", wantPage: 1, wantPageSize: 1},
			{name: "page_size over 100 clamped to 100", query: "?page_size=500", wantPage: 1, wantPageSize: 100},
			{name: "non numeric values use defaults", query: "?page=abc&page_size=xyz", wantPage: 1, wantPageSize: 20},
		}

		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				mock := &mockUsageLogStoreForUsage{result: &store.UsagePeriodResult{ByModel: map[string]store.ModelUsage{}}}
				w := httptest.NewRecorder()
				handleUsageLogs(mock)(w, httptest.NewRequest(http.MethodGet, "/admin/usage/logs"+tc.query, nil))
				require.Equal(t, http.StatusOK, w.Code)
				assert.Equal(t, tc.wantPage, mock.lastListParams.Page)
				assert.Equal(t, tc.wantPageSize, mock.lastListParams.PageSize)
			})
		}
	})

	t.Run("list error returns 500", func(t *testing.T) {
		mock := &failingUsageStore{listErr: errors.New("db")}
		w := httptest.NewRecorder()
		handleUsageLogs(mock)(w, httptest.NewRequest(http.MethodGet, "/admin/usage/logs", nil))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("nil logs encoded as empty array", func(t *testing.T) {
		mock := &mockUsageLogStoreForUsage{result: &store.UsagePeriodResult{ByModel: map[string]store.ModelUsage{}}}
		w := httptest.NewRecorder()
		handleUsageLogs(mock)(w, httptest.NewRequest(http.MethodGet, "/admin/usage/logs", nil))
		require.Equal(t, http.StatusOK, w.Code)

		var raw map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
		data, ok := raw["data"].([]any)
		require.True(t, ok, "data must encode as [] not null")
		assert.Empty(t, data)
		assert.Equal(t, float64(0), raw["total_pages"])
	})

	t.Run("status and api_key filters forwarded", func(t *testing.T) {
		mock := &mockUsageLogStoreForUsage{result: &store.UsagePeriodResult{ByModel: map[string]store.ModelUsage{}}}
		w := httptest.NewRecorder()
		handleUsageLogs(mock)(w, httptest.NewRequest(http.MethodGet, "/admin/usage/logs?status=error&api_key=k1", nil))
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "error", mock.lastListParams.Status)
		assert.Equal(t, "k1", mock.lastListParams.APIKeyName)
	})
}

func TestQueryInt(t *testing.T) {
	tests := []struct {
		name string
		in   string
		def  int
		want int
	}{
		{name: "empty uses default", in: "", def: 7, want: 7},
		{name: "invalid uses default", in: "abc", def: 7, want: 7},
		{name: "valid value parsed", in: "42", def: 7, want: 42},
		{name: "negative parsed as-is", in: "-5", def: 7, want: -5},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, queryInt(tc.in, tc.def))
		})
	}
}
