---
phase: 23-three-pool-routing
plan: 02
subsystem: token
tags: [pool-routing, picker, category, model-resolver]

requires:
  - phase: 23-01
    provides: PoolHeavy constant, PoolLevel tier system, PoolLevelFor/AllPoolNames
provides:
  - ModelResolver interface (decouples picker from registry)
  - GetPoolForModel returns []string based on pool_floor >= matching
  - PickForModel tries pools in ascending order
  - CostForModel reads cost from resolver
affects: [23-03 caller adaptation]

tech-stack:
  added: []
  patterns: [interface-based dependency injection for ModelResolver]

key-files:
  created: []
  modified: [internal/token/picker.go, internal/token/picker_test.go, internal/token/category.go, internal/token/category_test.go]

key-decisions:
  - "ModelResolver interface defined in picker.go to avoid circular import with registry"
  - "GetPoolForModel returns []string (ascending) replacing old (primary, fallback string) pattern"
  - "PickForModel tries pools sequentially, returns first available token"
  - "CostForModel defaults to 1 when resolver is nil or model not found"
  - "ParseModelEntry retained — still used by other callers"

patterns-established:
  - "ModelResolver interface for pool floor resolution — all callers use this instead of config"

requirements-completed: [POOL-02]

duration: 4min
completed: 2026-04-15
---

# Phase 23 Plan 02: Routing Core Rewrite Summary

**GetPoolForModel/PickForModel/CostForModel rewritten for three-pool routing with ModelResolver interface**

## Performance

- **Duration:** 4 min
- **Tasks:** 2
- **Files modified:** 4

## Accomplishments
- ModelResolver interface defined in picker.go (ResolvePoolFloor method)
- GetPoolForModel: pool_floor >= matching returns ascending pool list
- PickForModel: sequential pool attempt with proper error propagation
- CostForModel: reads cost from resolver, nil-safe with default=1
- Old dual-pool fallback logic (GetPoolsForModel, modelInList) removed

## Task Commits

1. **Task 1: GetPoolForModel + PickForModel rewrite (TDD)** - `1ea92bd` (test: RED) + `dfc2b3e` (feat: GREEN)
2. **Task 2: CostForModel rewrite (TDD)** - `e41780b` (test: RED) + `b225b65` (feat: GREEN)

## Files Modified
- `internal/token/picker.go` - ModelResolver interface, GetPoolForModel ([]string), PickForModel (sequential try)
- `internal/token/picker_test.go` - Full test coverage with mockResolver for all pool_floor levels
- `internal/token/category.go` - CostForModel uses ModelResolver instead of config
- `internal/token/category_test.go` - Tests with mockResolver for cost lookup

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
None

## Next Phase Readiness
- ModelResolver interface ready for registry to implement (Plan 03)
- All callers in flow/httpapi layers need to switch to new signatures (Plan 03)

---
*Phase: 23-three-pool-routing*
*Completed: 2026-04-15*

## Self-Check: PASSED
