package upstream

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsRecoverable_ContextErrorsNotRecoverable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"context canceled", context.Canceled, false},
		{"context deadline exceeded", context.DeadlineExceeded, false},
		{"wrapped canceled", fmt.Errorf("do: %w", context.Canceled), false},
		{"wrapped deadline", fmt.Errorf("do: %w", context.DeadlineExceeded), false},
		{"nil error", nil, false},
		{"unrelated error", errors.New("boom"), false},
		{"canceled plus network", fmt.Errorf("a: %w; b: %w", context.Canceled, ErrNetwork), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsRecoverable(tt.err))
		})
	}
}

func TestIsRecoverable_RecoverableSentinels(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
	}{
		{"network", ErrNetwork},
		{"server error", ErrServerError},
		{"stream corrupted", ErrStreamCorrupted},
		{"cf challenge", ErrCFChallenge},
		{"wrapped network", fmt.Errorf("call: %w", ErrNetwork)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, IsRecoverable(tt.err))
		})
	}
}
