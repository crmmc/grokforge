package httpapi

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"strconv"

	"github.com/crmmc/grokforge/internal/store"
)

// UsageLogStoreForUsage is a focused interface for the system usage and usage logs endpoints.
// Separate from UsageLogStoreInterface which serves the Dashboard stats.
type UsageLogStoreForUsage interface {
	PeriodUsage(ctx context.Context, period string) (*store.UsagePeriodResult, error)
	ListLogs(ctx context.Context, p store.UsageLogListParams) ([]store.UsageLogWithKeyName, int64, error)
}

// SystemUsageResponse matches the frontend UsageStats type.
type SystemUsageResponse struct {
	Period       string                      `json:"period"`
	Requests     int                         `json:"requests"`
	TokensInput  int                         `json:"tokens_input"`
	TokensOutput int                         `json:"tokens_output"`
	CacheTokens  int                         `json:"cache_tokens"`
	Errors       int                         `json:"errors"`
	ByModel      map[string]store.ModelUsage `json:"by_model"`
	ByAPIKey     []store.APIKeyUsage         `json:"by_api_key"`
}

// validPeriods defines accepted period values.
var validPeriods = map[string]bool{
	"hour":  true,
	"day":   true,
	"week":  true,
	"month": true,
}

// handleSystemUsage returns a handler for GET /admin/system/usage.
func handleSystemUsage(uls UsageLogStoreForUsage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		period := r.URL.Query().Get("period")
		if period == "" {
			period = "day"
		}
		if !validPeriods[period] {
			WriteError(w, 400, "invalid_request_error", "invalid_period",
				"Invalid period: must be hour, day, week, or month")
			return
		}

		result, err := uls.PeriodUsage(r.Context(), period)
		if err != nil {
			WriteError(w, 500, "server_error", "usage_failed", "Failed to get usage stats")
			return
		}

		byModel := result.ByModel
		if byModel == nil {
			byModel = make(map[string]store.ModelUsage)
		}

		byAPIKey := result.ByAPIKey
		if byAPIKey == nil {
			byAPIKey = []store.APIKeyUsage{}
		}

		WriteJSON(w, http.StatusOK, SystemUsageResponse{
			Period:       period,
			Requests:     result.Requests,
			TokensInput:  result.TokensInput,
			TokensOutput: result.TokensOutput,
			CacheTokens:  result.CacheTokens,
			Errors:       result.Errors,
			ByModel:      byModel,
			ByAPIKey:     byAPIKey,
		})
	}
}

// UsageLogsPaginatedResponse is the paginated response for usage logs.
type UsageLogsPaginatedResponse struct {
	Data       []store.UsageLogWithKeyName `json:"data"`
	Total      int64                       `json:"total"`
	Page       int                         `json:"page"`
	PageSize   int                         `json:"page_size"`
	TotalPages int                         `json:"total_pages"`
}

// handleUsageLogs returns a handler for GET /admin/usage/logs.
func handleUsageLogs(uls UsageLogStoreForUsage) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()

		page := queryInt(q.Get("page"), 1)
		pageSize := queryInt(q.Get("page_size"), 20)

		// Clamp values
		if page < 1 {
			page = 1
		}
		if pageSize < 1 {
			pageSize = 1
		}
		if pageSize > 100 {
			pageSize = 100
		}

		params := store.UsageLogListParams{
			Page:       page,
			PageSize:   pageSize,
			SortBy:     q.Get("sort_by"),
			SortDir:    q.Get("sort_dir"),
			Model:      q.Get("model"),
			Period:     q.Get("period"),
			Status:     q.Get("status"),
			APIKeyName: q.Get("api_key"),
		}

		// Default sort
		if params.SortBy == "" {
			params.SortBy = "time"
		}
		if params.SortDir == "" {
			params.SortDir = "desc"
		}

		logs, total, err := uls.ListLogs(r.Context(), params)
		if err != nil {
			slog.Error("ListLogs failed", "error", err)
			WriteError(w, 500, "server_error", "list_logs_failed", "Failed to list usage logs")
			return
		}

		if logs == nil {
			logs = []store.UsageLogWithKeyName{}
		}

		totalPages := 0
		if total > 0 {
			totalPages = int(math.Ceil(float64(total) / float64(pageSize)))
		}

		WriteJSON(w, http.StatusOK, UsageLogsPaginatedResponse{
			Data:       logs,
			Total:      total,
			Page:       page,
			PageSize:   pageSize,
			TotalPages: totalPages,
		})
	}
}

// queryInt parses an integer from a query string value, returning defaultVal if empty or invalid.
func queryInt(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return defaultVal
	}
	return v
}
