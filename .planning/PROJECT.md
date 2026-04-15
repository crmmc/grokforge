# GrokForge

## What This Is

Grok2API 的 Go 重写版。一个 OpenAI 兼容协议网关，对外暴露标准 OpenAI API，对内将请求转换为 Grok 私有协议（HTTP/SSE/WebSocket/gRPC-Web）。单二进制部署，面向个人或小团队使用。

## Core Value

Chat 主链路稳定可用：通过 tls-client 发起上游请求，正确转换 SSE 事件流，输出 OpenAI 兼容格式。

## Requirements

### Validated

- ✓ tls-client CGo 集成 + Chrome 133 profile — v1.0
- ✓ HTTP/HTTPS/SOCKS5 代理支持 — v1.0
- ✓ Session 重置策略 — v1.0
- ✓ Cookie/Header/Profile 管理 — v1.0
- ✓ /v1/chat/completions stream 模式 — v1.0
- ✓ /v1/chat/completions non-stream 模式 — v1.0
- ✓ OpenAI 兼容错误格式 — v1.0
- ✓ 重试与指数退避 — v1.0
- ✓ Multimodal 输入支持 — v1.0
- ✓ Tool Calling / Function Calling — v1.0
- ✓ Reasoning effort 控制 — v1.0
- ✓ 思维链输出 — v1.0
- ✓ SSE 错误事件 — v1.0
- ✓ Token 池管理 — v1.0
- ✓ Token 自动刷新 — v1.0
- ✓ Admin Token CRUD — v1.0
- ✓ 双池路由 — v1.0
- ✓ 配额双模式 — v1.0
- ✓ Image Generation — v1.0
- ✓ Image Edit — v1.0
- ✓ Video Generation — v1.0
- ✓ Responses API — v1.0
- ✓ GORM 持久化 — v1.0
- ✓ 配置加载 + 热重载 — v1.0
- ✓ Bearer Token 认证 — v1.0
- ✓ Model listing — v1.0
- ✓ 健康检查 — v1.0
- ✓ 结构化日志 — v1.0
- ✓ Admin 配置管理 API — v1.0
- ✓ Token 状态可视化 — v1.0
- ✓ Function 玩法页面 — v1.0
- ✓ Next.js + shadcn/ui 前端嵌入 — v1.0
- ✓ Dashboard 仪表盘（统计卡片、配额、调用图表） — v1.1
- ✓ Token 管理增强（筛选、批量、导入、备注、NSFW） — v1.1
- ✓ 系统设置在线管理 + 热重载 — v1.1
- ✓ 缓存管理页面（图片/视频统计、浏览、清理） — v1.1
- ✓ API Key CRUD + /v1/* 鉴权 + 限流 — v1.1
- ✓ 使用统计聚合 API — v1.1
- ✓ 100% 中英文国际化 — v1.1

- ✓ Model group management (ssoBasic/ssoSuper model CRUD in settings) — v1.2
- ✓ Priority group setting (global preference when model in both groups) — v1.2
- ✓ Unknown model 404 response — v1.2
- ✓ Token import default quota setting — v1.2
- ✓ Remove cooling pool fallback — v1.2
- ✓ Token selection algorithm (high-quota-first / random / round-robin) — v1.2
- ✓ Token priority field — v1.2
- ✓ Group cooldown time setting — v1.2
- ✓ Per-request usage logging (memory buffer + periodic DB flush) — v1.2
- ✓ Settings page tab split (model settings tab) — v1.2
- ✓ Detailed usage stats page (per-request log table) — v1.2
- ✓ Usage overview token consumption by time period — v1.2
- ✓ Remove token distribution chart — v1.2
- ✓ Dead code cleanup — v1.2

- ✓ Chat playground (multi-turn streaming + model selection + abort) — v1.3
- ✓ Conversation management (create/switch/delete + localStorage persistence) — v1.3
- ✓ Markdown rendering (react-markdown + syntax highlighting) — v1.3
- ✓ Image/Video dynamic model selection — v1.3
- ✓ Admin 功能体验 visibility toggle — v1.3
- ✓ Grok API P0 compatibility (model mapping + payload fields + dual stream format) — v1.3
- ✓ Grok API P1 hardening (missing fields + Sec-Ch-Ua + cf_clearance) — v1.3

### Active

## Current Milestone: v1.4 Model Management & Three-Pool

**Goal:** 模型定义数据化，消除硬编码，支持三池路由

**Target features:**
- model_family + model_mode 两张 DB 表，替代代码中散落的模型定义
- config/models.seed.toml 种子数据，首次启动自动导入
- ModelRegistry 内存快照，O(1) 请求名解析
- 三池支持 (basic/super/heavy)，pool_floor 硬门槛
- 请求名固定派生规则 + 写入时冲突校验
- Admin API 模型 CRUD 端点
- 前端模型管理页面 + token 管理 heavy 池选项
- 废弃旧 basic_models / super_models / preferred_pool 配置

### Out of Scope

- 多实例 / 分布式设计 — V1 单实例优先，不为假想场景付复杂度
- DDD 分层 / domain 目录 — 这是协议网关，不是企业信息系统
- 第二个 outbound engine — V1 只用 tls-client
- BrowserTransport 抽象接口 — 先写具体实现，有第二实现时再提炼
- SOCKS4/4a 代理 — 不进入 V1 承诺
- SSR 前端运行时 — Next.js 静态导出，不独立运行
- MIGR-01/02/03 迁移功能 — 人工手动完成，不纳入 v1.1
- VOICE-01~05 语音功能 — Deferred to v2 (LiveKit 高风险)
- CACHE-03 在线资产管理 — Deferred to v2
- quota_default / quota_override 运行时接入 — v1.4 预留字段，独立后续任务
- 上游 /rest/rate-limits 配额实时同步 — 不在 v1.4 范围
- 旧模型名 alias 兼容 — 当前版本不做，后续按需追加
- prefer_best 级联降级 — pool_floor 硬门槛足够

## Context

### 原项目现状
- Python FastAPI 单体网关，代码结构混乱，God file 问题严重
- 当前前端是静态 HTML + 原生 JS + Tailwind CDN，非构建型 SPA
- 本地存储：data/config.toml + data/token.json
- 持久化后端支持 local / Redis / SQL（MySQL/PostgreSQL）

### 复杂度来源
- 上游逆向 transport：TLS 指纹、HTTP/2、WebSocket、代理、Cookie、会话重置
- OpenAI 兼容层：请求格式兼容、SSE 事件转换、错误映射
- Token 池状态机：选择、成功/失败记录、冷却、刷新、持久化
- 进程内运行时状态：batch task、imagine/video session、定时任务

### 上游端点
- Chat: POST https://grok.com/rest/app-chat/conversations/new (HTTP + SSE)
- Imagine: wss://grok.com/ws/imagine/listen (WebSocket)
- LiveKit: GET https://grok.com/rest/livekit/tokens + wss://livekit.grok.com
- Media: POST https://grok.com/rest/media/post/create
- Assets: upload/list/delete/download via grok.com/rest/*
- Rate limits: POST https://grok.com/rest/rate-limits
- Video upscale: POST https://grok.com/rest/media/video/upscale
- gRPC-Web: accept-tos, nsfw-mgmt via accounts.x.ai / grok.com

### Token 模型
- 状态：active / disabled / expired / cooling
- 池：ssoBasic（基础模型优先）/ ssoSuper（高 tier 模型专用），模型分组可在 admin 配置
- 存储为裸 token，不存 sso= 前缀
- 默认 quota 可配置（auto: ssoBasic 80, ssoSuper 200）
- 连续 401 达阈值后标记 expired
- Token 选择算法可配置（high-quota-first / random / round-robin）
- Token priority 字段影响选择顺序
- 无 cooling fallback — all tokens cooling 时返回 503

## Constraints

- **Tech stack**: Go 后端 + net/http + chi + GORM + slog + tls-client
- **Tech stack**: Next.js + shadcn/ui + Tailwind CSS 前端，静态导出嵌入 Go embed.FS
- **Storage**: GORM 统一适配 SQLite（本地）和 PostgreSQL，不再用文件存储
- **Architecture**: 请求链路三段式 handler -> flow -> infra，不超过三层
- **Deployment**: 单二进制、单实例，前端构建产物嵌入
- **Dependencies**: httpapi -> flow -> token/xai/store/runtime，单向无循环
- **Transport**: tls-client 是 V1 唯一 outbound backend

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| GORM 替代 sqlc+pgx+goose 和文件存储 | 统一适配 SQLite/PostgreSQL，降低多后端维护成本 | ✓ Good |
| Next.js 替代 Vue 3 + Element Plus | 用户决策变更，采用 React 生态 | ✓ Good |
| shadcn/ui + Tailwind 替代 Element Plus | 配合 Next.js，现代 React UI 方案 | ✓ Good |
| 静态导出嵌入 Go 二进制 | 保持单二进制部署目标 | ✓ Good |
| Phase 0 纳入 scope | tls-client 验证未完成，需要先证明可行性 | ✓ Good |
| 单实例优先 | 当前就是单实例，先做稳定再考虑扩展 | ✓ Good |
| 不做 DDD / domain 层 | 协议网关的复杂度不在领域建模 | ✓ Good |
| token 保持一个包 | 连续状态机不拆跨包，避免一致性问题 | ✓ Good |
| WebSocket 用 gorilla 回退 | tls-client 原生 WS 握手失败，gorilla 可用 | ✓ Good |
| 前端 API 路径 /admin/* | 与后端路由一致，避免代理配置 | ✓ Good |
| Config-driven model routing 替代 hardcoded maps | 统一配置源，支持热重载，消除双重模型列表 | ✓ Good |
| UsageBuffer 内存缓冲 + 定时 flush | 减少 DB I/O，批量写入，30s 可配置间隔 | ✓ Good |
| Tab-based form split (per-tab useForm) | 各 tab 独立提交，partial PUT 避免跨 tab 覆盖 | ✓ Good |
| 移除 cooling fallback 返回 503 | 明确错误优于静默降级，符合 OpenAI 错误格式 | ✓ Good |

| useModels hook + prefix-based filtering | 共享 React Query cache，image/video/chat 复用 | ✓ Good |
| Single modelMappings table for 16 models | 替代 switch-based mapModel，易维护 | ✓ Good |
| Dual SSE+NDJSON stream parser | auto-detect by first char，兼容两种上游格式 | ✓ Good |
| localStorage for chat persistence | Playground 场景足够，避免 DB schema 变更 | ✓ Good |
| react-markdown + rehype-highlight | 成熟方案，github-dark 主题，think block 隔离 | ✓ Good |

## Status

- **Current Version:** v1.4 in progress
- **Previous:** v1.3 Playground Enhancement — SHIPPED 2026-03-15, v1.2 Token Routing & Analytics — SHIPPED 2026-03-12, v1.1 Frontend Complete — SHIPPED 2026-03-11, v1.0 MVP — SHIPPED 2026-03-08
- **Codebase:** 79,360 LOC Go + 8,500 LOC TS/TSX (87,860 total)
- **Tech Stack:** Go + chi + GORM + slog + tls-client / Next.js + shadcn/ui + Tailwind

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-04-15 — Phase 22 complete: Seed data loading + ModelRegistry in-memory snapshot*
