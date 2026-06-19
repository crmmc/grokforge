package flow

import (
	"context"
	"errors"

	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/crmmc/grokforge/internal/xai"
)

const (
	httpStatusServerErrorMin = 500
	httpStatusServerErrorMax = 599
)

func isTransportError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if errors.Is(err, xai.ErrNetwork) || errors.Is(err, upstream.ErrNetwork) ||
		errors.Is(err, upstream.ErrServerError) || errors.Is(err, upstream.ErrStreamCorrupted) {
		return true
	}
	statusCode, ok := extractStatusCode(err)
	if !ok {
		return false
	}
	return statusCode >= httpStatusServerErrorMin && statusCode <= httpStatusServerErrorMax
}

// isServerError returns true if the error is a 5xx server error.
func isServerError(err error) bool {
	if err == nil {
		return false
	}
	statusCode, ok := extractStatusCode(err)
	if !ok {
		return false
	}
	return statusCode >= httpStatusServerErrorMin && statusCode <= httpStatusServerErrorMax
}

func shouldSkipTokenPenalty(err error) bool {
	return errors.Is(err, xai.ErrForbidden) || errors.Is(err, xai.ErrCFChallenge) ||
		errors.Is(err, upstream.ErrForbidden) || errors.Is(err, upstream.ErrCFChallenge)
}

func reportTrackedTokenError(tokenSvc TokenServicer, tokenID uint, mode string, err error) {
	if tokenSvc == nil || err == nil {
		return
	}
	reason := truncateReason(err.Error())
	switch {
	case errors.Is(err, xai.ErrInvalidToken), errors.Is(err, upstream.ErrInvalidToken):
		tokenSvc.MarkExpired(tokenID, reason)
	case shouldSkipTokenPenalty(err):
		tokenSvc.ReleaseToken(tokenID)
		return
	case ShouldCoolToken(err, nil):
		tokenSvc.ReportRateLimit(tokenID, mode, reason)
	default:
		recoverable := isTransportError(err) || isServerError(err)
		tokenSvc.ReportError(tokenID, mode, recoverable, reason)
	}
}
