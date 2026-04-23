# Static Model Catalog Redesign — 项目调研分析

## 📂 项目结构与布局

### 涉及的核心模块

```
cmd/grokforge/main.go              — 启动入口，model seeding + registry 构建
internal/
  store/
    models.go                       — ModelFamily, ModelMode 结构体 (~174行)
    model_store.go                  — ModelStore CRUD (~418行)
    model_tx.go                     — 事务封装 (~54行)
    model_helpers.go                — 辅助函数 + upstream 验证 (~149行)
    model_constraints.go            — DB 约束 + 触发器 (~136行)
    seed.go                         — 种子数据导入 (~152行)
  registry/
    registry.go                     — 运行时模型索引 (~294行)
  httpapi/
    server.go                       — 路由注册 (~341行)
    admin_model.go                  — Admin CRUD handlers (~480行)
    openai/
      handler.go                    — Handler struct (~28行)
      chat_routing.go               — 媒体路由 (~350行)
      provider.go                   — 请求构建 (~161行)
      models.go                     — /v1/models (~59行)
  config/
    config.go                       — TOML 配置加载 (~125行)
config/
  models.seed.toml                  — 种子配置 (~77行)
  embed.go                          — go:embed SeedFS (~8行)
  config.defaults.toml              — 默认配置 (~137行)
web/src/
  app/(admin)/models/               — 模型管理页面 (4文件)
  lib/hooks/use-model-families.ts   — Model CRUD hooks (~113行)
```

### 测试文件 (~3633行)

| 文件 | 行数 | 覆盖范围 |
|------|------|----------|
| store/model_store_test.go | ~1001 | Family/Mode CRUD, 约束, 冲突检测 |
| store/seed_test.go | ~231 | TOML 解析, 导入逻辑 |
| httpapi/admin_model_test.go | ~599 | Admin API, 事务回滚 |
| httpapi/model_integration_test.go | ~504 | 端到端集成 (SQLite) |
| registry/registry_test.go | ~592 | Refresh, Resolve, 并发安全 |
| openai/chat_test.go + models_test.go | ~706 | 路由, /v1/models 响应 |

---

## 🎨 代码风格规范

- Go 标准布局，包按职责划分
- 接口定义在消费方（如 `ModelStoreInterface` 在 `httpapi` 包）
- GORM 模型使用 `gorm:"..."` tag，JSON 使用 `json:"..."` tag
- 错误处理：自定义 sentinel errors (`ErrNotFound`, `ErrConflict`, `ErrInvalidInput`)
- 测试：table-driven tests + 辅助函数 (`setupModelTestDB`, `newTestFamily`)

---

## 📦 技术栈分析

| 组件 | 技术 |
|------|------|
| 路由 | chi |
| ORM | GORM (SQLite/PostgreSQL) |
| 配置 | BurntSushi/toml |
| 嵌入 | go:embed |
| 前端 | Next.js 15 + shadcn/ui + React Query |
| 测试 | testing + testify |

---

## 🏗️ 架构设计

### 当前模型系统架构

```
config/models.seed.toml → store.SeedModels() → DB (model_families + model_modes)
                                                    ↓
                                              store.ModelStore (CRUD)
                                                    ↓
                                              registry.ModelRegistry (内存索引)
                                                    ↓
                                    ┌───────────────┼───────────────┐
                                    ↓               ↓               ↓
                              openai/handler    admin_model     token/picker
                              (路由+响应)       (CRUD API)      (pool 选择)
```

### 目标架构

```
internal/modelconfig/models.toml → modelconfig.Load() → []ModelSpec
                                                              ↓
                                                    registry.ModelRegistry (只读索引)
                                                              ↓
                                          ┌───────────────────┼───────────────┐
                                          ↓                   ↓               ↓
                                    openai/handler      GET /admin/models   token/picker
                                    (路由+响应)         (只读 API)          (pool 选择)
```

### 关键变化

1. **数据源**: DB → 静态 TOML 文件
2. **Registry 构建**: `ModelStore` → `[]modelconfig.ModelSpec`
3. **Admin API**: CRUD → 只读
4. **前端**: 管理页面 → Settings 只读表格
5. **ResolvedModel**: 持有 `*store.ModelFamily/*ModelMode` → 持有 `*modelconfig.ModelSpec`

---

## ⚙️ 工程实践

### 依赖注入模式
- `cmd/main.go` 手动构建依赖图
- Flow 层通过 `ModelResolver` 接口解耦 registry
- Admin handler 通过 `ModelStoreInterface` 接口解耦 store

### 事务模式
- `ModelStoreTx` 封装 GORM 事务
- `mutateAndRefreshRegistry` 实现原子写入 + snapshot 交换
- 重构后此模式完全移除

### 嵌入模式
- `config/embed.go`: `go:embed models.seed.toml` → `SeedFS`
- 新增: `internal/modelconfig/embed.go`: `go:embed models.toml`

---

## 🧪 测试策略

### 当前测试层次
1. **单元测试**: store CRUD, registry resolve, seed 解析
2. **集成测试**: 真实 SQLite + 完整 admin API 链路
3. **HTTP 测试**: httptest.Server + mock store

### 重构后测试策略
1. **modelconfig 单元测试**: TOML 解析, 验证规则, 嵌入/外部加载
2. **registry 单元测试**: 从 `[]ModelSpec` 构建, resolve, list
3. **HTTP 测试**: 新 registry 构造方式
4. **删除**: 所有 model CRUD 测试 (~2335行)

---

## 🔧 遗留问题识别

| 问题 | 影响 |
|------|------|
| `upstream` 验证逻辑重复 | `model_helpers.go` 和 `registry.go` 各有一份 |
| `"image"` 类型命名歧义 | 内部 `"image"` 实际是 `image_lite`，需重命名 |
| `ResolvedModel` 持有 store 指针 | 与 DB 强耦合，需改为持有 `ModelSpec` |
| `AllModels()` 包含 model 结构体 | 需从 AutoMigrate 列表中移除 |
| Settings 页面无 model 内容 | 设计要求在 Settings 添加只读表格 |

---

## 🔑 关键发现总结

1. **影响范围**: 涉及 ~20 个 Go 文件 + ~4 个前端文件，删除 ~3633 行测试 + ~1500 行 store 代码
2. **核心风险**: registry `ResolvedModel` 被 flow/openai 层广泛引用，字段访问路径变更影响面大
3. **类型重命名**: `"image"` → `"image_lite"` 需全局搜索替换
4. **Config 扩展**: `config.go` 需新增 `ModelsFile` 字段
5. **前端**: models 页面完全删除，Settings 页面新增只读表格
6. **token.ModelResolver 接口**: `ResolvePoolFloor(requestName)` 保持不变，registry 继续实现
7. **无 DB 迁移**: 不需要迁移旧数据，直接删除 model 相关表
