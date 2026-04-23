package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/modelconfig"
	"github.com/crmmc/grokforge/internal/registry"
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
	// 2 active, 1 exhausted, 1 disabled, 1 expired
	ms.CreateToken(context.Background(), &store.Token{Token: "a1_xxxxxxxxxxxxxxxxxxxx", Pool: "ssoBasic", Status: store.TokenStatusActive})
	ms.CreateToken(context.Background(), &store.Token{Token: "a2_xxxxxxxxxxxxxxxxxxxx", Pool: "ssoBasic", Status: store.TokenStatusActive})
	ms.CreateToken(context.Background(), &store.Token{
		Token:       "a3_xxxxxxxxxxxxxxxxxxxx",
		Pool:        "ssoSuper",
		Status:      store.TokenStatusActive,
		Quotas:      store.IntMap{"auto": 0},
		LimitQuotas: store.IntMap{"auto": 50},
	})
	ms.CreateToken(context.Background(), &store.Token{Token: "d1_xxxxxxxxxxxxxxxxxxxx", Pool: "ssoBasic", Status: store.TokenStatusDisabled})
	ms.CreateToken(context.Background(), &store.Token{Token: "e1_xxxxxxxxxxxxxxxxxxxx", Pool: "ssoSuper", Status: store.TokenStatusExpired})

	handler := handleTokenStats(ms, nil)
	req := httptest.NewRequest(http.MethodGet, "/admin/stats/tokens", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp TokenStatsResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, 5, resp.Total)
	assert.Equal(t, 2, resp.Active)
	assert.Equal(t, 1, resp.Exhausted)
	assert.Equal(t, 1, resp.Expired)
	assert.Equal(t, 1, resp.Disabled)
}

func TestHandleQuotaStats(t *testing.T) {
	ms := newMockTokenStore()
	ms.CreateToken(context.Background(), &store.Token{Token: "q1_xxxxxxxxxxxxxxxxxxxx", Pool: "ssoBasic", Status: store.TokenStatusActive, Quotas: store.IntMap{"auto": 60}, LimitQuotas: store.IntMap{"auto": 100}})
	ms.CreateToken(context.Background(), &store.Token{Token: "q2_xxxxxxxxxxxxxxxxxxxx", Pool: "ssoBasic", Status: store.TokenStatusActive, Quotas: store.IntMap{"auto": 50}, LimitQuotas: store.IntMap{"auto": 80}})
	ms.CreateToken(context.Background(), &store.Token{Token: "q3_xxxxxxxxxxxxxxxxxxxx", Pool: "ssoSuper", Status: store.TokenStatusActive, Quotas: store.IntMap{"auto": 180}, LimitQuotas: store.IntMap{"auto": 200}})
	ms.CreateToken(context.Background(), &store.Token{Token: "q4_xxxxxxxxxxxxxxxxxxxx", Pool: "ssoBasic", Status: store.TokenStatusDisabled, Quotas: store.IntMap{"auto": 100}}) // disabled, excluded from quota aggregation

	handler := handleQuotaStats(ms, nil)
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
	assert.Equal(t, 110, basic.ModeQuotas["auto"].TotalRemaining)
	assert.Equal(t, 180, basic.ModeQuotas["auto"].TotalLimit)

	super := poolMap["ssoSuper"]
	assert.Equal(t, 180, super.ModeQuotas["auto"].TotalRemaining)
	assert.Equal(t, 200, super.ModeQuotas["auto"].TotalLimit)
}

func TestHandleQuotaStats_EmptyQuotas(t *testing.T) {
	ms := newMockTokenStore()
	ms.CreateToken(context.Background(), &store.Token{
		Token:  "q1_xxxxxxxxxxxxxxxxxxxx",
		Pool:   "ssoBasic",
		Status: store.TokenStatusActive,
	})

	handler := handleQuotaStats(ms, nil)
	req := httptest.NewRequest(http.MethodGet, "/admin/stats/quota", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp QuotaStatsResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Len(t, resp.Pools, 1)
	assert.Equal(t, 1, resp.Pools[0].ActiveCount)
}

func TestHandleQuotaStats_FiltersUnknownModesByRegistry(t *testing.T) {
	ms := newMockTokenStore()
	ms.CreateToken(context.Background(), &store.Token{
		Token:       "q1_xxxxxxxxxxxxxxxxxxxx",
		Pool:        "ssoBasic",
		Status:      store.TokenStatusActive,
		Quotas:      store.IntMap{"auto": 10, "legacy": 99},
		LimitQuotas: store.IntMap{"auto": 20, "legacy": 99},
	})

	reg := registry.NewTestRegistry(
		[]modelconfig.ModelSpec{
			{
				ID:         "grok-4.20",
				Type:       modelconfig.TypeChat,
				Enabled:    true,
				PoolFloor:  modelconfig.PoolBasic,
				Mode:       "auto",
				PublicType: "chat",
			},
		},
		[]modelconfig.ModeSpec{
			{
				ID:            "auto",
				UpstreamName:  "auto",
				WindowSeconds: 7200,
				DefaultQuota: map[string]int{
					"basic": 20,
					"super": 50,
					"heavy": 150,
				},
			},
		},
	)

	handler := handleQuotaStats(ms, reg)
	req := httptest.NewRequest(http.MethodGet, "/admin/stats/quota", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp QuotaStatsResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Len(t, resp.Pools, 1)
	assert.Contains(t, resp.Pools[0].ModeQuotas, "auto")
	assert.NotContains(t, resp.Pools[0].ModeQuotas, "legacy")
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
