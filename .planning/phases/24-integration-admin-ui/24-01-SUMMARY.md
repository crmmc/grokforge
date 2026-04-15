---
phase: 24-integration-admin-ui
plan: "01"
subsystem: api
tags: [model-registry, upstream-routing, hardcode-removal, openai-compat]

requires:
  - phase: 23-three-pool-routing
    provides: ModelRegistry with Resolve/AllEnabled, NewTestRegistry helper

provides:
  - UpstreamModel/UpstreamMode fields on xai.ChatRequest and flow.ChatRequest
  - Registry-driven model type routing (replaces hardcoded maps)
  - /v1/models type field in API response
  - Frontend type-based model filtering
  - ResolveUpstream callback on ChatFlowConfig

affects: [24-integration-admin-ui]

tech-stack:
  added: []
  patterns:
    - "Registry-driven routing: resolveModelType() replaces hardcoded model type maps"
    - "Upstream passthrough: UpstreamModel/UpstreamMode flow from registry through flow to xai layer"

key-files:
  created: []
  modified:
    - internal/xai/client.go
    - internal/xai/chat.go
    - internal/xai/stream_test.go
    - internal/flow/chat_types.go
    - internal/flow/chat_request.go
    - internal/httpapi/openai/chat_routing.go
    - internal/httpapi/openai/provider.go
    - internal/httpapi/openai/models.go
    - internal/httpapi/openai/chat_test.go
    - cmd/grokforge/main.go
    - web/src/lib/function-api.ts
    - web/src/lib/hooks/use-models.ts

key-decisions:
  - "Dual resolution path: HTTP layer resolves upstream in toFlowRequest, flow layer has ResolveUpstream callback as fallback"
  - "resolveModelType returns 'chat' for unknown models (safe default, not empty string)"

patterns-established:
  - "Registry-driven routing: all model type checks go through Handler.resolveModelType()"
  - "Upstream passthrough: UpstreamModel/UpstreamMode set once, passed through layers without re-resolution"

requirements-completed: []

duration: 12min
completed: 2026-04-15
---

# Phase 24 Plan 01: Hardcode Removal Summary

**Deleted hardcoded modelMappings/routing maps, wired ModelRegistry for upstream resolution and type-based routing across xai/flow/httpapi/frontend**

## Performance

- **Duration:** ~12 min
- **Started:** 2026-04-15T13:57:00Z
- **Completed:** 2026-04-15T14:09:04Z
- **Tasks:** 2
- **Files modified:** 12

## Accomplishments
- Removed all hardcoded model mappings (modelMappings map, mapModel(), modelModeForModel()) from xai layer
- Replaced hardcoded imageModels/videoModels/imageEditModels routing maps with registry-based resolveModelType()
- /v1/models API now returns type field from ModelFamily.Type
- Frontend filters models by type field instead of hardcoded ID sets

## Task Commits

Each task was committed atomically:

1. **Task 1: xai layer — UpstreamModel/UpstreamMode + delete hardcoded maps** (TDD)
   - `6153725` test(24-01): add failing tests for UpstreamModel/UpstreamMode
   - `7311893` feat(24-01): remove hardcoded modelMappings, use UpstreamModel/UpstreamMode
2. **Task 2: flow + routing + models + frontend — Registry full integration**
   - `4046a37` feat(24-01): wire registry into flow/routing/models/frontend
   - `e37bd9c` feat(24-01): add ResolveUpstream callback + rename resolveModelType

## Files Created/Modified
- `internal/xai/client.go` — Added UpstreamModel/UpstreamMode fields to ChatRequest
- `internal/xai/chat.go` — Deleted modelMappings/mapModel/modelModeForModel, buildChatBody uses upstream fields
- `internal/xai/stream_test.go` — Replaced TestMapModel/TestModelModeForModel with TestBuildChatBodyUpstream
- `internal/flow/chat_types.go` — Added UpstreamModel/UpstreamMode to ChatRequest, ResolveUpstream to ChatFlowConfig
- `internal/flow/chat_request.go` — buildXAIRequest passes upstream fields + ResolveUpstream fallback
- `internal/httpapi/openai/chat_routing.go` — Replaced hardcoded maps with Handler.resolveModelType()
- `internal/httpapi/openai/provider.go` — toFlowRequest resolves upstream from registry, handleMediaRoutes uses methods
- `internal/httpapi/openai/models.go` — ModelEntry.Type field, populated from Family.Type
- `internal/httpapi/openai/chat_test.go` — Added testMediaRegistry helper, updated routing tests
- `cmd/grokforge/main.go` — Wired ResolveUpstream callback
- `web/src/lib/function-api.ts` — Added type field to ModelEntry interface
- `web/src/lib/hooks/use-models.ts` — Filter by type field instead of hardcoded ID sets

## Decisions Made
- Dual resolution path: HTTP layer resolves upstream in toFlowRequest (primary), flow layer ResolveUpstream callback (fallback) — ensures upstream is always set regardless of entry point
- resolveModelType returns "chat" for unknown models — safe default prevents routing errors for unregistered models

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Fixed test compilation after toFlowRequest became method**
- **Found during:** Task 2
- **Issue:** chat_test.go called toFlowRequest as standalone function, broke after converting to Handler method
- **Fix:** Updated test to create Handler{} and call h.toFlowRequest()
- **Files modified:** internal/httpapi/openai/chat_test.go
- **Committed in:** 4046a37

**2. [Rule 1 - Bug] Fixed routing tests missing ModelRegistry**
- **Found during:** Task 2
- **Issue:** TestHandleChat_ImageModelRoute/VideoModelRoute created Handler{} without ModelRegistry, causing isImageModel/isVideoModel to return false
- **Fix:** Added testMediaRegistry() helper using registry.NewTestRegistry, updated test Handler initialization
- **Files modified:** internal/httpapi/openai/chat_test.go
- **Committed in:** 4046a37

---

**Total deviations:** 2 auto-fixed (2 bugs)
**Impact on plan:** Both fixes necessary for test correctness after planned refactoring. No scope creep.

## Issues Encountered
- Pre-existing untracked `internal/flow/chat_request_test.go` and `internal/tokencount/` cause test failures when running `go test ./...` — these are out of scope and pre-existing

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- All hardcoded model mappings removed, registry is the single source of truth
- Admin UI model management page (24-02) can now add/modify models with immediate effect on routing
- Frontend automatically adapts to new model types via /v1/models type field

---
*Phase: 24-integration-admin-ui*
*Completed: 2026-04-15*

## Self-Check: PASSED

All 11 modified files exist. All 4 commit hashes verified.
