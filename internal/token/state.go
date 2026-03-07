// Package token provides token lifecycle management with state machine and pool selection.
package token

import "time"

// Status represents the state of a token in the pool.
type Status string

const (
	// StatusActive indicates the token is available for use.
	StatusActive Status = "active"
	// StatusCooling indicates the token is temporarily unavailable (rate limited).
	StatusCooling Status = "cooling"
	// StatusDisabled indicates the token is manually disabled by the user.
	StatusDisabled Status = "disabled"
	// StatusExpired indicates the token was auto-detected as invalid (e.g. 401).
	StatusExpired Status = "expired"
)

// Pool tier constants.
const (
	PoolBasic = "ssoBasic"
	PoolSuper = "ssoSuper"
)

// Default cooling configuration.
const (
	DefaultCoolDuration   = 5 * time.Minute
	DefaultCoolCycleLimit = 3
)
