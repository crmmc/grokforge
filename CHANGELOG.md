# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/), and this project adheres to [Semantic Versioning](https://semver.org/).

## v0.1.0-beta

更新内容:

b8ba1de: feat: GrokForge v0.1.0-beta — OpenAI-compatible Grok API gateway with admin panel
- OpenAI Chat Completions API（流式/非流式完整兼容）
- 思维链推理 + reasoning_effort 控制
- Tool Calling / Function Calling
- 多模态输入（图片 URL / base64）
- 图片生成 / 编辑（WebSocket）
- 视频生成
- /v1/models 动态模型列表
- Token 池管理（双池路由、三种选择算法、配额双模式）
- API Key 管理（CRUD + 白名单 + 限额）
- Next.js + shadcn/ui 管理面板（Dashboard / Token / API Key / 设置 / 缓存 / 使用统计 / Playground）
- 中英文国际化
- 单二进制部署（前端 go:embed）

---

## v0.2.0-beta

更新内容:

cc107d3: fix: upgrade Go 1.25.8 → 1.25.9 to fix crypto/x509 and crypto/tls vulnerabilities

9200cc3: fix: remove overly broad "grokforge" from .gitignore

8dc1725: feat: image_lite flow, enable_pro, force_thinking, admin auth improvements
- 新增 image_lite 流程（HTTP 方式，Basic 池可用）
- 新增 enable_pro 模式（Pro 图片生成）
- 新增 force_thinking 配置（强制开启推理）
- 管理面板认证改进（IP 限速 + 锁定）

2bd21b4: refactor: rename model type "image" → "image_ws", "image_lite" → "image"

0805f74: refactor: 清理 lint 警告 — 移除未使用的函数和参数

f911c08: fix: 修复退出登录无法跳转到登录页

ff76931: ci: build windows releases on native runner

c7a2664: feat(24): model management system — registry-driven routing, admin CRUD, frontend UI
- 注册表驱动的模型路由系统
- 管理面板模型 CRUD 页面（Master-Detail 布局）
- Admin 模型 CRUD API
- 移除硬编码 modelMappings
- React Query hooks + i18n + sidebar 导航

9c35619: test(24): add model integration tests + fix family dialog create payload

5b38992: fix(24-03): move models page into (admin) route group + default make target

0f978d7: feat(24-03): Models page Master-Detail layout with family/mode CRUD dialogs

a2a527a: feat(24-03): React Query hooks, i18n, and sidebar navigation for models page

e37bd9c: feat(24-01): add ResolveUpstream callback + rename resolveModelType

4046a37: feat(24-01): wire registry into flow/routing/models/frontend

9e9b94c: feat(24-02): admin model CRUD API + server wiring

7311893: feat(24-01): remove hardcoded modelMappings, use UpstreamModel/UpstreamMode in buildChatBody

7ab320a: feat(23-03): wire ModelResolver into all callers, frontend three-pool support
- 前端三池（basic/super/heavy）完整支持
- 全链路接入 ModelResolver

f39579e: refactor(23-03): delete BasicModels/SuperModels/PreferredPool from config layer
- 移除旧配置项，路由完全由 ModelRegistry 驱动

b225b65: feat(23-02): rewrite CostForModel to use ModelResolver

dfc2b3e: feat(23-02): rewrite GetPoolForModel + PickForModel for three-pool routing
- 三池路由核心重写（basic/super/heavy）

7400565: feat(23-01): add heavy pool cooling config + ReportRateLimit adaptation

5a7270d: feat(23-01): add PoolHeavy constant + PoolLevel tier system

61557af: feat(22-02): wire ModelRegistry into startup sequence

6f505c5: feat(22-02): implement ModelRegistry with in-memory snapshot
- 内存快照，O(1) 请求名解析

c7e77d7: feat(22-01): wire SeedModels into startup sequence after AutoMigrate

3e12b66: feat(22-01): implement SeedModels with TOML parsing and DB import

ea9c253: feat(22-01): create seed data TOML and embed.FS declaration
- config/models.seed.toml 种子数据文件

ed6f97f: feat(store): implement ModelStore CRUD with derived request name conflict checking

67ee8ff: feat(store): add ModelFamily + ModelMode structs and register in AllModels()
- 数据库模型定义，替代硬编码

adf09b4: fix: correct 12 doc-vs-code discrepancies found by audit

da177bd: fix runtime config edge cases

616895f: fix runtime config and stability issues

feea09c: fix: upgrade Go 1.24.1 → 1.25.8 to resolve 19 stdlib vulnerabilities

667e172: fix: make SPA handler tests self-contained with fstest.MapFS

---

## v0.3.0-beta

更新内容:

c8a1493: docs: update expert model pool_floor from super to basic in READMEs

5ba72ac: chore: clean up stale i18n references to removed Models page

0f57c65: refactor: replace DB-driven model registry with static TOML catalog

f2729ec: docs: update all admin panel screenshots with demo data

23b628c: feat: add SSE heartbeat, deepsearch passthrough, and grok-4.3-beta model
- SSE 心跳保活：2KB 初始填充 + 15s ping，防止反代/CDN 超时断连
- DeepSearch 透传：请求体新增 `deepsearch` 参数，透传到上游 `deepsearchPreset`
- 新增 Grok 4.3 Beta 模型（pool_floor=super, upstream=grok-420-computer-use-sa）

0765812: docs: update READMEs and CHANGELOG for heartbeat, deepsearch, grok-4.3-beta

86588a1: feat: replace fixed quota fields with dynamic mode-based quota system
- 移除固定四类配额（chat/image/video/grok43），改为 mode-based 动态配额
- TokenService 接入 ModeSpec，normalizeTokenQuotas 对齐 mode
- 引入 FirstUseTracker 驱动配额窗口刷新
- 前端重写配额展示：quota-presentation.ts + dashboard-quota-panel

8eea7ce: refactor: export PoolToShort and eliminate cross-package pool mapping duplication

31db192: fix: make usage chart height responsive to quota panel in dashboard grid

017f82e: refactor: replace token cooling state machine with mode-based dynamic quota system
- 移除 cooling 状态机及相关配置（CoolingStatusCodes、cool_duration_min 系列）
- ShouldCoolToken 硬编码为仅 429，403 不再触发 MarkExpired
- ImageFlow.Generate 切换至 WebSocket 路径，新增 image_ws.go 及 token+model 级内存冷却
- WebSocket 握手失败映射为语义化错误（401/403/429）
- admin/models API 返回 mode_groups 元数据
- 新增首次启动自动生成 admin bootstrap password（进程级临时密码）

c61657e: chore: gitignore grokforge binary and PRD_Tracking directory

c2bd3f6: docs: align READMEs with mode-based quota and bootstrap password changes

---
