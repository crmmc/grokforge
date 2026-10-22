package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingUsageStore is a UsageLogStoreInterface fake with per-method error injection.
type failingUsageStore struct {
	mockUsageLogStore

	todayErr     error
	hourlyErr    error
	yesterdayErr error
	totalsErr    error
	periodErr    error
	listErr      error
}

func (f *failingUsageStore) TodayCountByEndpoint(ctx context.Context) (map[string]int, error) {
	if f.todayErr != nil {
		return nil, f.todayErr
	}
	return f.mockUsageLogStore.TodayCountByEndpoint(ctx)
}

func (f *failingUsageStore) HourlyBreakdown(ctx context.Context, hours int) ([]HourlyUsage, error) {
	if f.hourlyErr != nil {
		return nil, f.hourlyErr
	}
	return f.mockUsageLogStore.HourlyBreakdown(ctx, hours)
}

func (f *failingUsageStore) YesterdayCountByEndpoint(ctx context.Context) (map[string]int, error) {
	if f.yesterdayErr != nil {
		return nil, f.yesterdayErr
	}
	return f.mockUsageLogStore.YesterdayCountByEndpoint(ctx)
}

func (f *failingUsageStore) TodayTokenTotals(ctx context.Context) (*store.TokenTotals, error) {
	if f.totalsErr != nil {
		return nil, f.totalsErr
	}
	return f.mockUsageLogStore.TodayTokenTotals(ctx)
}

func (f *failingUsageStore) PeriodUsage(ctx context.Context, period string) (*store.UsagePeriodResult, error) {
	if f.periodErr != nil {
		return nil, f.periodErr
	}
	return f.mockUsageLogStore.PeriodUsage(ctx, period)
}

func (f *failingUsageStore) ListLogs(ctx context.Context, p store.UsageLogListParams) ([]store.UsageLogWithKeyName, int64, error) {
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	return f.mockUsageLogStore.ListLogs(ctx, p)
}

func TestHandleTokenStats_ListError(t *testing.T) {
	ts := &errTokenStore{mockTokenStore: newMockTokenStore(), listErr: errors.New("db")}
	w := httptest.NewRecorder()
	handleTokenStats(ts, nil)(w, httptest.NewRequest(http.MethodGet, "/admin/stats/tokens", nil))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleQuotaStats_Extra(t *testing.T) {
	t.Run("list error returns 500", func(t *testing.T) {
		ts := &errTokenStore{mockTokenStore: newMockTokenStore(), listErr: errors.New("db")}
		w := httptest.NewRecorder()
		handleQuotaStats(ts, nil)(w, httptest.NewRequest(http.MethodGet, "/admin/stats/quota", nil))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("non standard status counted but quota skipped", func(t *testing.T) {
		ts := newMockTokenStore()
		require.NoError(t, ts.CreateToken(context.Background(), &store.Token{
			Token:  longToken("s_"),
			Pool:   "ssoBasic",
			Status: "cooldown",
			Quotas: store.IntMap{"auto": 42},
		}))

		w := httptest.NewRecorder()
		handleQuotaStats(ts, nil)(w, httptest.NewRequest(http.MethodGet, "/admin/stats/quota", nil))
		require.Equal(t, http.StatusOK, w.Code)

		var resp QuotaStatsResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		require.Len(t, resp.Pools, 1)
		assert.Equal(t, 1, resp.Pools[0].TokenCount)
		assert.Equal(t, 0, resp.Pools[0].ActiveCount)
		assert.Empty(t, resp.Pools[0].ModeQuotas, "quota aggregation skipped for non-active tokens")
	})
}

func TestHandleUsageStats_Extra(t *testing.T) {
	t.Run("today error returns 500", func(t *testing.T) {
		us := &failingUsageStore{todayErr: errors.New("db")}
		w := httptest.NewRecorder()
		handleUsageStats(us)(w, httptest.NewRequest(http.MethodGet, "/admin/stats/usage", nil))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("hourly error returns 500", func(t *testing.T) {
		us := &failingUsageStore{hourlyErr: errors.New("db")}
		w := httptest.NewRecorder()
		handleUsageStats(us)(w, httptest.NewRequest(http.MethodGet, "/admin/stats/usage", nil))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("yesterday error returns 500", func(t *testing.T) {
		us := &failingUsageStore{yesterdayErr: errors.New("db")}
		w := httptest.NewRecorder()
		handleUsageStats(us)(w, httptest.NewRequest(http.MethodGet, "/admin/stats/usage", nil))
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("token totals error is non fatal", func(t *testing.T) {
		us := &failingUsageStore{
			mockUsageLogStore: mockUsageLogStore{
				todayCounts:     map[string]int{"chat": 3},
				yesterdayCounts: map[string]int{"chat": 1},
			},
			totalsErr: errors.New("db"),
		}
		w := httptest.NewRecorder()
		handleUsageStats(us)(w, httptest.NewRequest(http.MethodGet, "/admin/stats/usage", nil))
		require.Equal(t, http.StatusOK, w.Code)

		var resp UsageStatsResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Nil(t, resp.TokensToday)
		require.NotNil(t, resp.Delta["chat"])
	})
}

func TestHandleQuotaStats_ExpiredStatusCounted(t *testing.T) {
	ts := newMockTokenStore()
	require.NoError(t, ts.CreateToken(context.Background(), &store.Token{
		Token:  longToken("x_"),
		Pool:   "ssoBasic",
		Status: store.TokenStatusExpired,
	}))

	w := httptest.NewRecorder()
	handleQuotaStats(ts, nil)(w, httptest.NewRequest(http.MethodGet, "/admin/stats/quota", nil))
	require.Equal(t, http.StatusOK, w.Code)

	var resp QuotaStatsResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	require.Len(t, resp.Pools, 1)
	assert.Equal(t, 1, resp.Pools[0].ExpiredCount)
}
