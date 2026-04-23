// Package flow provides chat orchestration and retry logic.
package flow

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"regexp"
	"strings"
	"time"

	"github.com/crmmc/grokforge/internal/xai"
)

// ErrRetryBudgetExceeded indicates retry time budget has been exhausted.
var ErrRetryBudgetExceeded = errors.New("retry budget exhausted")

// RetryConfig holds retry behavior configuration.
type RetryConfig struct {
	// MaxTokens is the maximum number of different tokens to try.
	MaxTokens int

	// PerTokenRetries is the number of retries per token before switching.
	PerTokenRetries int

	// BaseDelay is the initial backoff delay.
	BaseDelay time.Duration

	// MaxDelay is the maximum backoff delay.
	MaxDelay time.Duration

	// JitterFactor is the jitter range as a fraction of delay (e.g., 0.25 = +/-25%).
	JitterFactor float64

	// BackoffFactor controls exponential backoff growth (e.g., 2.0 = doubling).
	BackoffFactor float64

	// ResetSessionStatusCodes are HTTP status codes that require session reset before retry.
	ResetSessionStatusCodes []int

	// RetryBudget caps total retry time. Zero means no budget.
	RetryBudget time.Duration
}

// DefaultRetryConfig returns sensible default retry configuration.
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxTokens:               5,
		PerTokenRetries:         2,
		BaseDelay:               time.Second,
		MaxDelay:                30 * time.Second,
		JitterFactor:            0.25,
		BackoffFactor:           2.0,
		ResetSessionStatusCodes: []int{403},
	}
}

// BackoffWithJitter calculates backoff delay with exponential growth and jitter.
// Formula: min(base * 2^attempt, max) * (1 +/- jitter)
func BackoffWithJitter(attempt int, cfg *RetryConfig) time.Duration {
	backoffFactor := cfg.BackoffFactor
	if backoffFactor <= 0 {
		backoffFactor = 1
	}
	base := float64(cfg.BaseDelay)
	delay := time.Duration(base * math.Pow(backoffFactor, float64(attempt)))

	// Cap at max
	if delay > cfg.MaxDelay {
		delay = cfg.MaxDelay
	}

	// Apply jitter: +/- (JitterFactor * delay)
	jitterRange := float64(delay) * cfg.JitterFactor
	jitter := (rand.Float64()*2 - 1) * jitterRange // -jitterRange to +jitterRange

	return time.Duration(float64(delay) + jitter)
}

// IsNonRecoverable returns true if the error should NOT be retried.
// Non-recoverable: context errors, ErrInvalidToken (401), 400 Bad Request.
func IsNonRecoverable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if errors.Is(err, xai.ErrInvalidToken) {
		return true
	}
	if statusCode, ok := extractStatusCode(err); ok {
		if statusCode == 400 || statusCode == 401 {
			return true
		}
	}
	return false
}

// IsRetryable returns true if the error is retryable (all errors except non-recoverable).
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	return !IsNonRecoverable(err)
}

// ShouldSwapToken returns true if the error should trigger an immediate token swap.
// CF challenge does NOT swap (same token can retry after session reset).
// Token-level 403 (ErrForbidden) swaps because the token is bad.
func ShouldSwapToken(err error, cfg *RetryConfig) bool {
	if errors.Is(err, xai.ErrForbidden) {
		return true // token-level 403 → swap immediately
	}
	if errors.Is(err, xai.ErrCFChallenge) {
		return false // CF challenge → same token, just reset session
	}
	return ShouldCoolToken(err, cfg)
}

// ShouldResetSession returns true if a session reset should be attempted.
// CF challenge always resets session. Token-level 403 does NOT (token is bad, not session).
func ShouldResetSession(err error, cfg *RetryConfig) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, xai.ErrCFChallenge) {
		return true // CF challenge always needs session reset
	}
	if errors.Is(err, xai.ErrForbidden) {
		return false // token-level 403 → no session reset needed
	}
	resetCodes := defaultResetCodes(cfg)
	if statusCode, ok := extractStatusCode(err); ok {
		return containsInt(resetCodes, statusCode)
	}
	return false
}

func defaultResetCodes(cfg *RetryConfig) []int {
	if cfg == nil || len(cfg.ResetSessionStatusCodes) == 0 {
		return []int{403}
	}
	return cfg.ResetSessionStatusCodes
}

// ShouldCoolToken returns true only for 429 quota exhaustion semantics.
func ShouldCoolToken(err error, _ *RetryConfig) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, xai.ErrRateLimited) {
		return true
	}

	if statusCode, ok := extractStatusCode(err); ok {
		return statusCode == 429
	}

	// Fallback: check error message for rate limit text
	msg := err.Error()
	if strings.Contains(msg, xai.ErrRateLimited.Error()) {
		return true
	}

	return false
}

func containsInt(values []int, target int) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

var statusCodePattern = regexp.MustCompile(`\b(\d{3})\b`)

func extractStatusCode(err error) (int, bool) {
	matches := statusCodePattern.FindStringSubmatch(err.Error())
	if len(matches) < 2 {
		return 0, false
	}
	code := 0
	for _, ch := range matches[1] {
		code = code*10 + int(ch-'0')
	}
	if code < 100 || code > 599 {
		return 0, false
	}
	return code, true
}

// IsCFChallenge returns true if the error is a Cloudflare challenge.
func IsCFChallenge(err error) bool {
	return errors.Is(err, xai.ErrCFChallenge)
}
