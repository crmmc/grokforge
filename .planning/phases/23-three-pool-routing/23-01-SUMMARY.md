---
phase: 23-three-pool-routing
plan: 01
subsystem: token
tags: [pool-routing, config, cooling]

requires:
  - phase: 22-model-registry
    provides: ModelRegistry with pool_floor field
provides:
  - PoolHeavy constant ("ssoHeavy")
  - PoolLevel numeric tier system (basic=1, super=2, heavy=3)
  - PoolLevelFor() and PoolNameForLevel() conversion functions
  - AllPoolNames() ascending pool list
  - HeavyCoolDurationMin config field (default 60 min)
  - ReportRateLimit PoolHeavy branch
affects: [23-02 routing overhaul, 23-03 config cleanup]

tech-stack:
  added: []
  patterns: [numeric pool level comparison for >= matching]

key-files:
  created: [internal/token/state_test.go]
  modified: [internal/token/state.go, internal/config/config.go, internal/config/defaults.go, internal/token/service.go]

key-decisions:
  - "PoolLevel uses explicit int constants (1/2/3), not iota, for clarity and stable values"
  - "PoolLevelFor accepts both pool names (ssoBasic) and floor names (basic) for flexibility"
  - "HeavyCoolDurationMin defaults to 60 min (shorter than basic=240, super=120)"

patterns-established:
  - "PoolLevel numeric comparison: PoolLevelFor(pool) >= PoolLevelFor(floor) for tier matching"

requirements-completed: [POOL-01]

duration: 4min
completed: 2026-04-15
---

# Phase 23 Plan 01: Pool Infrastructure Summary

**PoolHeavy constant + PoolLevel numeric tier system + heavy cooling config for three-pool routing foundation**

## Performance

- **Duration:** 4 min
- **Started:** 2026-04-15T07:50:33Z
- **Completed:** 2026-04-15T07:54:57Z
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- PoolHeavy = "ssoHeavy" constant alongside PoolBasic/PoolSuper
- PoolLevel numeric type with conversion functions (pool name <-> level)
- HeavyCoolDurationMin config with DB override support
- ReportRateLimit correctly routes PoolHeavy to heavy cooling duration

## Task Commits

Each task was committed atomically:

1. **Task 1: PoolHeavy + PoolLevel (TDD)** - `f8f77e4` (test: RED) + `5a7270d` (feat: GREEN)
2. **Task 2: heavy cooling config + ReportRateLimit** - `7400565` (feat)

## Files Created/Modified
- `internal/token/state.go` - PoolHeavy constant, PoolLevel type, conversion functions
- `internal/token/state_test.go` - Full test coverage for pool level system
- `internal/config/config.go` - HeavyCoolDurationMin field + DB override case
- `internal/config/defaults.go` - Default heavy cooling: 60 min
- `internal/token/service.go` - ReportRateLimit PoolHeavy branch

## Decisions Made
- PoolLevel uses explicit int constants (1/2/3) instead of iota for stable, readable values
- PoolLevelFor accepts both "ssoBasic" and "basic" forms for ergonomic use with store.ModelFamily.PoolFloor
- Heavy cooling default 60 min — shorter than basic (240) and super (120), reflecting heavy pool's premium nature

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- PoolHeavy and PoolLevel infrastructure ready for Plan 02 routing overhaul
- GetPoolForModel can now use PoolLevelFor() for >= tier matching

---
*Phase: 23-three-pool-routing*
*Completed: 2026-04-15*

## Self-Check: PASSED
