package upstream

import (
	"context"
	"errors"
)

var (
	ErrInvalidToken = errors.New("upstream: invalid or expired token")
	ErrForbidden    = errors.New("upstream: forbidden")
	ErrCFChallenge  = errors.New("upstream: cloudflare challenge")
)

var (
	ErrRateLimited     = errors.New("upstream: rate limited")
	ErrCreditExhausted = errors.New("upstream: credit exhausted")
)

var (
	ErrNetwork         = errors.New("upstream: network error")
	ErrServerError     = errors.New("upstream: server error")
	ErrStreamCorrupted = errors.New("upstream: stream corrupted")
)

// IsTokenLevel: 换 token，不可同 token 重试。
func IsTokenLevel(err error) bool {
	return errors.Is(err, ErrInvalidToken) || errors.Is(err, ErrForbidden)
}

// IsQuotaLevel: cool 当前 token 后换 token。
func IsQuotaLevel(err error) bool {
	return errors.Is(err, ErrRateLimited) || errors.Is(err, ErrCreditExhausted)
}

// NeedsCFRefresh: 触发 CF refresh 后同 token 重试。
func NeedsCFRefresh(err error) bool {
	return errors.Is(err, ErrCFChallenge)
}

// IsRecoverable: 可 backoff 重试（含 CF challenge）。context 取消不算。
func IsRecoverable(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	return errors.Is(err, ErrNetwork) || errors.Is(err, ErrServerError) ||
		errors.Is(err, ErrStreamCorrupted) || errors.Is(err, ErrCFChallenge)
}
