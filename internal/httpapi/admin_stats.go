package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"sort"

	"github.com/crmmc/grokforge/internal/store"
)

// HourlyUsage represents usage count for a specific hour and endpoint.
type HourlyUsage = store.HourlyUsage

// UsageLogStoreInterface defines the methods needed for usage stats.
type UsageLogStoreInterface interface {
	Record(ctx context.Context, log *store.UsageLog) error
	TodayCountByEndpoint(ctx context.Context) (map[string]int, error)
	HourlyBreakdown(ctx context.Context, hours int) ([]HourlyUsage, error)
	YesterdayCountByEndpoint(ctx context.Context) (map[string]int, error)
	PeriodUsage(ctx context.Context, period string) (*store.UsagePeriodResult, error)
	ListLogs(ctx context.Context, p store.UsageLogListParams) ([]store.UsageLogWithKeyName, int64, error)
	TodayTokenTotals(ctx context.Context) (*store.TokenTotals, error)
}

// TokenStatsResponse is the response for token stats endpoint.
type TokenStatsResponse struct {
	Total    int `json:"total"`
	Active   int `json:"active"`
	Expired  int `json:"expired"`
	Disabled int `json:"disabled"`
}

// QuotaStatsResponse is the response for quota stats endpoint.
type QuotaStatsResponse struct {
	Pools []PoolQuota `json:"pools"`
}

// PoolQuota represents quota stats for a single pool.
type PoolQuota struct {
	Pool          string                      `json:"pool"`
	TokenCount    int                         `json:"token_count"`
	ActiveCount   int                         `json:"active_count"`
	DisabledCount int                         `json:"disabled_count"`
	ExpiredCount  int                         `json:"expired_count"`
	ModeQuotas    map[string]ModeQuotaSummary `json:"mode_quotas"`
}

// ModeQuotaSummary aggregates remaining and limit quota for a single mode.
type ModeQuotaSummary struct {
	TotalRemaining int `json:"total_remaining"`
	TotalLimit     int `json:"total_limit"`
}

// UsageStatsResponse is the response for usage stats endpoint.
type UsageStatsResponse struct {
	Today       map[string]int      `json:"today"`
	Total       int                 `json:"total"`
	Hourly      []HourlyUsage       `json:"hourly"`
	Delta       map[string]*float64 `json:"delta"`
	TokensToday *store.TokenTotals  `json:"tokens_today,omitempty"`
}

// handleTokenStats returns a handler that aggregates token counts by status.
func handleTokenStats(ts TokenStoreInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokens, err := ts.ListTokens(r.Context())
		if err != nil {
			WriteError(w, 500, "server_error", "stats_failed", "Failed to get token stats")
			return
		}

		resp := TokenStatsResponse{Total: len(tokens)}
		for _, t := range tokens {
			switch t.Status {
			case store.TokenStatusActive:
				resp.Active++
			case store.TokenStatusExpired:
				resp.Expired++
			case store.TokenStatusDisabled:
				resp.Disabled++
			}
		}

		WriteJSON(w, http.StatusOK, resp)
	}
}

// handleQuotaStats returns a handler that aggregates quota totals and remaining quota by pool for active tokens.
func handleQuotaStats(ts TokenStoreInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokens, err := ts.ListTokens(r.Context())
		if err != nil {
			WriteError(w, 500, "server_error", "stats_failed", "Failed to get quota stats")
			return
		}

		poolMap := make(map[string]*PoolQuota)
		for _, t := range tokens {
			pq, ok := poolMap[t.Pool]
			if !ok {
				pq = &PoolQuota{
					Pool:       t.Pool,
					ModeQuotas: make(map[string]ModeQuotaSummary),
				}
				poolMap[t.Pool] = pq
			}
			pq.TokenCount++
			switch t.Status {
			case store.TokenStatusActive:
				pq.ActiveCount++
			case store.TokenStatusDisabled:
				pq.DisabledCount++
			case store.TokenStatusExpired:
				pq.ExpiredCount++
			}

			// Only aggregate quotas for active tokens
			if t.Status != store.TokenStatusActive {
				continue
			}
			for mode, remaining := range t.Quotas {
				ms := pq.ModeQuotas[mode]
				ms.TotalRemaining += remaining
				if limit, ok := t.LimitQuotas[mode]; ok {
					ms.TotalLimit += limit
				}
				pq.ModeQuotas[mode] = ms
			}
		}

		pools := make([]PoolQuota, 0, len(poolMap))
		for _, pq := range poolMap {
			pools = append(pools, *pq)
		}
		sort.Slice(pools, func(i, j int) bool {
			return pools[i].Pool < pools[j].Pool
		})

		WriteJSON(w, http.StatusOK, QuotaStatsResponse{Pools: pools})
	}
}

// handleUsageStats returns a handler that returns today's usage with hourly breakdown and delta.
func handleUsageStats(us UsageLogStoreInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		todayCounts, err := us.TodayCountByEndpoint(r.Context())
		if err != nil {
			WriteError(w, 500, "server_error", "stats_failed", "Failed to get usage stats")
			return
		}

		hourly, err := us.HourlyBreakdown(r.Context(), 24)
		if err != nil {
			WriteError(w, 500, "server_error", "stats_failed", "Failed to get hourly breakdown")
			return
		}

		yesterdayCounts, err := us.YesterdayCountByEndpoint(r.Context())
		if err != nil {
			WriteError(w, 500, "server_error", "stats_failed", "Failed to get yesterday stats")
			return
		}

		// Compute total
		total := 0
		for _, c := range todayCounts {
			total += c
		}

		// Compute delta: (today - yesterday) / yesterday * 100
		delta := make(map[string]*float64)
		allEndpoints := make(map[string]bool)
		for ep := range todayCounts {
			allEndpoints[ep] = true
		}
		for ep := range yesterdayCounts {
			allEndpoints[ep] = true
		}
		for ep := range allEndpoints {
			yCount, hasYesterday := yesterdayCounts[ep]
			if !hasYesterday || yCount == 0 {
				delta[ep] = nil // null delta when no yesterday data
				continue
			}
			tCount := todayCounts[ep]
			d := float64(tCount-yCount) / float64(yCount) * 100
			delta[ep] = &d
		}

		// Get token totals for today (non-blocking -- log warning on failure)
		var tokenTotals *store.TokenTotals
		tt, err := us.TodayTokenTotals(r.Context())
		if err != nil {
			slog.Warn("TodayTokenTotals failed", "error", err)
		} else {
			tokenTotals = tt
		}

		WriteJSON(w, http.StatusOK, UsageStatsResponse{
			Today:       todayCounts,
			Total:       total,
			Hourly:      hourly,
			Delta:       delta,
			TokensToday: tokenTotals,
		})
	}
}
