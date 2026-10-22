package token

import (
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/modelconfig"
	"github.com/stretchr/testify/assert"
)

func TestClampRateLimits(t *testing.T) {
	cases := []struct {
		name          string
		resp          RateLimitsResponse
		wantRemaining int
		wantLimit     int
	}{
		{"positive values pass through", RateLimitsResponse{RemainingQueries: 7, TotalQueries: 10}, 7, 10},
		{"negative limit clamps to zero", RateLimitsResponse{RemainingQueries: 0, TotalQueries: -5}, 0, 0},
		{"negative remaining clamps to zero", RateLimitsResponse{RemainingQueries: -3, TotalQueries: 10}, 0, 10},
		{"remaining above limit clamps", RateLimitsResponse{RemainingQueries: 99, TotalQueries: 10}, 10, 10},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			remaining, limit := clampRateLimits(&tc.resp)
			assert.Equal(t, tc.wantRemaining, remaining)
			assert.Equal(t, tc.wantLimit, limit)
		})
	}
}

func TestCalculateRefreshFailureResumeAt_MinBackoffClamp(t *testing.T) {
	now := time.Now()
	mode := modelconfig.ModeSpec{WindowSeconds: 60} // 60/4 = 15s, below the 1min minimum

	resumeAt := calculateRefreshFailureResumeAt(now, mode)

	assert.InDelta(t, now.Add(time.Minute).Unix(), int64(resumeAt), 2)
}
