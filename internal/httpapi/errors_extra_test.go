package httpapi

import (
	"errors"
	"fmt"
	"testing"

	"github.com/crmmc/grokforge/internal/upstream"
	"github.com/crmmc/grokforge/internal/xai"
	"github.com/stretchr/testify/assert"
)

func TestMapGatewayError_Extra(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedCode   string
	}{
		{name: "xai CF challenge maps to 502", err: xai.ErrCFChallenge, expectedStatus: 502, expectedCode: "upstream_error"},
		{name: "upstream CF challenge maps to 502", err: upstream.ErrCFChallenge, expectedStatus: 502, expectedCode: "upstream_error"},
		{name: "xai console credit exhausted maps to 429", err: xai.ErrConsoleCreditExhausted, expectedStatus: 429, expectedCode: "rate_limit_exceeded"},
		{name: "upstream rate limited maps to 429", err: upstream.ErrRateLimited, expectedStatus: 429, expectedCode: "rate_limit_exceeded"},
		{name: "upstream server error maps to 502", err: upstream.ErrServerError, expectedStatus: 502, expectedCode: "upstream_error"},
		{name: "wrapped forbidden keeps mapping", err: fmt.Errorf("wrapped: %w", upstream.ErrForbidden), expectedStatus: 401, expectedCode: "invalid_api_key"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, apiErr := MapGatewayError(tc.err)
			assert.Equal(t, tc.expectedStatus, status)
			assert.Equal(t, tc.expectedCode, apiErr.Error.Code)
			assert.NotEmpty(t, apiErr.Error.Message)
		})
	}
}

func TestMapGatewayError_UnknownErrorDefaultsTo502(t *testing.T) {
	status, apiErr := MapGatewayError(errors.New("mystery"))
	assert.Equal(t, 502, status)
	assert.Equal(t, "upstream_error", apiErr.Error.Code)
}
