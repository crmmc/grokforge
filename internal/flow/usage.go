package flow

import (
	"context"

	"github.com/crmmc/grokforge/internal/store"
)

// UsageRecorder defines the interface for recording API usage.
type UsageRecorder interface {
	Record(ctx context.Context, log *store.UsageLog) error
}
