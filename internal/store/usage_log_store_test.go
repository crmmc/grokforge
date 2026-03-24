package store

import (
	"context"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupUsageLogTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))
	return db
}

func TestUsageLogStore_Record(t *testing.T) {
	db := setupUsageLogTestDB(t)
	s := NewUsageLogStore(db)
	ctx := context.Background()

	log := &UsageLog{
		TokenID:    1,
		Model:      "grok-3",
		Endpoint:   "chat",
		Status:     200,
		DurationMs: 150,
		CreatedAt:  time.Now(),
	}
	err := s.Record(ctx, log)
	require.NoError(t, err)
	assert.NotZero(t, log.ID)
}

func TestUsageLogStore_TodayCountByEndpoint(t *testing.T) {
	db := setupUsageLogTestDB(t)
	s := NewUsageLogStore(db)
	ctx := context.Background()

	// Use todayStart + 1h to ensure "today" records are well within today's UTC boundary
	todayTime := todayStart().Add(1 * time.Hour)
	yesterdayTime := todayStart().Add(-1 * time.Hour) // 1h before today's UTC midnight = yesterday

	// Today: 2 chat (1 success, 1 error), 1 image success
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Endpoint: "chat", Status: 200, CreatedAt: todayTime}))
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Endpoint: "chat", Status: 500, CreatedAt: todayTime})) // should be excluded
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Endpoint: "image", Status: 200, CreatedAt: todayTime}))
	// Yesterday: should not count
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Endpoint: "chat", Status: 200, CreatedAt: yesterdayTime}))

	counts, err := s.TodayCountByEndpoint(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, counts["chat"])
	assert.Equal(t, 1, counts["image"])
	assert.Equal(t, 0, counts["video"])
}

func TestUsageLogStore_HourlyBreakdown(t *testing.T) {
	db := setupUsageLogTestDB(t)
	s := NewUsageLogStore(db)
	ctx := context.Background()

	now := time.Now()

	// Insert records in current hour
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Endpoint: "chat", Status: 200, CreatedAt: now}))
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Endpoint: "chat", Status: 200, CreatedAt: now}))
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Endpoint: "image", Status: 200, CreatedAt: now}))

	hourly, err := s.HourlyBreakdown(ctx, 24)
	require.NoError(t, err)
	assert.NotEmpty(t, hourly)

	// Find current hour entries
	currentHour := now.Format("15")
	chatCount := 0
	imageCount := 0
	for _, h := range hourly {
		if h.Hour == currentHour && h.Endpoint == "chat" {
			chatCount = h.Count
		}
		if h.Hour == currentHour && h.Endpoint == "image" {
			imageCount = h.Count
		}
	}
	assert.Equal(t, 2, chatCount)
	assert.Equal(t, 1, imageCount)
}

func TestUsageLogStore_PeriodUsage_Day(t *testing.T) {
	db := setupUsageLogTestDB(t)
	s := NewUsageLogStore(db)
	ctx := context.Background()

	now := time.Now().UTC()
	yesterday := now.Add(-25 * time.Hour)

	// Today: 2 success with different models and tokens
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3", Endpoint: "chat", Status: 200, DurationMs: 100, TokensInput: 50, TokensOutput: 100, CreatedAt: now}))
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3-mini", Endpoint: "chat", Status: 200, DurationMs: 80, TokensInput: 30, TokensOutput: 60, CreatedAt: now}))
	// Today: 1 error
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3", Endpoint: "chat", Status: 500, DurationMs: 50, TokensInput: 10, TokensOutput: 0, CreatedAt: now}))
	// Yesterday: should not count
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3", Endpoint: "chat", Status: 200, DurationMs: 100, TokensInput: 999, TokensOutput: 999, CreatedAt: yesterday}))

	result, err := s.PeriodUsage(ctx, "day")
	require.NoError(t, err)
	assert.Equal(t, 2, result.Requests)       // only status < 400
	assert.Equal(t, 80, result.TokensInput)   // 50 + 30
	assert.Equal(t, 160, result.TokensOutput) // 100 + 60
	assert.Equal(t, 1, result.Errors)         // status >= 400
	assert.Len(t, result.ByModel, 2)
	assert.Equal(t, 1, result.ByModel["grok-3"].Requests)
	assert.Equal(t, 1, result.ByModel["grok-3-mini"].Requests)
}

func TestUsageLogStore_PeriodUsage_Hour(t *testing.T) {
	db := setupUsageLogTestDB(t)
	s := NewUsageLogStore(db)
	ctx := context.Background()

	now := time.Now().UTC()
	twoHoursAgo := now.Add(-2 * time.Hour)

	// Within last hour
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3", Endpoint: "chat", Status: 200, TokensInput: 10, TokensOutput: 20, CreatedAt: now}))
	// 2 hours ago: should not count for "hour"
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3", Endpoint: "chat", Status: 200, TokensInput: 999, TokensOutput: 999, CreatedAt: twoHoursAgo}))

	result, err := s.PeriodUsage(ctx, "hour")
	require.NoError(t, err)
	assert.Equal(t, 1, result.Requests)
	assert.Equal(t, 10, result.TokensInput)
	assert.Equal(t, 20, result.TokensOutput)
}

func TestUsageLogStore_PeriodUsage_Errors(t *testing.T) {
	db := setupUsageLogTestDB(t)
	s := NewUsageLogStore(db)
	ctx := context.Background()

	now := time.Now().UTC()

	// Various error statuses
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3", Endpoint: "chat", Status: 400, CreatedAt: now}))
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3", Endpoint: "chat", Status: 429, CreatedAt: now}))
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3", Endpoint: "chat", Status: 500, CreatedAt: now}))
	// Success
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3", Endpoint: "chat", Status: 200, CreatedAt: now}))

	result, err := s.PeriodUsage(ctx, "day")
	require.NoError(t, err)
	assert.Equal(t, 3, result.Errors)   // 400, 429, 500
	assert.Equal(t, 1, result.Requests) // only 200
}

func TestUsageLogStore_PeriodUsage_ByModel(t *testing.T) {
	db := setupUsageLogTestDB(t)
	s := NewUsageLogStore(db)
	ctx := context.Background()

	now := time.Now().UTC()

	// grok-3: 2 requests
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3", Endpoint: "chat", Status: 200, TokensInput: 100, TokensOutput: 200, CreatedAt: now}))
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3", Endpoint: "chat", Status: 200, TokensInput: 150, TokensOutput: 300, CreatedAt: now}))
	// grok-3-mini: 1 request
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3-mini", Endpoint: "chat", Status: 200, TokensInput: 50, TokensOutput: 80, CreatedAt: now}))
	// grok-3 error: should NOT be in by_model
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3", Endpoint: "chat", Status: 500, TokensInput: 10, TokensOutput: 0, CreatedAt: now}))

	result, err := s.PeriodUsage(ctx, "day")
	require.NoError(t, err)
	assert.Len(t, result.ByModel, 2)

	g3 := result.ByModel["grok-3"]
	assert.Equal(t, 2, g3.Requests)
	assert.Equal(t, 250, g3.TokensInput)  // 100 + 150
	assert.Equal(t, 500, g3.TokensOutput) // 200 + 300

	mini := result.ByModel["grok-3-mini"]
	assert.Equal(t, 1, mini.Requests)
	assert.Equal(t, 50, mini.TokensInput)
	assert.Equal(t, 80, mini.TokensOutput)
}

func TestUsageLogStore_PeriodUsage_InvalidPeriod(t *testing.T) {
	db := setupUsageLogTestDB(t)
	s := NewUsageLogStore(db)
	ctx := context.Background()

	now := time.Now().UTC()
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3", Endpoint: "chat", Status: 200, TokensInput: 10, TokensOutput: 20, CreatedAt: now}))

	// Invalid period should default to "day" behavior
	result, err := s.PeriodUsage(ctx, "invalid")
	require.NoError(t, err)
	assert.Equal(t, 1, result.Requests)
	assert.Equal(t, 10, result.TokensInput)
	assert.Equal(t, 20, result.TokensOutput)
}

func TestUsageLogStore_YesterdayCountByEndpoint(t *testing.T) {
	db := setupUsageLogTestDB(t)
	s := NewUsageLogStore(db)
	ctx := context.Background()

	todayTime := todayStart().Add(1 * time.Hour)
	yesterdayTime := todayStart().Add(-12 * time.Hour) // well within yesterday's UTC range

	// Yesterday records
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Endpoint: "chat", Status: 200, CreatedAt: yesterdayTime}))
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Endpoint: "chat", Status: 200, CreatedAt: yesterdayTime}))
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Endpoint: "image", Status: 200, CreatedAt: yesterdayTime}))
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Endpoint: "chat", Status: 500, CreatedAt: yesterdayTime})) // excluded

	// Today records (should not count)
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Endpoint: "chat", Status: 200, CreatedAt: todayTime}))

	counts, err := s.YesterdayCountByEndpoint(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, counts["chat"])
	assert.Equal(t, 1, counts["image"])
}

func TestUsageLogStore_BatchInsert(t *testing.T) {
	db := setupUsageLogTestDB(t)
	s := NewUsageLogStore(db)
	ctx := context.Background()

	now := time.Now()
	logs := []*UsageLog{
		{TokenID: 1, Model: "grok-3", Endpoint: "chat", Status: 200, DurationMs: 100, CreatedAt: now},
		{TokenID: 2, Model: "grok-3-mini", Endpoint: "chat", Status: 200, DurationMs: 80, CreatedAt: now},
		{TokenID: 3, Model: "grok-3", Endpoint: "image", Status: 200, DurationMs: 500, CreatedAt: now},
		{TokenID: 4, Model: "grok-3", Endpoint: "chat", Status: 500, DurationMs: 50, CreatedAt: now},
		{TokenID: 5, Model: "grok-3", Endpoint: "video", Status: 200, DurationMs: 3000, CreatedAt: now},
	}

	err := s.BatchInsert(ctx, logs)
	require.NoError(t, err)

	// Verify all 5 records were created
	var count int64
	db.Model(&UsageLog{}).Count(&count)
	assert.Equal(t, int64(5), count)

	// Verify individual record IDs are set
	for _, log := range logs {
		assert.NotZero(t, log.ID)
	}
}

func TestUsageLogStore_BatchInsert_Empty(t *testing.T) {
	db := setupUsageLogTestDB(t)
	s := NewUsageLogStore(db)
	ctx := context.Background()

	err := s.BatchInsert(ctx, []*UsageLog{})
	require.NoError(t, err)

	err = s.BatchInsert(ctx, nil)
	require.NoError(t, err)
}

// --- ListLogs tests ---

func seedListLogsData(t *testing.T, db *gorm.DB, s *UsageLogStore) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	yesterday := now.Add(-25 * time.Hour)

	// Create api_keys for JOIN resolution
	db.Create(&APIKey{Name: "test-key-1"})
	db.Create(&APIKey{Name: "test-key-2"})

	// Today: 4 records with different models, api_keys, token values
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, APIKeyID: 1, Model: "grok-3", Endpoint: "chat", Status: 200, TokensInput: 100, TokensOutput: 200, CacheTokens: 50, DurationMs: 150, TTFTMs: 30, CreatedAt: now}))
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 2, APIKeyID: 2, Model: "grok-3-mini", Endpoint: "chat", Status: 200, TokensInput: 50, TokensOutput: 80, CacheTokens: 20, DurationMs: 80, TTFTMs: 15, CreatedAt: now.Add(-1 * time.Minute)}))
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, APIKeyID: 1, Model: "grok-3", Endpoint: "image", Status: 200, TokensInput: 200, TokensOutput: 400, CacheTokens: 0, DurationMs: 500, TTFTMs: 100, CreatedAt: now.Add(-2 * time.Minute)}))
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, APIKeyID: 0, Model: "grok-3", Endpoint: "chat", Status: 500, TokensInput: 10, TokensOutput: 0, CacheTokens: 0, DurationMs: 50, TTFTMs: 0, CreatedAt: now.Add(-3 * time.Minute)}))
	// Yesterday record
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, APIKeyID: 1, Model: "grok-3", Endpoint: "chat", Status: 200, TokensInput: 999, TokensOutput: 999, CacheTokens: 100, DurationMs: 100, TTFTMs: 20, CreatedAt: yesterday}))
}

func TestListLogs_Default(t *testing.T) {
	db := setupUsageLogTestDB(t)
	s := NewUsageLogStore(db)
	seedListLogsData(t, db, s)

	logs, total, err := s.ListLogs(context.Background(), UsageLogListParams{
		Page:     1,
		PageSize: 20,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, logs, 5)
	// Default order: created_at DESC -- newest first
	assert.Equal(t, "grok-3", logs[0].Model)
	// First record should have api_key_name resolved
	assert.Equal(t, "test-key-1", logs[0].APIKeyName)
	// Record with APIKeyID=0 should have empty api_key_name
	found := false
	for _, l := range logs {
		if l.APIKeyID == 0 {
			assert.Empty(t, l.APIKeyName)
			found = true
		}
	}
	assert.True(t, found, "should find record with APIKeyID=0")
}

func TestListLogs_Pagination(t *testing.T) {
	db := setupUsageLogTestDB(t)
	s := NewUsageLogStore(db)
	seedListLogsData(t, db, s)

	// Page 1: 2 items
	logs1, total, err := s.ListLogs(context.Background(), UsageLogListParams{
		Page:     1,
		PageSize: 2,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(5), total)
	assert.Len(t, logs1, 2)

	// Page 2: 2 items
	logs2, _, err := s.ListLogs(context.Background(), UsageLogListParams{
		Page:     2,
		PageSize: 2,
	})
	require.NoError(t, err)
	assert.Len(t, logs2, 2)

	// Page 3: 1 item (last)
	logs3, _, err := s.ListLogs(context.Background(), UsageLogListParams{
		Page:     3,
		PageSize: 2,
	})
	require.NoError(t, err)
	assert.Len(t, logs3, 1)

	// No overlap between pages
	assert.NotEqual(t, logs1[0].ID, logs2[0].ID)
	assert.NotEqual(t, logs2[0].ID, logs3[0].ID)
}

func TestListLogs_FilterModel(t *testing.T) {
	db := setupUsageLogTestDB(t)
	s := NewUsageLogStore(db)
	seedListLogsData(t, db, s)

	logs, total, err := s.ListLogs(context.Background(), UsageLogListParams{
		Page:     1,
		PageSize: 20,
		Model:    "grok-3",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(5), total) // 4 grok-3 + 1 grok-3-mini (LIKE partial match)
	for _, l := range logs {
		assert.Contains(t, l.Model, "grok-3")
	}
}

func TestListLogs_FilterPeriod(t *testing.T) {
	db := setupUsageLogTestDB(t)
	s := NewUsageLogStore(db)
	seedListLogsData(t, db, s)

	logs, total, err := s.ListLogs(context.Background(), UsageLogListParams{
		Page:     1,
		PageSize: 20,
		Period:   "day",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(4), total) // 4 today records
	assert.Len(t, logs, 4)
}

func TestListLogs_Sort(t *testing.T) {
	db := setupUsageLogTestDB(t)
	s := NewUsageLogStore(db)
	seedListLogsData(t, db, s)

	logs, _, err := s.ListLogs(context.Background(), UsageLogListParams{
		Page:     1,
		PageSize: 20,
		SortBy:   "tokens_input",
		SortDir:  "asc",
	})
	require.NoError(t, err)
	assert.True(t, len(logs) > 1)
	// Verify ascending order
	for i := 1; i < len(logs); i++ {
		assert.LessOrEqual(t, logs[i-1].TokensInput, logs[i].TokensInput)
	}
}

func TestListLogs_InvalidSort(t *testing.T) {
	db := setupUsageLogTestDB(t)
	s := NewUsageLogStore(db)
	seedListLogsData(t, db, s)

	// Unknown sortBy should fall back to created_at DESC
	logs, _, err := s.ListLogs(context.Background(), UsageLogListParams{
		Page:     1,
		PageSize: 20,
		SortBy:   "bobby_tables; DROP TABLE usage_logs;--",
		SortDir:  "asc",
	})
	require.NoError(t, err)
	assert.True(t, len(logs) > 1)
	// Should be ordered by created_at DESC (default), ignoring invalid sortDir for invalid column
	// The newest record should come first
	assert.True(t, logs[0].CreatedAt.After(logs[len(logs)-1].CreatedAt) || logs[0].CreatedAt.Equal(logs[len(logs)-1].CreatedAt))
}

func TestPeriodUsage_CacheTokens(t *testing.T) {
	db := setupUsageLogTestDB(t)
	s := NewUsageLogStore(db)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3", Endpoint: "chat", Status: 200, TokensInput: 100, TokensOutput: 200, CacheTokens: 50, CreatedAt: now}))
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3", Endpoint: "chat", Status: 200, TokensInput: 50, TokensOutput: 80, CacheTokens: 30, CreatedAt: now}))
	// Error should not count
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3", Endpoint: "chat", Status: 500, TokensInput: 10, TokensOutput: 0, CacheTokens: 99, CreatedAt: now}))

	result, err := s.PeriodUsage(ctx, "day")
	require.NoError(t, err)
	assert.Equal(t, 80, result.CacheTokens) // 50 + 30, excludes error's 99
}

func TestPeriodUsage_ByAPIKey(t *testing.T) {
	db := setupUsageLogTestDB(t)
	s := NewUsageLogStore(db)
	ctx := context.Background()
	now := time.Now().UTC()

	// Create API keys and capture their IDs
	keyAlpha := &APIKey{Name: "key-alpha", Key: "sk-alpha"}
	keyBeta := &APIKey{Name: "key-beta", Key: "sk-beta"}
	db.Create(keyAlpha)
	db.Create(keyBeta)

	// key-alpha: 2 requests
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, APIKeyID: keyAlpha.ID, Model: "grok-3", Endpoint: "chat", Status: 200, TokensInput: 100, TokensOutput: 200, CreatedAt: now}))
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, APIKeyID: keyAlpha.ID, Model: "grok-3", Endpoint: "chat", Status: 200, TokensInput: 50, TokensOutput: 80, CreatedAt: now}))
	// key-beta: 1 request
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 2, APIKeyID: keyBeta.ID, Model: "grok-3-mini", Endpoint: "chat", Status: 200, TokensInput: 30, TokensOutput: 60, CreatedAt: now}))
	// Error record (should be excluded)
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, APIKeyID: keyAlpha.ID, Model: "grok-3", Endpoint: "chat", Status: 500, TokensInput: 10, TokensOutput: 0, CreatedAt: now}))

	result, err := s.PeriodUsage(ctx, "day")
	require.NoError(t, err)
	require.NotNil(t, result.ByAPIKey)
	assert.Len(t, result.ByAPIKey, 2)

	// Build map for easier assertion
	byKey := make(map[string]APIKeyUsage)
	for _, k := range result.ByAPIKey {
		byKey[k.APIKeyName] = k
	}

	alpha := byKey["key-alpha"]
	assert.Equal(t, 2, alpha.Requests)
	assert.Equal(t, 150, alpha.TokensInput)  // 100 + 50
	assert.Equal(t, 280, alpha.TokensOutput) // 200 + 80

	beta := byKey["key-beta"]
	assert.Equal(t, 1, beta.Requests)
	assert.Equal(t, 30, beta.TokensInput)
	assert.Equal(t, 60, beta.TokensOutput)
}

func TestListLogs_Estimated(t *testing.T) {
	db := setupUsageLogTestDB(t)
	s := NewUsageLogStore(db)
	ctx := context.Background()
	now := time.Now().UTC()

	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3", Endpoint: "chat", Status: 200, TokensInput: 100, TokensOutput: 200, Estimated: true, CreatedAt: now}))
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3", Endpoint: "chat", Status: 200, TokensInput: 50, TokensOutput: 80, Estimated: false, CreatedAt: now}))

	logs, total, err := s.ListLogs(ctx, UsageLogListParams{Page: 1, PageSize: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)

	// Newest first (estimated=true was inserted first but same time, check both exist)
	estimatedCount := 0
	for _, l := range logs {
		if l.Estimated {
			estimatedCount++
		}
	}
	assert.Equal(t, 1, estimatedCount)
}

func TestTodayTokenTotals(t *testing.T) {
	db := setupUsageLogTestDB(t)
	s := NewUsageLogStore(db)
	ctx := context.Background()
	now := time.Now().UTC()
	yesterday := now.Add(-25 * time.Hour)

	// Today success records
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3", Endpoint: "chat", Status: 200, TokensInput: 100, TokensOutput: 200, CacheTokens: 50, CreatedAt: now}))
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3-mini", Endpoint: "chat", Status: 200, TokensInput: 50, TokensOutput: 80, CacheTokens: 20, CreatedAt: now}))
	// Today error (excluded)
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3", Endpoint: "chat", Status: 500, TokensInput: 10, TokensOutput: 5, CacheTokens: 99, CreatedAt: now}))
	// Yesterday (excluded)
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3", Endpoint: "chat", Status: 200, TokensInput: 999, TokensOutput: 999, CacheTokens: 999, CreatedAt: yesterday}))

	totals, err := s.TodayTokenTotals(ctx)
	require.NoError(t, err)
	assert.Equal(t, 150, totals.Input)  // 100 + 50
	assert.Equal(t, 280, totals.Output) // 200 + 80
	assert.Equal(t, 70, totals.Cache)   // 50 + 20
	assert.Equal(t, 500, totals.Total)  // 150 + 280 + 70
}
