package flow

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/crmmc/grokforge/internal/xai"
)

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	if cfg.MaxTokens != 5 {
		t.Errorf("MaxTokens = %d, want 5", cfg.MaxTokens)
	}
	if cfg.PerTokenRetries != 2 {
		t.Errorf("PerTokenRetries = %d, want 2", cfg.PerTokenRetries)
	}
	if cfg.BaseDelay != time.Second {
		t.Errorf("BaseDelay = %v, want 1s", cfg.BaseDelay)
	}
	if cfg.MaxDelay != 30*time.Second {
		t.Errorf("MaxDelay = %v, want 30s", cfg.MaxDelay)
	}
	if cfg.JitterFactor != 0.25 {
		t.Errorf("JitterFactor = %v, want 0.25", cfg.JitterFactor)
	}
	if cfg.BackoffFactor != 2.0 {
		t.Errorf("BackoffFactor = %v, want 2.0", cfg.BackoffFactor)
	}
	if !reflect.DeepEqual(cfg.ResetSessionStatusCodes, []int{403}) {
		t.Errorf("ResetSessionStatusCodes = %v, want [403]", cfg.ResetSessionStatusCodes)
	}
	if !reflect.DeepEqual(cfg.CoolingStatusCodes, []int{429}) {
		t.Errorf("CoolingStatusCodes = %v, want [429]", cfg.CoolingStatusCodes)
	}
}

func TestBackoffWithJitter_Attempt0(t *testing.T) {
	cfg := DefaultRetryConfig()

	// attempt=0 should return ~base (1s +/- 25%)
	for i := 0; i < 10; i++ {
		d := BackoffWithJitter(0, cfg)
		if d < 750*time.Millisecond || d > 1250*time.Millisecond {
			t.Errorf("attempt=0: got %v, want between 750ms and 1250ms", d)
		}
	}
}

func TestBackoffWithJitter_ExponentialGrowth(t *testing.T) {
	cfg := DefaultRetryConfig()

	d0 := BackoffWithJitter(0, cfg)
	d1 := BackoffWithJitter(1, cfg)
	d2 := BackoffWithJitter(2, cfg)

	if d1 < 1500*time.Millisecond || d1 > 2500*time.Millisecond {
		t.Errorf("attempt=1: got %v, want between 1.5s and 2.5s", d1)
	}
	if d2 < 3*time.Second || d2 > 5*time.Second {
		t.Errorf("attempt=2: got %v, want between 3s and 5s", d2)
	}

	_ = d0
}

func TestBackoffWithJitter_CapsAtMax(t *testing.T) {
	cfg := DefaultRetryConfig()

	for i := 0; i < 10; i++ {
		d := BackoffWithJitter(5, cfg)
		if d < 22500*time.Millisecond || d > 37500*time.Millisecond {
			t.Errorf("attempt=5: got %v, want between 22.5s and 37.5s", d)
		}
	}

	for i := 0; i < 10; i++ {
		d := BackoffWithJitter(10, cfg)
		if d < 22500*time.Millisecond || d > 37500*time.Millisecond {
			t.Errorf("attempt=10: got %v, want between 22.5s and 37.5s", d)
		}
	}
}

func TestIsRetryable_RetryableErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"rate limited", xai.ErrRateLimited, true},
		{"network error", xai.ErrNetwork, true},
		{"forbidden", xai.ErrForbidden, true},
		{"cf challenge", xai.ErrCFChallenge, true},
		{"502 error", errors.New("server returned 502"), true},
		{"503 error", errors.New("503 Service Unavailable"), true},
		{"504 error", errors.New("504 Gateway Timeout"), true},
		{"403 error", errors.New("403 Forbidden"), true},
		{"429 error", errors.New("429 Too Many Requests"), true},
		{"wrapped rate limit", errors.New("request failed: " + xai.ErrRateLimited.Error()), true},
		{"unknown error", errors.New("something went wrong"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.want {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsRetryable_NonRetryableErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"invalid token", xai.ErrInvalidToken, false},
		{"context canceled", context.Canceled, false},
		{"deadline exceeded", context.DeadlineExceeded, false},
		{"nil error", nil, false},
		{"401 error", errors.New("401 Unauthorized"), false},
		{"400 error", errors.New("400 Bad Request"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.want {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsNonRecoverable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"context canceled", context.Canceled, true},
		{"deadline exceeded", context.DeadlineExceeded, true},
		{"invalid token", xai.ErrInvalidToken, true},
		{"400 bad request", errors.New("400 Bad Request"), true},
		{"401 unauthorized", errors.New("401 Unauthorized"), true},
		{"rate limited", xai.ErrRateLimited, false},
		{"forbidden", xai.ErrForbidden, false},
		{"cf challenge", xai.ErrCFChallenge, false},
		{"502 error", errors.New("server returned 502"), false},
		{"network error", xai.ErrNetwork, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsNonRecoverable(tt.err); got != tt.want {
				t.Errorf("IsNonRecoverable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestShouldCoolToken(t *testing.T) {
	tests := []struct {
		name string
		err  error
		cfg  *RetryConfig
		want bool
	}{
		{"nil error", nil, nil, false},
		{"rate limited with defaults", xai.ErrRateLimited, nil, true},
		{"forbidden with defaults", xai.ErrForbidden, nil, false},
		{"cf challenge with defaults", xai.ErrCFChallenge, nil, false},
		{"502 gateway error with defaults", errors.New("server returned 502"), nil, false},
		{"503 service unavailable with defaults", errors.New("503 Service Unavailable"), nil, false},
		{"504 gateway timeout with defaults", errors.New("504 Gateway Timeout"), nil, false},
		{"network error", xai.ErrNetwork, nil, false},
		{"rate limited message in error", errors.New("request failed: " + xai.ErrRateLimited.Error()), nil, true},
		{"custom cooling codes includes 502", errors.New("server returned 502"), &RetryConfig{CoolingStatusCodes: []int{429, 502}}, true},
		{"custom cooling codes excludes 429", xai.ErrRateLimited, &RetryConfig{CoolingStatusCodes: []int{403}}, false},
		{"forbidden excluded from cooling", xai.ErrForbidden, &RetryConfig{CoolingStatusCodes: []int{429}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldCoolToken(tt.err, tt.cfg); got != tt.want {
				t.Errorf("ShouldCoolToken(%v, %v) = %v, want %v", tt.err, tt.cfg, got, tt.want)
			}
		})
	}
}

func TestShouldSwapToken(t *testing.T) {
	if ShouldSwapToken(xai.ErrRateLimited, nil) != true {
		t.Error("expected ShouldSwapToken to return true for rate limited")
	}
	if ShouldSwapToken(xai.ErrNetwork, nil) != false {
		t.Error("expected ShouldSwapToken to return false for network error")
	}
	if ShouldSwapToken(xai.ErrForbidden, nil) != true {
		t.Error("expected ShouldSwapToken to return true for token-level 403")
	}
	if ShouldSwapToken(xai.ErrCFChallenge, nil) != false {
		t.Error("expected ShouldSwapToken to return false for CF challenge")
	}
}

func TestShouldResetSession(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"CF challenge", xai.ErrCFChallenge, true},
		{"token-level 403", xai.ErrForbidden, false},
		{"rate limited", xai.ErrRateLimited, false},
		{"network error", xai.ErrNetwork, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldResetSession(tt.err, nil); got != tt.want {
				t.Errorf("ShouldResetSession(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsCFChallenge(t *testing.T) {
	if !IsCFChallenge(xai.ErrCFChallenge) {
		t.Error("expected IsCFChallenge to return true for ErrCFChallenge")
	}
	if IsCFChallenge(xai.ErrForbidden) {
		t.Error("expected IsCFChallenge to return false for ErrForbidden")
	}
	if IsCFChallenge(nil) {
		t.Error("expected IsCFChallenge to return false for nil")
	}
}
