# Phase 24: Integration & Admin UI - Context

**Gathered:** 2026-04-15
**Status:** Ready for planning

<domain>
## Phase Boundary

主链路和 API 端点全面接入 ModelRegistry，前端提供模型管理界面。包含：xai/chat.go modelMappings 移除、chat_routing.go 类型判断改造、Admin 模型 CRUD API、前端模型管理页面、token 管理页面 heavy 池选项。不含 quota 运行时接入、旧模型名 alias 兼容。

</domain>

<decisions>
## Implementation Decisions

### 主链路接入
- **D-01:** 一步到位删除 xai/chat.go 的 modelMappings 硬编码 map 和 mapModel()/modelModeForModel() 函数，全部改为从 ModelRegistry Resolve
- **D-02:** 删除 chat_routing.go 的 imageModels/videoModels/imageEditModels 硬编码 map，isImageModel/isVideoModel/isImageEditModel 改为通过 Registry Resolve 后读 family.Type 判断
- **D-03:** 种子数据已在 Phase 22 导入 DB，不需要过渡期

### Admin 模型 CRUD API
- **D-04:** family 和 mode 分离端点：/admin/models/families 和 /admin/models/modes，与现有 /admin/tokens、/admin/apikeys 模式一致
- **D-05:** CRUD 操作后同步调用 registry.Refresh() 刷新内存快照，对低频 admin 操作足够

### 前端模型管理页面
- **D-06:** Master-Detail 布局：左侧 family 列表，右侧展示选中 family 的 modes
- **D-07:** family 和 mode 的编辑使用 Dialog 弹窗，与 tokens 页的 token-dialog 模式一致
- **D-08:** token 管理页面的池选项增加 heavy 池

### mapModel 兼容与 fallback
- **D-09:** 严格模式：Registry 查不到的模型名直接返回 400 错误，不做 grok- 前缀 passthrough，不做默认回退
- **D-10:** 所有合法模型都在 Registry 中，未知模型名即为错误请求

### Claude's Discretion
- Admin API 的具体请求/响应 DTO 结构
- 前端模型管理页面的具体组件拆分和样式细节
- 错误提示文案
- 测试用例覆盖范围

</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### 设计文档
- `plans.md` — 完整的模型管理方案设计，包含表结构、派生规则、池等级、种子数据格式

### 主链路改造目标
- `internal/xai/chat.go` §189-234 — modelMappings 硬编码 map + mapModel()/modelModeForModel()，需删除并替换为 Registry 调用
- `internal/httpapi/openai/chat_routing.go` §23-55 — imageModels/videoModels/imageEditModels 硬编码 map + isXxxModel() 函数，需改为 Registry 类型判断
- `internal/httpapi/openai/provider.go` — 调用 isImageModel/isVideoModel 的路由分发点

### Registry（已完成，Phase 22 产出）
- `internal/registry/registry.go` — ModelRegistry：Resolve/EnabledByType/AllRequestNames/Refresh 方法
- `internal/httpapi/openai/models.go` — HandleModelsFromRegistry 已实现 /v1/models 从 Registry 构建

### 现有 Admin API 模式
- `internal/httpapi/admin_token.go` — Admin token CRUD 端点模式参考
- `internal/httpapi/admin_apikey.go` — Admin apikey CRUD 端点模式参考

### Phase 21/23 决策
- `.planning/phases/21-model-schema-store/21-CONTEXT.md` — 表结构、派生规则、store CRUD 模式
- `.planning/phases/23-three-pool-routing/23-CONTEXT.md` — 三池路由、pool_floor 匹配语义

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- `registry.ModelRegistry` — Resolve(requestName) 返回 ResolvedModel（含 Family/Mode/EffectiveFloor/UpstreamModel/UpstreamMode）
- `registry.EnabledByType(typ)` — 按 type 查询已启用模型，可用于替代硬编码类型 map
- `store.ModelStore` — Phase 21 产出的 CRUD 接口
- `HandleModelsFromRegistry()` — /v1/models 已从 Registry 构建，INTG-02 基本完成
- `Handler.ModelRegistry` — openai Handler 已持有 Registry 引用

### Established Patterns
- Admin API 按实体拆文件：admin_token.go、admin_apikey.go
- 前端页面按目录组织：web/src/app/{entity}/
- Dialog 编辑模式：token-dialog.tsx 参考
- React Query 数据获取模式

### Integration Points
- `cmd/grokforge/main.go` — Admin 路由注册，需新增模型 CRUD 路由
- `internal/httpapi/openai/handler.go` — Handler 已有 ModelRegistry 字段
- `web/src/app/tokens/` — token 页面需增加 heavy 池选项
- `web/src/app/` — 新增 models/ 目录

</code_context>

<specifics>
## Specific Ideas

- /v1/models 端点（INTG-02）已由 HandleModelsFromRegistry 实现，Phase 24 主要验证其正确性
- 严格模式意味着 API 用户必须使用 Registry 中注册的模型名，提供清晰的错误信息引导
- Master-Detail 布局让 family-mode 层级关系直观可见

</specifics>

<deferred>
## Deferred Ideas

None — 讨论内容完全在 Phase 24 范围内。

</deferred>

---

*Phase: 24-integration-admin-ui*
*Context gathered: 2026-04-15*
