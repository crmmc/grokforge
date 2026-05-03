package token

import (
	"context"
)

// RefreshRequester refreshes exhausted token mode quotas from upstream.
type RefreshRequester interface {
	RequestRefresh(tokenID uint, mode string)
	RefreshToken(ctx context.Context, id uint) error
	ForgetToken(tokenID uint)
}
