package httpapi

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/crmmc/grokforge/internal/token"
	"github.com/crmmc/grokforge/internal/xai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAPIError(t *testing.T) {
	err := NewAPIError(400, "invalid_request_error", "invalid_model", "model not found")

	assert.Equal(t, 400, err.Status)
	assert.Equal(t, "model not found", err.Error.Message)
	assert.Equal(t, "invalid_request_error", err.Error.Type)
	assert.Equal(t, "invalid_model", err.Error.Code)
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()

	WriteError(w, 401, "authentication_error", "invalid_api_key", "Invalid API key")

	assert.Equal(t, 401, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	var resp APIError
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	assert.Equal(t, "Invalid API key", resp.Error.Message)
	assert.Equal(t, "authentication_error", resp.Error.Type)
	assert.Equal(t, "invalid_api_key", resp.Error.Code)
}

func TestMapXAIError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectedType   string
		expectedCode   string
	}{
		{
			name:           "forbidden maps to 401 auth error",
			err:            xai.ErrForbidden,
			expectedStatus: 401,
			expectedType:   "authentication_error",
			expectedCode:   "invalid_api_key",
		},
		{
			name:           "rate limited maps to 429",
			err:            xai.ErrRateLimited,
			expectedStatus: 429,
			expectedType:   "rate_limit_error",
			expectedCode:   "rate_limit_exceeded",
		},
		{
			name:           "network error maps to 502",
			err:            xai.ErrNetwork,
			expectedStatus: 502,
			expectedType:   "server_error",
			expectedCode:   "upstream_error",
		},
		{
			name:           "invalid token maps to 401",
			err:            xai.ErrInvalidToken,
			expectedStatus: 401,
			expectedType:   "authentication_error",
			expectedCode:   "invalid_api_key",
		},
		{
			name:           "stream closed maps to 502",
			err:            xai.ErrStreamClosed,
			expectedStatus: 502,
			expectedType:   "server_error",
			expectedCode:   "upstream_error",
		},
		{
			name:           "unknown error maps to 502",
			err:            errors.New("unknown error"),
			expectedStatus: 502,
			expectedType:   "server_error",
			expectedCode:   "upstream_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, apiErr := MapXAIError(tt.err)

			assert.Equal(t, tt.expectedStatus, status)
			assert.Equal(t, tt.expectedType, apiErr.Error.Type)
			assert.Equal(t, tt.expectedCode, apiErr.Error.Code)
		})
	}
}

func TestMapXAIError_PoolExhausted(t *testing.T) {
	status, apiErr := MapXAIError(ErrPoolExhausted)

	assert.Equal(t, 503, status)
	assert.Equal(t, "server_error", apiErr.Error.Type)
	assert.Equal(t, "service_unavailable", apiErr.Error.Code)
}

func TestMapXAIError_NoTokenAvailable(t *testing.T) {
	status, apiErr := MapXAIError(token.ErrNoTokenAvailable)

	assert.Equal(t, 503, status)
	assert.Equal(t, "server_error", apiErr.Error.Type)
	assert.Equal(t, "no_token_available", apiErr.Error.Code)
	assert.Contains(t, apiErr.Error.Message, "No token available")
}
