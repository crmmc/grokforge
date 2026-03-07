package xai

import "errors"

// Sentinel errors for common API error conditions.
var (
	// ErrRateLimited indicates a 429 Too Many Requests response.
	ErrRateLimited = errors.New("xai: rate limited (429)")

	// ErrForbidden indicates a 403 Forbidden response due to token-level rejection
	// (account banned, permissions revoked). The token itself is bad.
	ErrForbidden = errors.New("xai: forbidden (403)")

	// ErrCFChallenge indicates a 403 response from Cloudflare challenge.
	// The token is fine; session reset + CF refresh should fix it.
	ErrCFChallenge = errors.New("xai: cloudflare challenge (403)")

	// ErrNetwork indicates a network-level error (connection, timeout, DNS).
	ErrNetwork = errors.New("xai: network error")

	// ErrInvalidToken indicates the provided token is invalid or expired.
	ErrInvalidToken = errors.New("xai: invalid token")

	// ErrStreamClosed indicates the stream was closed unexpectedly.
	ErrStreamClosed = errors.New("xai: stream closed unexpectedly")
)
