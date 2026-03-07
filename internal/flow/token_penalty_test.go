package flow

import (
	"context"
	"errors"
	"testing"

	"github.com/crmmc/grokforge/internal/xai"
)

func TestIsTransportError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"network error", xai.ErrNetwork, true},
		{"deadline exceeded", context.DeadlineExceeded, true},
		{"500 status", errors.New("unexpected status 500: boom"), true},
		{"503 status", errors.New("503 Service Unavailable"), true},
		{"rate limited", xai.ErrRateLimited, false},
		{"forbidden", xai.ErrForbidden, false},
		{"cf challenge", xai.ErrCFChallenge, false},
		{"invalid token", xai.ErrInvalidToken, false},
		{"nil error", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransportError(tt.err); got != tt.want {
				t.Errorf("isTransportError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
