---
phase: "23"
plan: "03"
subsystem: config-cleanup
tags: [config, registry, frontend, three-pool]
dependency-graph:
  requires: [23-01, 23-02]
  provides: [registry-wired, config-cleaned]
  affects: [config, flow, httpapi, registry, frontend]
tech-stack:
  added: []
  patterns: [ModelResolver-interface, registry-driven-routing]
key-files:
  created: []
  modified:
    - internal/config/config.go
    - internal/config/defaults.go
    - internal/config/runtime.go
    - internal/config/runtime_test.go
    - config.defaults.toml
    - internal/httpapi/admin_config.go
    - internal/httpapi/admin_config_types.go
    - internal/httpapi/admin_config_update.go
    - internal/flow/chat.go
    - internal/flow/chat_types.go
    - internal/flow/image.go
    - internal/flow/video.go
    - internal/flow/chat_test.go
    - internal/flow/chat_stream_state_test.go
    - internal/httpapi/openai/handler.go
    - internal/httpapi/openai/models.go
    - internal/httpapi/openai/models_test.go
    - internal/httpapi/openai/provider.go
    - internal/httpapi/openai/chat_test.go
    - internal/registry/registry.go
    - internal/store/models.go
    - internal/store/seed.go
    - config/models.seed.toml
    - cmd/grokforge/main.go
    - web/src/app/settings/models-config-form.tsx
    - web/src/app/tokens/token-dialog.tsx
    - web/src/app/tokens/import-dialog.tsx
    - web/src/components/features/token-table.tsx
    - web/src/lib/i18n/en.ts
    - web/src/lib/i18n/zh.ts
    - web/src/lib/validations/config.ts
    - web/src/types/system.ts
decisions:
  - "Added QuotaCost field to ModelMode and seed data for registry-driven cost resolution"
  - "Replaced old openai.ModelRegistry with registry.ModelRegistry for /v1/models endpoint"
  - "Added NewTestRegistry helper to registry package for test isolation without DB"
metrics:
  tasks: 2
  completed: "2026-04-15"
---

# Phase 23 Plan 03: Config Cleanup & Frontend Three-Pool Summary

Delete legacy config fields (BasicModels/SuperModels/PreferredPool), wire ModelResolver into all flow/handler callers, add ssoHeavy pool to frontend.

## Task Summary

| Task | Name | Commit | Key Changes |
|------|------|--------|-------------|
| 1 | Delete old config fields | f39579e | Remove BasicModels/SuperModels/PreferredPool from TokenConfig, defaults, runtime, admin API, config.defaults.toml |
| 2 | Wire ModelResolver + frontend cleanup | 7ab320a | Registry implements ModelResolver, flow layer uses GetPoolForModel(resolver), frontend adds ssoHeavy pool option |

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Missing] Added QuotaCost to ModelMode and seed data**
- **Found during:** Task 2
- **Issue:** CostForModel(model, resolver) needs cost data from registry, but ModelMode had no QuotaCost field
- **Fix:** Added QuotaCost int field to ModelMode, SeedMode, ResolvedModel; populated in seed.go and models.seed.toml for expert/heavy modes (cost=4)
- **Files modified:** internal/store/models.go, internal/store/seed.go, config/models.seed.toml, internal/registry/registry.go

**2. [Rule 2 - Missing] Added NewTestRegistry helper**
- **Found during:** Task 2
- **Issue:** models_test.go needed a registry without DB; old openai.ModelRegistry was removed
- **Fix:** Added registry.NewTestRegistry([]TestFamilyWithModes) for test isolation
- **Files modified:** internal/registry/registry.go, internal/httpapi/openai/models_test.go

**3. [Rule 1 - Bug] Fixed TestHandleChat_InvalidModel test**
- **Found during:** Task 2
- **Issue:** Test created Handler without ModelRegistry, so validateModel skipped model check (returned 501 instead of 404)
- **Fix:** Added ModelRegistry to test handler setup
- **Files modified:** internal/httpapi/openai/chat_test.go

## Verification

All tests pass:
- `go test ./internal/config/` - PASS
- `go test ./internal/token/` - PASS
- `go test ./internal/registry/` - PASS
- `go test ./internal/flow/` - PASS
- `go test ./internal/httpapi/openai/` - PASS

## Self-Check: PASSED
