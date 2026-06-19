package upstream

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrorClassification(t *testing.T) {
	cases := []struct {
		err                           error
		token, quota, cf, recoverable bool
	}{
		{ErrInvalidToken, true, false, false, false},
		{ErrForbidden, true, false, false, false},
		{ErrRateLimited, false, true, false, false},
		{ErrCreditExhausted, false, true, false, false},
		{ErrCFChallenge, false, false, true, true},
		{ErrNetwork, false, false, false, true},
		{ErrServerError, false, false, false, true},
		{ErrStreamCorrupted, false, false, false, true},
		{fmt.Errorf("wrap: %w", ErrRateLimited), false, true, false, false},
	}
	for _, c := range cases {
		if IsTokenLevel(c.err) != c.token {
			t.Errorf("IsTokenLevel(%v)=%v want %v", c.err, IsTokenLevel(c.err), c.token)
		}
		if IsQuotaLevel(c.err) != c.quota {
			t.Errorf("IsQuotaLevel(%v)=%v want %v", c.err, IsQuotaLevel(c.err), c.quota)
		}
		if NeedsCFRefresh(c.err) != c.cf {
			t.Errorf("NeedsCFRefresh(%v)=%v want %v", c.err, NeedsCFRefresh(c.err), c.cf)
		}
		if IsRecoverable(c.err) != c.recoverable {
			t.Errorf("IsRecoverable(%v)=%v want %v", c.err, IsRecoverable(c.err), c.recoverable)
		}
	}
	if !errors.Is(fmt.Errorf("wrap: %w", ErrRateLimited), ErrRateLimited) {
		t.Fatal("wrapped sentinel should match errors.Is")
	}
}
