---
phase: 22-seed-data-registry
plan: 01
subsystem: database
tags: [toml, embed, seed-data, gorm]

requires:
  - phase: 21-model-schema-store
    provides: ModelFamily/ModelMode structs, ModelStore CRUD, DeriveRequestName

provides:
  - config/models.seed.toml seed data file (5 families, 9 modes)
  - config/embed.go SeedFS embed.FS declaration
  - store.SeedModels() function for first-run import
  - main.go startup wiring (AutoMigrate -> SeedModels -> config overrides)

affects: [22-02-registry, 23-three-pool, 24-integration]

tech-stack:
  added: []
  patterns: [seed-data-import, external-file-priority-with-embed-fallback]

key-files:
  created:
    - config/models.seed.toml
    - config/embed.go
    - internal/store/seed.go
    - internal/store/seed_test.go
  modified:
    - cmd/grokforge/main.go

key-decisions:
  - "Seed import bypasses ModelStore CRUD (direct GORM tx) since empty table has no conflicts"
  - "seedconfig alias for config package import to avoid collision with internal/config"

patterns-established:
  - "Seed data pattern: check empty table -> load TOML (external > embed) -> per-family transaction -> DefaultModeID backfill"

requirements-completed: [SEED-01]

duration: 6min
completed: 2026-04-14
---

# Phase 22 Plan 01: Seed Data & Import Summary

**models.seed.toml 种子文件 (5 families/9 modes) + SeedModels 首次启动自动导入 + embed.FS fallback**

## Performance

- **Duration:** 6 min
- **Started:** 2026-04-14T15:52:53Z
- **Completed:** 2026-04-14T15:59:01Z
- **Tasks:** 3
- **Files modified:** 5

## Accomplishments
- 种子数据 TOML 文件包含 5 个 model family (grok-4.20, mini, image, image-edit, video) 共 9 个 mode
- SeedModels 函数：空表导入、非空跳过、外部文件优先、embed.FS fallback、DefaultModeID 回填
- main.go 启动序列正确接入：AutoMigrate -> SeedModels -> DB config overrides

## Task Commits

1. **Task 1: 创建种子数据文件 + embed 声明** - `ea9c253` (feat)
2. **Task 2: 实现 SeedModels 函数 + 单元测试 (TDD)**
   - RED: `fcc1ac3` (test) - 7 个失败测试
   - GREEN: `3e12b66` (feat) - 实现通过
3. **Task 3: main.go 接入种子数据加载** - `c7e77d7` (feat)

## Files Created/Modified
- `config/models.seed.toml` - 种子数据定义 (5 families, 9 modes)
- `config/embed.go` - SeedFS embed.FS 声明
- `internal/store/seed.go` - SeedModels + loadSeedData + importFamily
- `internal/store/seed_test.go` - 7 个测试函数覆盖所有场景
- `cmd/grokforge/main.go` - 启动序列接入 SeedModels

## Decisions Made
- 种子导入绕过 ModelStore CRUD 直接用 GORM 事务（空表无冲突，避免 N+1 校验开销）
- 使用 `seedconfig` 别名导入 `config` 包，避免与 `internal/config` 冲突
- SeedModels 使用 `context.Background()` 而非 `rootCtx`（后者在调用点尚未创建）

## Deviations from Plan

None - plan executed exactly as written.

## Issues Encountered
- `cmd/grokforge/` 目录被 .gitignore 中的 `grokforge` 规则匹配，需要 `git add -f` 强制添加

## Next Phase Readiness
- 种子数据基础设施就绪，Plan 02 (ModelRegistry) 可直接从 DB 加载已导入的模型数据
- SeedModels 导出函数签名稳定，无需后续修改

---
*Phase: 22-seed-data-registry*
*Completed: 2026-04-14*

## Self-Check: PASSED

All 4 created files verified. All 4 commits (ea9c253, fcc1ac3, 3e12b66, c7e77d7) verified.
