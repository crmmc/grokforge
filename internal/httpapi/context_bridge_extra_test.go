package httpapi

import (
	"context"
	"testing"

	"github.com/crmmc/grokforge/internal/flow"
	"github.com/stretchr/testify/assert"
)

func TestBridgeFlowContext_Extra(t *testing.T) {
	t.Run("no api key id returns context unchanged", func(t *testing.T) {
		ctx := context.Background()

		got := BridgeFlowContext(ctx)

		assert.Equal(t, ctx, got)
		assert.Equal(t, uint(0), flow.FlowAPIKeyIDFromContext(got))
	})

	t.Run("api key id is bridged", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), apiKeyIDKey, uint(42))

		got := BridgeFlowContext(ctx)

		assert.Equal(t, uint(42), flow.FlowAPIKeyIDFromContext(got))
	})
}
