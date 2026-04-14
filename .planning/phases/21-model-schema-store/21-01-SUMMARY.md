---
phase: 21-model-schema-store
plan: 01
status: done
started: 2026-04-14
completed: 2026-04-14
---

# Plan 01 Summary: Model Schema & Store

## 完成内容

3 个 task 全部完成，3 次原子提交。

### Task 1: ModelFamily + ModelMode struct 定义
- `internal/store/models.go` 新增 `ModelFamily`（11 字段）和 `ModelMode`（10 字段）两个 GORM model
- `ModelMode` 使用 `idx_model_mode` 复合唯一索引 (model_id, mode)
- `AllModels()` 注册两个新 model，AutoMigrate 自动建表
- `internal/store/token_store.go` 新增 `ErrConflict` 哨兵错误

### Task 2: ModelStore CRUD + 冲突校验
- `internal/store/model_store.go`（292 行）：
  - `NewModelStore` 构造函数
  - `DeriveRequestName` 导出函数（默认 mode → family.model，非默认 → family.model-mode）
  - Family CRUD: Get, List, ListEnabled, Create, Update, Delete（级联删除 modes）
  - Mode CRUD: Get, ListByFamily, Create, Update, Delete（清除 default_mode_id 引用）
  - `checkConflict` 事务内全表查询冲突检测（D-07, D-08）
  - 枚举校验：type ∈ {chat,image,image_edit,video}，pool_floor ∈ {basic,super,heavy}
  - DefaultModeID 归属校验（D-04）

### Task 3: 单元测试
- `internal/store/model_store_test.go`（18 个测试函数）全部 PASS
- 覆盖：CRUD、唯一约束、跨 family mode 允许、冲突检测（含 D-08 双向场景）、无误报、级联删除、default_mode_id 清理、枚举校验、归属校验、enabled 过滤、DeriveRequestName

## 修复

- `checkConflict` 中排除 family 时需同时排除其下所有 mode 的派生名，避免 UpdateFamily 自身触发误报

## 验证

- `go build ./internal/store/...` — 编译通过
- `go test -v ./internal/store/...` — 全部 PASS（含现有测试）

## 产出文件

| 文件 | 操作 | 行数 |
|------|------|------|
| `internal/store/models.go` | 修改 | +30 |
| `internal/store/token_store.go` | 修改 | +3 |
| `internal/store/model_store.go` | 新建 | 292 |
| `internal/store/model_store_test.go` | 新建 | 541 |
