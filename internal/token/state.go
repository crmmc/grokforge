// Package token provides token lifecycle management with state machine and pool selection.
package token

import (
	"errors"
	"strings"
)

// Status represents the state of a token in the pool.
type Status string

const (
	// StatusActive indicates the token is available for use.
	StatusActive Status = "active"
	// StatusDisabled indicates the token is manually disabled by the user.
	StatusDisabled Status = "disabled"
	// StatusExpired indicates the token was auto-detected as invalid (e.g. 401).
	StatusExpired Status = "expired"
)

// Pool tier constants.
const (
	PoolBasic = "ssoBasic"
	PoolSuper = "ssoSuper"
	PoolHeavy = "ssoHeavy"
)

var ErrInvalidPool = errors.New("invalid token pool")

// PoolLevel represents the numeric tier of a pool for comparison.
// Higher level pools can serve models with lower pool_floor requirements.
type PoolLevel int

const (
	PoolLevelBasic PoolLevel = 1
	PoolLevelSuper PoolLevel = 2
	PoolLevelHeavy PoolLevel = 3
)

// PoolLevelFor converts a pool name or floor name to its numeric level.
// Accepts full pool names ("ssoBasic") and short floor names ("basic").
// Returns 0 for unknown inputs.
func PoolLevelFor(pool string) PoolLevel {
	switch pool {
	case PoolBasic, "basic":
		return PoolLevelBasic
	case PoolSuper, "super":
		return PoolLevelSuper
	case PoolHeavy, "heavy":
		return PoolLevelHeavy
	default:
		return 0
	}
}

// PoolNameForLevel returns the pool name for a given level.
// Returns empty string for unknown levels.
func PoolNameForLevel(level PoolLevel) string {
	switch level {
	case PoolLevelBasic:
		return PoolBasic
	case PoolLevelSuper:
		return PoolSuper
	case PoolLevelHeavy:
		return PoolHeavy
	default:
		return ""
	}
}

// AllPoolNames returns all pool names in ascending level order.
func AllPoolNames() []string {
	return []string{PoolBasic, PoolSuper, PoolHeavy}
}

// NormalizePoolName converts accepted pool aliases to canonical pool names.
func NormalizePoolName(pool string) (string, error) {
	switch strings.TrimSpace(pool) {
	case PoolBasic, "basic":
		return PoolBasic, nil
	case PoolSuper, "super":
		return PoolSuper, nil
	case PoolHeavy, "heavy":
		return PoolHeavy, nil
	default:
		return "", ErrInvalidPool
	}
}

// PoolToShort converts canonical pool name to short form for catalog lookup.
func PoolToShort(pool string) string {
	switch pool {
	case PoolBasic:
		return "basic"
	case PoolSuper:
		return "super"
	case PoolHeavy:
		return "heavy"
	default:
		return pool
	}
}
