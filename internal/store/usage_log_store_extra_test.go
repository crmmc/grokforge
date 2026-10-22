package store

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func seedUsageLog(t *testing.T, db *gorm.DB) *UsageLogStore {
	t.Helper()
	s := NewUsageLogStore(db)
	require.NoError(t, s.Record(context.Background(), &UsageLog{
		TokenID:      1,
		Model:        "grok-3",
		Endpoint:     "chat",
		Status:       200,
		TokensInput:  10,
		TokensOutput: 20,
		CreatedAt:    time.Now().UTC(),
	}))
	return s
}

func TestUsageLogStore_QueryErrorPaths(t *testing.T) {
	tests := []struct {
		name string
		act  func(ctx context.Context, s *UsageLogStore) error
	}{
		{
			name: "today count by endpoint fails",
			act: func(ctx context.Context, s *UsageLogStore) error {
				_, err := s.TodayCountByEndpoint(ctx)
				return err
			},
		},
		{
			name: "hourly breakdown fails",
			act: func(ctx context.Context, s *UsageLogStore) error {
				_, err := s.HourlyBreakdown(ctx, 24)
				return err
			},
		},
		{
			name: "yesterday count by endpoint fails",
			act: func(ctx context.Context, s *UsageLogStore) error {
				_, err := s.YesterdayCountByEndpoint(ctx)
				return err
			},
		},
		{
			name: "today token totals fails",
			act: func(ctx context.Context, s *UsageLogStore) error {
				_, err := s.TodayTokenTotals(ctx)
				return err
			},
		},
		{
			name: "period usage fails",
			act: func(ctx context.Context, s *UsageLogStore) error {
				_, err := s.PeriodUsage(ctx, "day")
				return err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := NewUsageLogStore(closedDB(t))
			assert.Error(t, tc.act(context.Background(), s))
		})
	}
}

func TestUsageLogStore_PeriodUsage_QueryErrors(t *testing.T) {
	// gorm routes PeriodUsage's Count through the "query" processor and its
	// three Scan calls through the "row" processor (Scan delegates to Rows).
	tests := []struct {
		name      string
		processor string
		failAt    int
	}{
		{name: "success stats scan fails", processor: "row", failAt: 1},
		{name: "error count query fails", processor: "query", failAt: 1},
		{name: "by model scan fails", processor: "row", failAt: 2},
		{name: "by api key scan fails", processor: "row", failAt: 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := setupUsageLogTestDB(t)
			seedUsageLog(t, db)
			if tc.processor == "query" {
				failOnNthQuery(t, db, tc.failAt)
			} else {
				failOnNthRowScan(t, db, tc.failAt)
			}

			result, err := NewUsageLogStore(db).PeriodUsage(context.Background(), "day")
			assert.ErrorIs(t, err, errInjected)
			assert.Nil(t, result)
		})
	}
}

func TestUsageLogStore_PeriodUsage_WeekAndMonth(t *testing.T) {
	tests := []struct {
		name   string
		period string
	}{
		{name: "week window includes today", period: "week"},
		{name: "month window includes today", period: "month"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := setupUsageLogTestDB(t)
			s := seedUsageLog(t, db)

			result, err := s.PeriodUsage(context.Background(), tc.period)
			require.NoError(t, err)
			assert.Equal(t, 1, result.Requests)
			assert.Equal(t, 10, result.TokensInput)
			assert.Equal(t, 20, result.TokensOutput)
		})
	}
}

func TestUsageLogStore_HourlyBreakdown_PostgresDialect(t *testing.T) {
	db := setupUsageLogTestDB(t)
	seedUsageLog(t, db)

	// Reuse the same sqlite connection pool behind a postgres dialector so the
	// postgres hour expression branch runs; the query itself then fails because
	// sqlite has no to_char function.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	pgDB, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	require.NoError(t, err)

	pgStore := NewUsageLogStore(pgDB)
	hourly, err := pgStore.HourlyBreakdown(context.Background(), 24)
	assert.Error(t, err, "to_char does not exist in sqlite")
	assert.Empty(t, hourly)

	// The sqlite-dialect path on the same data still succeeds.
	sqliteStore := NewUsageLogStore(db)
	hourly, err = sqliteStore.HourlyBreakdown(context.Background(), 24)
	require.NoError(t, err)
	assert.NotEmpty(t, hourly)
}

func TestUsageLogStore_ListLogs_QueryErrors(t *testing.T) {
	tests := []struct {
		name   string
		failAt int
	}{
		{name: "count query fails", failAt: 1},
		{name: "find query fails", failAt: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := setupUsageLogTestDB(t)
			seedUsageLog(t, db)
			failOnNthQuery(t, db, tc.failAt)

			logs, total, err := NewUsageLogStore(db).ListLogs(context.Background(), UsageLogListParams{Page: 1, PageSize: 10})
			assert.ErrorIs(t, err, errInjected)
			assert.Zero(t, total)
			assert.Nil(t, logs)
		})
	}
}

func seedListLogsFilterData(t *testing.T, db *gorm.DB, s *UsageLogStore) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	keyAlpha := &APIKey{Name: "alpha-key", Key: "sk-alpha"}
	keyBeta := &APIKey{Name: "beta-key", Key: "sk-beta"}
	require.NoError(t, db.Create(keyAlpha).Error)
	require.NoError(t, db.Create(keyBeta).Error)

	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, APIKeyID: keyAlpha.ID, Model: "grok-3", Endpoint: "chat", Status: 200, CreatedAt: now}))
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 2, APIKeyID: keyBeta.ID, Model: "grok-3-mini", Endpoint: "chat", Status: 500, CreatedAt: now.Add(-time.Minute)}))
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, APIKeyID: 0, Model: "grok-3", Endpoint: "chat", Status: 200, CreatedAt: now.Add(-2 * time.Minute)}))
}

func TestUsageLogStore_ListLogs_Filters(t *testing.T) {
	db := setupUsageLogTestDB(t)
	s := NewUsageLogStore(db)
	seedListLogsFilterData(t, db, s)
	ctx := context.Background()

	tests := []struct {
		name      string
		params    UsageLogListParams
		wantTotal int64
	}{
		{
			name:      "status prefix 5 matches errors only",
			params:    UsageLogListParams{Page: 1, PageSize: 20, Status: "5"},
			wantTotal: 1,
		},
		{
			name:      "status prefix 2 matches successes",
			params:    UsageLogListParams{Page: 1, PageSize: 20, Status: "2"},
			wantTotal: 2,
		},
		{
			name:      "api key name partial match",
			params:    UsageLogListParams{Page: 1, PageSize: 20, APIKeyName: "alpha"},
			wantTotal: 1,
		},
		{
			name:      "api key name without match",
			params:    UsageLogListParams{Page: 1, PageSize: 20, APIKeyName: "does-not-exist"},
			wantTotal: 0,
		},
		{
			name:      "combined model and period filters",
			params:    UsageLogListParams{Page: 1, PageSize: 20, Model: "mini", Period: "day"},
			wantTotal: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logs, total, err := s.ListLogs(ctx, tc.params)
			require.NoError(t, err)
			assert.Equal(t, tc.wantTotal, total)
			assert.Len(t, logs, int(total))
		})
	}
}

func TestValidSortDir(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "asc", want: "asc"},
		{in: "desc", want: "desc"},
		{in: "", want: "desc"},
		{in: "bogus", want: "desc"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, validSortDir(tc.in))
		})
	}
}

func TestPeriodToSince(t *testing.T) {
	tests := []struct {
		name   string
		period string
		pinned time.Time
		want   time.Time
	}{
		{
			name:   "hour is one hour back",
			period: "hour",
			pinned: time.Date(2026, 9, 25, 14, 30, 45, 0, time.UTC),
			want:   time.Date(2026, 9, 25, 13, 30, 45, 0, time.UTC),
		},
		{
			name:   "week starts on monday",
			period: "week",
			pinned: time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC), // Wednesday
			want:   time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC),
		},
		{
			name:   "week on sunday wraps to previous monday",
			period: "week",
			pinned: time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC), // Sunday
			want:   time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC),
		},
		{
			name:   "month starts on the first",
			period: "month",
			pinned: time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC),
			want:   time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:   "unknown period falls back to day",
			period: "bogus",
			pinned: time.Date(2026, 9, 25, 23, 59, 59, 0, time.UTC),
			want:   time.Date(2026, 9, 25, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orig := nowFunc
			nowFunc = func() time.Time { return tc.pinned }
			t.Cleanup(func() { nowFunc = orig })

			assert.Equal(t, tc.want, periodToSince(tc.period))
		})
	}
}

func TestUsageLogStore_RecordOnFileDB(t *testing.T) {
	// OpenSQLite is the production entrypoint; exercise it end-to-end here.
	db, err := OpenSQLite(t.TempDir() + "/usage.db")
	require.NoError(t, err)
	t.Cleanup(func() { _ = Close(db) })
	require.NoError(t, AutoMigrate(db))

	s := NewUsageLogStore(db)
	ctx := context.Background()
	require.NoError(t, s.Record(ctx, &UsageLog{TokenID: 1, Model: "grok-3", Endpoint: "chat", Status: 200, CreatedAt: time.Now()}))

	counts, err := s.TodayCountByEndpoint(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, counts["chat"])
}
