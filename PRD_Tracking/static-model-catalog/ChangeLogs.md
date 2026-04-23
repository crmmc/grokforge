# Static Model Catalog Redesign — 变更记录

## 日期: 2026-04-20

## 功能总结

将 GrokForge 的模型管理从 DB 动态 CRUD 重构为静态 TOML catalog，消除运行时数据库依赖，简化部署和维护。

## 变更清单

### 新增
- `internal/modelconfig/` 包 — spec.go, loader.go, embed.go, models.toml
- `internal/modelconfig/loader_test.go` — 22 个验证测试
- `GET /admin/models` 只读 API
- Settings 页面 "Catalog" tab — 只读模型目录表格
- `web/src/lib/hooks/use-admin-models.ts` — 只读 hook
- `config.defaults.toml` — `models_file` 配置项

### 修改
- `internal/registry/registry.go` — 从 ModelSpec 构建，移除 store 依赖
- `internal/registry/registry_test.go` — 适配新构造方式
- `cmd/grokforge/main.go` — 静态 catalog 加载替代 DB seed
- `internal/httpapi/server.go` — 移除 modelStore，只读路由
- `internal/httpapi/admin_model.go` — 重写为只读 handler
- `internal/httpapi/openai/chat_routing.go` — `"image"` → `"image_lite"`
- `internal/httpapi/openai/models.go` — `rm.ID` + `rm.PublicType`
- `internal/httpapi/openai/chat_test.go` — 适配新 registry
- `internal/httpapi/openai/models_test.go` — 适配新 registry
- `internal/config/config.go` — 添加 ModelsFile 字段
- `internal/store/models.go` — 移除 ModelFamily/ModelMode
- `web/src/components/layout/sidebar.tsx` — 移除 models 导航
- README.md / README.zh.md — 更新文档

### 删除
- `internal/store/model_store.go` (~418行)
- `internal/store/model_tx.go` (~54行)
- `internal/store/model_helpers.go` (~149行)
- `internal/store/model_constraints.go` (~136行)
- `internal/store/seed.go` (~152行)
- `internal/store/model_store_test.go` (~1001行)
- `internal/store/seed_test.go` (~231行)
- `internal/httpapi/admin_model_test.go` (~599行)
- `internal/httpapi/model_integration_test.go` (~504行)
- `config/models.seed.toml`
- `config/embed.go`
- `web/src/app/(admin)/models/` (4文件)
- `web/src/lib/hooks/use-model-families.ts`

## 净效果
- 删除 ~3244 行 Go 代码 + ~500 行前端代码
- 新增 ~500 行 Go 代码 (modelconfig + registry + handler)
- 净减少 ~3200 行
