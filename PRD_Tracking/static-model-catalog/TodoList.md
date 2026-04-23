# Static Model Catalog — TodoList

## 依赖关系

```
T1 → T2(依赖T1类型) → T3(依赖T1+T2)
T4 独立
T1 → T5(依赖T1类型)
T5+T6+T7 串行（registry→main→openai 适配链）
T8 依赖 T5+T7
T9 依赖 T8
T10 依赖 T8
T11 依赖 T9
T12 依赖 T6+T10
T13 依赖 T5
T14 依赖 T7
T15 合并到 T3
T16 依赖全部
```

## 执行顺序

1. [x][P] **T1: 创建 modelconfig 包 — spec.go + embed.go + models.toml**
   - 创建 `internal/modelconfig/spec.go` — ModelSpec 结构体 + 枚举常量 + PublicType 派生
   - 创建 `internal/modelconfig/embed.go` — `go:embed models.toml`
   - 创建 `internal/modelconfig/models.toml` — 默认 catalog (从 config/models.seed.toml 转换为设计文档 schema)
   - 验证: 编译通过

2. [x][P] **T4: config.go 添加 ModelsFile 字段**
   - `internal/config/config.go` — App struct 添加 `ModelsFile string \`toml:"models_file"\``
   - `config/config.defaults.toml` — 添加注释 `# models_file = "models.toml"`
   - 验证: 编译通过

3. [x][S] **T2: 创建 modelconfig/loader.go — 加载 + 验证**
   - `Load(embeddedFS embed.FS, externalPath string) ([]ModelSpec, error)` — 加载 + 解析 + 验证
   - 完整验证规则: version=1, 至少1个model, id唯一, type/pool_floor/quota_mode枚举, upstream规则, quota/pool组合, force_thinking仅chat, enable_pro仅image_ws, 至少1个enabled, 未知字段拒绝
   - 路径解析: externalPath 相对于 config.toml 目录
   - 验证: 编译通过

4. [x][S] **T3: 添加 modelconfig 单元测试 (含启动测试)**
   - `internal/modelconfig/loader_test.go`
   - 测试用例: 嵌入加载成功, 外部加载成功, 无效TOML, 重复ID, 无效枚举, 未知字段, 无效upstream, 无效quota/pool组合, 零enabled, PublicType派生, 缺失外部文件失败, 无效catalog失败
   - 验证: `go test -v ./internal/modelconfig/`

5. [x][S] **T5: 重写 registry 从 ModelSpec 构建**
   - `internal/registry/registry.go` 重写:
     - 移除 `store` 包依赖
     - `NewModelRegistry(specs []modelconfig.ModelSpec)` 替代旧构造
     - `ResolvedModel` 字段改为直接值（ID, DisplayName, Type, PublicType, Enabled, PoolFloor, QuotaMode, UpstreamModel, UpstreamMode, ForceThinking, EnablePro）
     - 移除 Refresh, BuildSnapshotFromStore, CommitAndApply, modelReader, committer
     - 保留 Resolve, AllEnabled, EnabledByType, ResolvePoolFloor, AllRequestNames, Count
     - 保留/更新 NewTestRegistry 辅助
   - 验证: 编译通过（下游 T6/T7 修复）

6. [x][S] **T6: 切换 cmd/main.go 启动流程**
   - 移除 `store.SeedModels` 调用
   - 移除 `store.NewModelStore` 构建
   - 添加 `modelconfig.Load(modelconfig.EmbeddedFS, cfg.App.ModelsFile)` 调用
   - 用 `[]ModelSpec` 构建 registry: `registry.NewModelRegistry(specs)`
   - 移除 `ServerConfig.ModelStore` 字段
   - 启动日志: catalog source (embedded/external) + version + enabled count
   - 验证: `make build`

7. [x][S] **T7: 切换 openai 层适配新 registry**
   - `openai/provider.go` — `rm.Family.Type` → `rm.Type`, `rm.Mode.ForceThinking` → `rm.ForceThinking` 等
   - `openai/models.go` — `rm.Family.Type` → `rm.PublicType` 用于响应
   - `openai/chat_routing.go` — `"image"` → `"image_lite"` 类型检查 (isImageModel, isMediaModel)
   - `openai/handler.go` — registry 类型适配（如有需要）
   - 验证: `make build`

8. [x][S] **T8: 添加只读 GET /admin/models + 修改 server.go**
   - `internal/httpapi/server.go`:
     - 移除 `modelStore` 字段和 `ServerConfig.ModelStore`
     - 移除 `/admin/models/families/*` 和 `/admin/models/modes/*` CRUD 路由
     - 添加 `GET /admin/models` 只读路由
   - 新 handler 返回 registry 所有模型完整元数据 (id, display_name, type, public_type, pool_floor, quota_mode, upstream_model, upstream_mode, force_thinking, enable_pro, enabled)
   - 验证: `make build`

9. [x][P] **T13: 重写 registry 测试**
   - `internal/registry/registry_test.go` 重写:
     - 从 `[]ModelSpec` 构建
     - Resolve 正确, AllEnabled 只返回 enabled, EnabledByType 正确
     - ResolvePoolFloor 正确, PublicType 派生正确
     - 并发安全 (-race)
   - 验证: `go test -v -race ./internal/registry/`

10. [x][P] **T14: 重写 openai 测试**
    - `openai/chat_test.go` — 更新 newTestRegistry 构造方式，使用 ModelSpec
    - `openai/models_test.go` — 更新构造方式, 验证 public_type (image_lite → "image")
    - 验证: `go test -v ./internal/httpapi/openai/`

11. [x][P] **T9: 前端 — Settings 只读模型表格**
    - Settings 页面添加只读模型表格组件
    - 新增 `useModels` hook (GET /admin/models)
    - 显示列: ID, Display Name, Type, Pool, Quota Mode, Upstream Model, Upstream Mode, Flags
    - 无编辑/删除/创建控件
    - 验证: `cd web && npm run build`

12. [x][S] **T10: 删除 admin model CRUD API + 测试**
    - 删除 `internal/httpapi/admin_model.go`
    - 删除 `internal/httpapi/admin_model_test.go`
    - 删除 `internal/httpapi/model_integration_test.go`
    - 验证: `make build`

13. [x][S] **T11: 删除前端 models 页面和 CRUD hooks**
    - 删除 `web/src/app/(admin)/models/` 目录
    - 删除 `web/src/lib/hooks/use-model-families.ts`
    - 移除 sidebar 中 models 导航项
    - 清理所有引用
    - 验证: `cd web && npm run build`

14. [x][S] **T12: 删除 DB model store + seed + constraints**
    - 删除文件: model_store.go, model_tx.go, model_helpers.go, model_constraints.go, seed.go, model_store_test.go, seed_test.go
    - 删除 config/models.seed.toml, config/embed.go (SeedFS)
    - `internal/store/models.go` — 移除 ModelFamily, ModelMode 结构体
    - `AllModels()` — 移除 `&ModelFamily{}`, `&ModelMode{}`
    - 清理所有残留引用
    - 验证: `make build && make test`

15. [x][S] **T16: 更新文档**
    - README.md — 静态 catalog 说明, models_file 配置
    - README.zh.md — 中文版同步
    - config.defaults.toml 注释更新
    - 验证: 人工审查
