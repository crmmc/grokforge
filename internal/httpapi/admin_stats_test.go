package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/config"
	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockUsageLogStore implements UsageLogStoreInterface for testing.
type mockUsageLogStore struct {
	todayCounts     map[string]int
	yesterdayCounts map[string]int
	hourlyBreakdown []HourlyUsage
	tokenTotals     *store.TokenTotals
	tokenTotalsErr  error
}

func (m *mockUsageLogStore) Record(ctx context.Context, log *store.UsageLog) error {
	return nil
}

func (m *mockUsageLogStore) TodayCountByEndpoint(ctx context.Context) (map[string]int, error) {
	return m.todayCounts, nil
}

func (m *mockUsageLogStore) HourlyBreakdown(ctx context.Context, hours int) ([]HourlyUsage, error) {
	return m.hourlyBreakdown, nil
}

func (m *mockUsageLogStore) YesterdayCountByEndpoint(ctx context.Context) (map[string]int, error) {
	return m.yesterdayCounts, nil
}

func (m *mockUsageLogStore) PeriodUsage(ctx context.Context, period string) (*store.UsagePeriodResult, error) {
	return &store.UsagePeriodResult{ByModel: make(map[string]store.ModelUsage)}, nil
}

func (m *mockUsageLogStore) ListLogs(ctx context.Context, p store.UsageLogListParams) ([]store.UsageLogWithKeyName, int64, error) {
	return nil, 0, nil
}

func (m *mockUsageLogStore) TodayTokenTotals(ctx context.Context) (*store.TokenTotals, error) {
	return m.tokenTotals, m.tokenTotalsErr
}

func TestHandleTokenStats(t *testing.T) {
	ms := newMockTokenStore()
	// 3 active, 1 cooling, 1 expired
	ms.CreateToken(context.Background(), &store.Token{Token: "a1_xxxxxxxxxxxxxxxxxxxx", Pool: "ssoBasic", Status: store.TokenStatusActive})
	ms.CreateToken(context.Background(), &store.Token{Token: "a2_xxxxxxxxxxxxxxxxxxxx", Pool: "ssoBasic", Status: store.TokenStatusActive})
	ms.CreateToken(context.Background(), &store.Token{Token: "a3_xxxxxxxxxxxxxxxxxxxx", Pool: "ssoSuper", Status: store.TokenStatusActive})
	ms.CreateToken(context.Background(), &store.Token{Token: "c1_xxxxxxxxxxxxxxxxxxxx", Pool: "ssoBasic", Status: store.TokenStatusCooling})
	ms.CreateToken(context.Background(), &store.Token{Token: "e1_xxxxxxxxxxxxxxxxxxxx", Pool: "ssoSuper", Status: store.TokenStatusExpired})

	handler := handleTokenStats(ms)
	req := httptest.NewRequest(http.MethodGet, "/admin/stats/tokens", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp TokenStatsResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, 5, resp.Total)
	assert.Equal(t, 3, resp.Active)
	assert.Equal(t, 1, resp.Cooling)
	assert.Equal(t, 1, resp.Expired)
	assert.Equal(t, 0, resp.Disabled)
}

func TestHandleQuotaStats(t *testing.T) {
	ms := newMockTokenStore()
	ms.CreateToken(context.Background(), &store.Token{Token: "q1_xxxxxxxxxxxxxxxxxxxx", Pool: "ssoBasic", Status: store.TokenStatusActive, ChatQuota: 60, InitialChatQuota: 100, ImageQuota: 12, InitialImageQuota: 20, VideoQuota: 4, InitialVideoQuota: 5})
	ms.CreateToken(context.Background(), &store.Token{Token: "q2_xxxxxxxxxxxxxxxxxxxx", Pool: "ssoBasic", Status: store.TokenStatusActive, ChatQuota: 50, InitialChatQuota: 80, ImageQuota: 10, InitialImageQuota: 20, VideoQuota: 3, InitialVideoQuota: 5})
	ms.CreateToken(context.Background(), &store.Token{Token: "q3_xxxxxxxxxxxxxxxxxxxx", Pool: "ssoSuper", Status: store.TokenStatusActive, ChatQuota: 180, InitialChatQuota: 200, ImageQuota: 15, InitialImageQuota: 20, VideoQuota: 9, InitialVideoQuota: 10})
	ms.CreateToken(context.Background(), &store.Token{Token: "q4_xxxxxxxxxxxxxxxxxxxx", Pool: "ssoBasic", Status: store.TokenStatusDisabled, ChatQuota: 100}) // disabled, excluded

	handler := handleQuotaStats(ms, &config.TokenConfig{})
	req := httptest.NewRequest(http.MethodGet, "/admin/stats/quota", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp QuotaStatsResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Len(t, resp.Pools, 2)

	poolMap := make(map[string]PoolQuota)
	for _, p := range resp.Pools {
		poolMap[p.Pool] = p
	}

	basic := poolMap["ssoBasic"]
	assert.Equal(t, 180, basic.TotalChatQuota)
	assert.Equal(t, 110, basic.RemainingChatQuota)
	assert.Equal(t, 40, basic.TotalImageQuota)
	assert.Equal(t, 22, basic.RemainingImageQuota)
	assert.Equal(t, 10, basic.TotalVideoQuota)
	assert.Equal(t, 7, basic.RemainingVideoQuota)

	super := poolMap["ssoSuper"]
	assert.Equal(t, 200, super.TotalChatQuota)
	assert.Equal(t, 180, super.RemainingChatQuota)
	assert.Equal(t, 20, super.TotalImageQuota)
	assert.Equal(t, 15, super.RemainingImageQuota)
	assert.Equal(t, 10, super.TotalVideoQuota)
	assert.Equal(t, 9, super.RemainingVideoQuota)
}

func TestHandleUsageStats(t *testing.T) {
	t.Run("with yesterday data", func(t *testing.T) {
		ms := &mockUsageLogStore{
			todayCounts:     map[string]int{"chat": 100, "image": 20},
			yesterdayCounts: map[string]int{"chat": 80, "image": 10},
			hourlyBreakdown: []HourlyUsage{
				{Hour: "10", Endpoint: "chat", Count: 50},
				{Hour: "11", Endpoint: "chat", Count: 50},
			},
		}

		handler := handleUsageStats(ms)
		req := httptest.NewRequest(http.MethodGet, "/admin/stats/usage", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp UsageStatsResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, 100, resp.Today["chat"])
		assert.Equal(t, 20, resp.Today["image"])
		assert.Equal(t, 120, resp.Total)
		assert.Len(t, resp.Hourly, 2)

		// Delta: (100-80)/80*100 = 25%
		require.NotNil(t, resp.Delta["chat"])
		assert.Equal(t, 25.0, *resp.Delta["chat"])
		// Delta: (20-10)/10*100 = 100%
		require.NotNil(t, resp.Delta["image"])
		assert.Equal(t, 100.0, *resp.Delta["image"])
	})

	t.Run("null delta when yesterday is zero", func(t *testing.T) {
		ms := &mockUsageLogStore{
			todayCounts:     map[string]int{"chat": 50},
			yesterdayCounts: map[string]int{}, // no yesterday data
			hourlyBreakdown: []HourlyUsage{},
		}

		handler := handleUsageStats(ms)
		req := httptest.NewRequest(http.MethodGet, "/admin/stats/usage", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var resp UsageStatsResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Nil(t, resp.Delta["chat"])
	})
}

func TestHandleUsageStats_TokensToday(t *testing.T) {
	ms := &mockUsageLogStore{
		todayCounts:     map[string]int{"chat": 10},
		yesterdayCounts: map[string]int{},
		hourlyBreakdown: []HourlyUsage{},
		tokenTotals: &store.TokenTotals{
			Input:  500,
			Output: 1000,
			Cache:  200,
			Total:  1700,
		},
	}

	handler := handleUsageStats(ms)
	req := httptest.NewRequest(http.MethodGet, "/admin/stats/usage", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var raw map[string]any
	require.NoError(t, json.NewDecoder(w.Body).Decode(&raw))
	assert.Contains(t, raw, "tokens_today")

	tokensToday := raw["tokens_today"].(map[string]any)
	assert.Equal(t, float64(500), tokensToday["input"])
	assert.Equal(t, float64(1000), tokensToday["output"])
	assert.Equal(t, float64(200), tokensToday["cache"])
	assert.Equal(t, float64(1700), tokensToday["total"])
}
