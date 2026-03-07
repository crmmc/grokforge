package httpapi

import (
	"context"

	"github.com/crmmc/grokforge/internal/flow"
)

// BridgeFlowContext carries httpapi API key context to the flow layer.
func BridgeFlowContext(ctx context.Context) context.Context {
	if id, ok := APIKeyIDFromContext(ctx); ok {
		return context.WithValue(ctx, flow.FlowAPIKeyIDKey, id)
	}
	return ctx
}
