# Static Model Catalog — 任务拆分草稿

## 设计文档 Implementation Order 映射

| 设计步骤 | 任务 |
|----------|------|
| 1. 嵌入静态 catalog 源 | T1 |
| 2. config 添加 models_file | T2 |
| 3. internal/modelconfig 包 | T3, T4 |
| 4. 重写 registry | T5 |
| 5. 切换 /v1/models 和路由 | T6, T7 |
| 6. 添加只读 GET /admin/models | T8 |
| 7. Settings 只读表格 | T9 |
| 8. 删除 /admin/models/* 写 API | T10 |
| 9. 删除 /models 页面和相关 hooks | T11 |
| 10. 删除 DB model store + seed | T12 |
| 11. 删除旧测试 + 添加新测试 | T13, T14, T15 |
| 12. 更新文档 | T16 |

---

## 任务列表

### T1: 创建 internal/modelconfig 包 — spec.go + embed.go + models.toml

**输入**: 设计文档 Schema 定义
**输出**:
- `internal/modelconfig/spec.go` — ModelSpec 结构体 + 枚举常量 + PublicType 派生
- `internal/modelconfig/embed.go` — `go:embed models.toml`
- `internal/modelconfig/models.toml` — 默认 catalog (从 config/models.seed.toml 转换)

**验证**: 编译通过

---

### T2: 创建 internal/modelconfig/loader.go — 加载 + 验证

**输入**: T1 的类型定义
**输出**:
- `Load(embeddedFS, externalPath string) ([]ModelSpec, error)` — 加载 + 解析 + 验证
- 完整验证规则实现 (设计文档 Validation Rules 全部)
- 未知字段拒绝

**验证**: 编译通过

---

### T3: 添加 modelconfig 单元测试

**输入**: T1, T2
**输出**: `internal/modelconfig/loader_test.go`
- 嵌入 catalog 加载成功
- 外部 catalog 加载成功
- 拒绝无效 TOML
- 拒绝重复 ID
- 拒绝无效枚举
- 拒绝未知字段
- 拒绝无效 upstream 配置
- 拒绝无效 quota/pool 组合
- 拒绝零 enabled 模型
- PublicType 派生正确

**验证**: `go test -v ./internal/modelconfig/`

---

### T4: config.go 添加 ModelsFile 字段

**输入**: 设计文档 config 部分
**输出**:
- `internal/config/config.go` — App struct 添加 `ModelsFile string`
- `config/config.defaults.toml` — 添加注释说明 `# models_file = "models.toml"`

**验证**: 编译通过

---

### T5: 重写 registry 从 ModelSpec 构建

**输入**: T1 的 ModelSpec 类型
**输出**:
- `internal/registry/registry.go` — 重写
  - 移除 `store` 依赖
  - `NewModelRegistry(specs []modelconfig.ModelSpec)` 替代 `NewModelRegistry(modelStore)`
  - `ResolvedModel` 字段改为直接持有 ModelSpec 数据（非指针）
  - 移除 `Refresh`, `BuildSnapshotFromStore`, `CommitAndApply`
  - 保留 `Resolve`, `AllEnabled`, `EnabledByType`, `ResolvePoolFloor`, `AllRequestNames`, `Count`
  - 新增 `NewTestRegistry` 辅助

**验证**: 编译通过（下游暂时会报错，T6/T7 修复）

---

### T6: 切换 cmd/main.go 启动流程

**输入**: T1-T5
**输出**:
- `cmd/grokforge/main.go` —
  - 移除 `store.SeedModels` 调用
  - 移除 `store.NewModelStore` 构建
  - 添加 `modelconfig.Load()` 调用
  - 用 `[]ModelSpec` 构建 registry
  - 移除 `ServerConfig.ModelStore`
  - 启动日志输出 catalog source + version + enabled count

**验证**: `make build` 编译通过

---

### T7: 切换 openai 层适配新 registry

**输入**: T5 的新 ResolvedModel
**输出**:
- `openai/provider.go` — `rm.Family.Type` → `rm.Type`, `rm.ForceThinking` 等路径变更
- `openai/models.go` — `rm.Family.Type` → `rm.PublicType`
- `openai/chat_routing.go` — `"image"` → `"image_lite"` 类型检查
- `openai/handler.go` — registry 类型适配

**验证**: `make build` 编译通过

---

### T8: 添加只读 GET /admin/models + 修改 server.go

**输入**: T5-T7
**输出**:
- `internal/httpapi/server.go` — 移除 `modelStore` 字段, 移除 CRUD 路由, 添加 `GET /admin/models`
- 新 handler 返回 registry 中所有模型的完整元数据

**验证**: `make build` 编译通过

---

### T9: 前端 — Settings 只读模型表格

**输入**: T8 的 GET /admin/models API
**输出**:
- `web/src/app/(admin)/settings/` — 添加只读模型表格组件
- 新增 `useModels` hook (只读 GET)
- 显示列: ID, Display Name, Type, Pool, Quota Mode, Upstream Model, Upstream Mode, Flags

**验证**: `cd web && npm run build`

---

### T10: 删除 admin model CRUD API

**输入**: T8 已替换路由
**输出**:
- 删除 `internal/httpapi/admin_model.go`
- 删除 `internal/httpapi/admin_model_test.go`
- 删除 `internal/httpapi/model_integration_test.go`

**验证**: `make build`

---

### T11: 删除前端 models 页面和 CRUD hooks

**输入**: T9 已替换 UI
**输出**:
- 删除 `web/src/app/(admin)/models/` 目录
- 删除 `web/src/lib/hooks/use-model-families.ts`
- 移除 sidebar 中 models 导航项
- 清理所有引用

**验证**: `cd web && npm run build`

---

### T12: 删除 DB model store + seed + constraints

**输入**: T6 已移除 main.go 中的引用
**输出**:
- 删除 `internal/store/model_store.go`
- 删除 `internal/store/model_tx.go`
- 删除 `internal/store/model_helpers.go`
- 删除 `internal/store/model_constraints.go`
- 删除 `internal/store/seed.go`
- 删除 `internal/store/model_store_test.go`
- 删除 `internal/store/seed_test.go`
- 删除 `config/models.seed.toml`
- 删除 `config/embed.go` (SeedFS)
- `internal/store/models.go` — 移除 ModelFamily, ModelMode 结构体
- `AllModels()` — 移除 `&ModelFamily{}`, `&ModelMode{}`

**验证**: `make build && make test`

---

### T13: 重写 registry 测试

**输入**: T5 的新 registry
**输出**: `internal/registry/registry_test.go` —
- 从 ModelSpec 构建
- Resolve 正确
- AllEnabled 只返回 enabled
- EnabledByType 正确
- ResolvePoolFloor 正确
- PublicType 派生正确
- 并发安全

**验证**: `go test -v ./internal/registry/`

---

### T14: 重写 openai 测试

**输入**: T7 的新 openai 层
**输出**:
- `openai/chat_test.go` — 更新 test registry 构造
- `openai/models_test.go` — 更新 test registry 构造, 验证 public_type

**验证**: `go test -v ./internal/httpapi/openai/`

---

### T15: 添加启动测试

**输入**: T6 的启动流程
**输出**: `internal/modelconfig/startup_test.go` 或集成到 loader_test.go
- 嵌入 catalog 启动成功
- 外部 catalog 替换嵌入
- 缺失外部 catalog 启动失败
- 无效 catalog 启动失败

**验证**: `go test -v ./internal/modelconfig/`

---

### T16: 更新文档

**输入**: 全部完成
**输出**:
- README.md — 静态 catalog 说明
- README.zh.md — 中文版
- config.defaults.toml 注释更新

**验证**: 人工审查
