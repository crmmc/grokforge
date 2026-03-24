package store

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// HourlyUsage represents usage count for a specific hour and endpoint.
type HourlyUsage struct {
	Hour     string `json:"hour"`
	Endpoint string `json:"endpoint"`
	Count    int    `json:"count"`
}

// UsageLogStore handles usage log persistence and queries.
type UsageLogStore struct {
	db *gorm.DB
}

// NewUsageLogStore creates a new UsageLogStore.
func NewUsageLogStore(db *gorm.DB) *UsageLogStore {
	return &UsageLogStore{db: db}
}

// Record inserts a usage log record.
func (s *UsageLogStore) Record(ctx context.Context, log *UsageLog) error {
	return s.db.WithContext(ctx).Create(log).Error
}

// BatchInsert inserts multiple usage log records in batches of 100.
func (s *UsageLogStore) BatchInsert(ctx context.Context, logs []*UsageLog) error {
	if len(logs) == 0 {
		return nil
	}
	return s.db.WithContext(ctx).CreateInBatches(logs, 100).Error
}

// TodayCountByEndpoint returns success request counts grouped by endpoint for today.
// Only counts requests with status < 400.
func (s *UsageLogStore) TodayCountByEndpoint(ctx context.Context) (map[string]int, error) {
	todayStart := todayStart()

	var results []struct {
		Endpoint string
		Count    int
	}
	err := s.db.WithContext(ctx).
		Model(&UsageLog{}).
		Select("endpoint, COUNT(*) as count").
		Where("created_at >= ? AND status < 400", todayStart).
		Group("endpoint").
		Find(&results).Error
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int)
	for _, r := range results {
		counts[r.Endpoint] = r.Count
	}
	return counts, nil
}

// HourlyBreakdown returns usage counts grouped by hour and endpoint for the past N hours.
// Uses CASE expression to support both SQLite (substr) and PostgreSQL (to_char).
func (s *UsageLogStore) HourlyBreakdown(ctx context.Context, hours int) ([]HourlyUsage, error) {
	since := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)

	// Detect dialect: PostgreSQL uses to_char, SQLite uses strftime
	hourExpr := "substr(created_at, 12, 2)"
	if s.db.Dialector.Name() == "postgres" {
		hourExpr = "to_char(created_at, 'HH24')"
	}

	var results []HourlyUsage
	err := s.db.WithContext(ctx).
		Model(&UsageLog{}).
		Select(fmt.Sprintf("%s as hour, endpoint, COUNT(*) as count", hourExpr)).
		Where("created_at >= ?", since).
		Group("hour, endpoint").
		Order("hour ASC").
		Find(&results).Error
	if err != nil {
		return nil, err
	}
	return results, nil
}

// YesterdayCountByEndpoint returns success request counts grouped by endpoint for yesterday.
// Only counts requests with status < 400.
func (s *UsageLogStore) YesterdayCountByEndpoint(ctx context.Context) (map[string]int, error) {
	todayStart := todayStart()
	yesterdayStart := todayStart.Add(-24 * time.Hour)

	var results []struct {
		Endpoint string
		Count    int
	}
	err := s.db.WithContext(ctx).
		Model(&UsageLog{}).
		Select("endpoint, COUNT(*) as count").
		Where("created_at >= ? AND created_at < ? AND status < 400", yesterdayStart, todayStart).
		Group("endpoint").
		Find(&results).Error
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int)
	for _, r := range results {
		counts[r.Endpoint] = r.Count
	}
	return counts, nil
}

// ModelUsage represents per-model usage stats.
type ModelUsage struct {
	Requests     int `json:"requests"`
	TokensInput  int `json:"tokens_input"`
	TokensOutput int `json:"tokens_output"`
}

// APIKeyUsage represents per-API-key usage stats.
type APIKeyUsage struct {
	APIKeyName   string `json:"api_key_name"`
	Requests     int    `json:"requests"`
	TokensInput  int    `json:"tokens_input"`
	TokensOutput int    `json:"tokens_output"`
}

// UsagePeriodResult holds aggregated usage stats for a time period.
type UsagePeriodResult struct {
	Requests     int                   `json:"requests"`
	TokensInput  int                   `json:"tokens_input"`
	TokensOutput int                   `json:"tokens_output"`
	CacheTokens  int                   `json:"cache_tokens"`
	Errors       int                   `json:"errors"`
	ByModel      map[string]ModelUsage `json:"by_model"`
	ByAPIKey     []APIKeyUsage         `json:"by_api_key"`
}

// periodToSince converts a period string to a since timestamp.
func periodToSince(period string) time.Time {
	now := time.Now().UTC()
	switch period {
	case "hour":
		return now.Add(-1 * time.Hour)
	case "week":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7 // Sunday = 7
		}
		return time.Date(now.Year(), now.Month(), now.Day()-(weekday-1), 0, 0, 0, 0, time.UTC)
	case "month":
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	default: // "day" or invalid
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	}
}

// PeriodUsage returns aggregated usage stats for the given period.
func (s *UsageLogStore) PeriodUsage(ctx context.Context, period string) (*UsagePeriodResult, error) {
	since := periodToSince(period)

	result := &UsagePeriodResult{
		ByModel: make(map[string]ModelUsage),
	}

	// Query 1: success stats (status < 400)
	var successRow struct {
		Requests     int
		TokensInput  int
		TokensOutput int
		CacheTokens  int
	}
	err := s.db.WithContext(ctx).
		Model(&UsageLog{}).
		Select("COUNT(*) as requests, COALESCE(SUM(tokens_input),0) as tokens_input, COALESCE(SUM(tokens_output),0) as tokens_output, COALESCE(SUM(cache_tokens),0) as cache_tokens").
		Where("created_at >= ? AND status < 400", since).
		Scan(&successRow).Error
	if err != nil {
		return nil, err
	}
	result.Requests = successRow.Requests
	result.TokensInput = successRow.TokensInput
	result.TokensOutput = successRow.TokensOutput
	result.CacheTokens = successRow.CacheTokens

	// Query 2: error count (status >= 400)
	var errCount int64
	err = s.db.WithContext(ctx).
		Model(&UsageLog{}).
		Where("created_at >= ? AND status >= 400", since).
		Count(&errCount).Error
	if err != nil {
		return nil, err
	}
	result.Errors = int(errCount)

	// Query 3: by_model breakdown (status < 400)
	var modelRows []struct {
		Model        string
		Requests     int
		TokensInput  int
		TokensOutput int
	}
	err = s.db.WithContext(ctx).
		Model(&UsageLog{}).
		Select("model, COUNT(*) as requests, COALESCE(SUM(tokens_input),0) as tokens_input, COALESCE(SUM(tokens_output),0) as tokens_output").
		Where("created_at >= ? AND status < 400", since).
		Group("model").
		Scan(&modelRows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range modelRows {
		result.ByModel[row.Model] = ModelUsage{
			Requests:     row.Requests,
			TokensInput:  row.TokensInput,
			TokensOutput: row.TokensOutput,
		}
	}

	// Query 4: by_api_key breakdown (status < 400)
	var apiKeyRows []struct {
		APIKeyName   string
		Requests     int
		TokensInput  int
		TokensOutput int
	}
	err = s.db.WithContext(ctx).
		Table("usage_logs").
		Select("COALESCE(api_keys.name, 'unknown') as api_key_name, COUNT(*) as requests, COALESCE(SUM(usage_logs.tokens_input),0) as tokens_input, COALESCE(SUM(usage_logs.tokens_output),0) as tokens_output").
		Joins("LEFT JOIN api_keys ON api_keys.id = usage_logs.api_key_id").
		Where("usage_logs.created_at >= ? AND usage_logs.status < 400", since).
		Group("usage_logs.api_key_id").
		Scan(&apiKeyRows).Error
	if err != nil {
		return nil, err
	}
	for _, row := range apiKeyRows {
		result.ByAPIKey = append(result.ByAPIKey, APIKeyUsage{
			APIKeyName:   row.APIKeyName,
			Requests:     row.Requests,
			TokensInput:  row.TokensInput,
			TokensOutput: row.TokensOutput,
		})
	}

	return result, nil
}

// todayStart returns the start of today (midnight UTC).
func todayStart() time.Time {
	now := time.Now().UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

// UsageLogListParams holds parameters for ListLogs pagination, filtering, sorting.
type UsageLogListParams struct {
	Page       int
	PageSize   int
	SortBy     string
	SortDir    string
	Model      string // partial match
	Period     string
	Status     string // partial match on status code (e.g. "4" matches 400,401...)
	APIKeyName string // partial match on api_keys.name
}

// UsageLogWithKeyName embeds UsageLog with resolved API key name via JOIN.
type UsageLogWithKeyName struct {
	UsageLog
	APIKeyName string `json:"api_key_name"`
}

// sortColumnWhitelist maps allowed sort keys to fully-qualified column names.
var sortColumnWhitelist = map[string]string{
	"time":          "usage_logs.created_at",
	"model":         "usage_logs.model",
	"ttft":          "usage_logs.ttft_ms",
	"duration":      "usage_logs.duration_ms",
	"tokens_input":  "usage_logs.tokens_input",
	"tokens_output": "usage_logs.tokens_output",
	"cache_tokens":  "usage_logs.cache_tokens",
	"status":        "usage_logs.status",
}

// validSortDir returns "asc" or "desc" (default "desc").
func validSortDir(s string) string {
	if s == "asc" {
		return "asc"
	}
	return "desc"
}

// ListLogs returns paginated usage logs with api_key_name resolved via LEFT JOIN.
func (s *UsageLogStore) ListLogs(ctx context.Context, p UsageLogListParams) ([]UsageLogWithKeyName, int64, error) {
	// Build base query with JOIN (needed for APIKeyName filter)
	baseQuery := s.db.WithContext(ctx).
		Table("usage_logs").
		Joins("LEFT JOIN api_keys ON api_keys.id = usage_logs.api_key_id")

	// Apply filters
	if p.Model != "" {
		baseQuery = baseQuery.Where("usage_logs.model LIKE ?", "%"+p.Model+"%")
	}
	if p.Period != "" {
		since := periodToSince(p.Period)
		baseQuery = baseQuery.Where("usage_logs.created_at >= ?", since)
	}
	if p.Status != "" {
		baseQuery = baseQuery.Where("CAST(usage_logs.status AS TEXT) LIKE ?", p.Status+"%")
	}
	if p.APIKeyName != "" {
		baseQuery = baseQuery.Where("api_keys.name LIKE ?", "%"+p.APIKeyName+"%")
	}

	// Count total matching rows
	var total int64
	if err := baseQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Build sort clause
	col, colValid := sortColumnWhitelist[p.SortBy]
	if !colValid {
		col = "usage_logs.created_at"
	}
	dir := "desc"
	if colValid {
		dir = validSortDir(p.SortDir)
	}
	orderClause := fmt.Sprintf("%s %s", col, dir)

	var results []UsageLogWithKeyName
	err := baseQuery.Session(&gorm.Session{}).
		Select("usage_logs.*, COALESCE(api_keys.name, '') as api_key_name").
		Order(orderClause).
		Offset((p.Page - 1) * p.PageSize).
		Limit(p.PageSize).
		Find(&results).Error
	if err != nil {
		return nil, 0, err
	}

	return results, total, nil
}

// TokenTotals holds today's aggregated token usage.
type TokenTotals struct {
	Input  int `json:"input"`
	Output int `json:"output"`
	Cache  int `json:"cache"`
	Total  int `json:"total"`
}

// TodayTokenTotals returns aggregated token totals for today (success only).
func (s *UsageLogStore) TodayTokenTotals(ctx context.Context) (*TokenTotals, error) {
	start := todayStart()

	var row struct {
		Input  int
		Output int
		Cache  int
	}
	err := s.db.WithContext(ctx).
		Model(&UsageLog{}).
		Select("COALESCE(SUM(tokens_input),0) as input, COALESCE(SUM(tokens_output),0) as output, COALESCE(SUM(cache_tokens),0) as cache").
		Where("created_at >= ? AND status < 400", start).
		Scan(&row).Error
	if err != nil {
		return nil, err
	}

	return &TokenTotals{
		Input:  row.Input,
		Output: row.Output,
		Cache:  row.Cache,
		Total:  row.Input + row.Output + row.Cache,
	}, nil
}
