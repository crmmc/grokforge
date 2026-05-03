package token

import (
	"errors"
	"log/slog"
	"time"

	"github.com/crmmc/grokforge/internal/modelconfig"
)

const (
	refreshFailureBackoffDivisor = 4
	refreshFailureMinBackoff     = time.Minute
)

func (s *Scheduler) applyRateLimits(
	tokenID uint,
	mode modelconfig.ModeSpec,
	resp *RateLimitsResponse,
	now time.Time,
) (remaining int, limit int, resumeAt int) {
	remaining, limit = clampRateLimits(resp)
	s.manager.UpdateModeQuota(tokenID, mode.ID, remaining, limit)
	if remaining > 0 {
		s.manager.ClearResumeAt(tokenID, mode.ID)
		return remaining, limit, 0
	}
	resumeAt = calculateResumeAt(now, resp, mode)
	s.manager.SetResumeAt(tokenID, mode.ID, resumeAt)
	return remaining, limit, resumeAt
}

func clampRateLimits(resp *RateLimitsResponse) (remaining int, limit int) {
	limit = resp.TotalQueries
	if limit < 0 {
		limit = 0
	}
	remaining = resp.RemainingQueries
	if remaining < 0 {
		remaining = 0
	}
	if remaining > limit {
		remaining = limit
	}
	return remaining, limit
}

func calculateResumeAt(now time.Time, resp *RateLimitsResponse, mode modelconfig.ModeSpec) int {
	waitSeconds := resp.WaitTimeSeconds
	if waitSeconds <= 0 {
		waitSeconds = resp.WindowSizeSeconds
	}
	if waitSeconds <= 0 {
		waitSeconds = mode.WindowSeconds
	}
	return int(now.Add(time.Duration(waitSeconds) * time.Second).Unix())
}

func (s *Scheduler) applyRefreshFailureBackoff(
	target ExhaustedModeTarget,
	mode modelconfig.ModeSpec,
	now time.Time,
) int {
	resumeAt := calculateRefreshFailureResumeAt(now, mode)
	changed := s.manager.SetResumeAtIfDue(target.TokenID, mode.ID, resumeAt, int(now.Unix()))
	if changed {
		slog.Debug("refresh: failure backoff applied",
			"token_id", target.TokenID,
			"pool", target.Pool,
			"mode", mode.ID,
			"resume_at", resumeAt)
	}
	return resumeAt
}

func calculateRefreshFailureResumeAt(now time.Time, mode modelconfig.ModeSpec) int {
	backoff := time.Duration(mode.WindowSeconds) * time.Second / refreshFailureBackoffDivisor
	if backoff < refreshFailureMinBackoff {
		backoff = refreshFailureMinBackoff
	}
	return int(now.Add(backoff).Unix())
}

func (s *Scheduler) logRefreshFailure(
	target ExhaustedModeTarget,
	mode modelconfig.ModeSpec,
	err error,
) {
	statusCode, preview, truncated := refreshFailureDetails(err)
	slog.Warn("refresh: mode quota sync failed",
		"token_id", target.TokenID,
		"pool", target.Pool,
		"mode", mode.ID,
		"upstream_name", mode.UpstreamName,
		"action", "refresh_failed",
		"http_status", statusCode,
		"response_body_preview", preview,
		"response_body_truncated", truncated,
		"error", err)
}

func refreshFailureDetails(err error) (int, string, bool) {
	var httpErr *rateLimitsHTTPError
	if errors.As(err, &httpErr) {
		return httpErr.statusCode, httpErr.bodyPreview, httpErr.bodyTruncated
	}
	return 0, "", false
}
