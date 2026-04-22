package flow

import (
	"context"
	"errors"

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
	if errors.Is(err, xai.ErrNetwork) {
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
