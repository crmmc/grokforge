package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/crmmc/grokforge/internal/token"
	"github.com/crmmc/grokforge/internal/xai"
)

// ErrPoolExhausted indicates all tokens in the pool are exhausted or cooling down.
var ErrPoolExhausted = errors.New("httpapi: token pool exhausted")

// APIError represents an OpenAI-compatible error response.
type APIError struct {
	Status int         `json:"-"`
	Error  ErrorDetail `json:"error"`
}

// ErrorDetail contains the error details in OpenAI format.
type ErrorDetail struct {
	Message string  `json:"message"`
	Type    string  `json:"type"`
	Param   *string `json:"param"`
	Code    string  `json:"code"`
}

// NewAPIError creates a new API error with the given parameters.
func NewAPIError(status int, errType, code, message string) *APIError {
	return &APIError{
		Status: status,
		Error: ErrorDetail{
			Message: message,
			Type:    errType,
			Code:    code,
		},
	}
}

// WriteError writes a JSON error response with the given status code.
func WriteError(w http.ResponseWriter, status int, errType, code, message string) {
	apiErr := NewAPIError(status, errType, code, message)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(apiErr)
}

// MapXAIError maps xai package errors to HTTP status and APIError.
// Error mapping:
//   - ErrForbidden, ErrInvalidToken -> 401 authentication_error
//   - ErrCFChallenge -> 502 server_error (CF blocking, not auth issue)
//   - ErrRateLimited -> 429 rate_limit_error
//   - ErrNetwork, ErrStreamClosed, other -> 502 server_error
//   - ErrPoolExhausted -> 503 service_unavailable
func MapXAIError(err error) (int, *APIError) {
	switch {
	case errors.Is(err, xai.ErrForbidden), errors.Is(err, xai.ErrInvalidToken):
		return 401, NewAPIError(401, "authentication_error", "invalid_api_key",
			"Invalid authentication credentials")

	case errors.Is(err, xai.ErrCFChallenge):
		return 502, NewAPIError(502, "server_error", "upstream_error",
			"Upstream service temporarily blocked")

	case errors.Is(err, xai.ErrRateLimited):
		return 429, NewAPIError(429, "rate_limit_error", "rate_limit_exceeded",
			"Rate limit exceeded, please retry later")

	case errors.Is(err, ErrPoolExhausted):
		return 503, NewAPIError(503, "server_error", "service_unavailable",
			"Service temporarily unavailable, please retry later")

	case errors.Is(err, token.ErrNoTokenAvailable):
		return 503, NewAPIError(503, "server_error", "no_token_available",
			"No available token for the requested model")

	case errors.Is(err, token.ErrModelNotFound):
		return 404, NewAPIError(404, "not_found", "model_not_found",
			"The requested model is not configured")

	default:
		// ErrNetwork, ErrStreamClosed, and unknown errors
		return 502, NewAPIError(502, "server_error", "upstream_error",
			"Upstream service error")
	}
}
