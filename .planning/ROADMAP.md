# Roadmap: GrokForge

## Milestones

- ✅ **v1.0 MVP** — Phases 0-8 (shipped 2026-03-08)
- ✅ **v1.1 Frontend Complete** — Phases 9-13 (shipped 2026-03-11)
- ✅ **v1.2 Token Routing & Analytics** — Phases 14-17 (shipped 2026-03-12)
- ✅ **v1.3 Playground Enhancement** — Phases 18-20 (shipped 2026-03-15)
- 🚧 **v1.4 Model Management & Three-Pool** — Phases 21-24 (in progress)

## Phases

<details>
<summary>✅ v1.0 MVP (Phases 0-8) — SHIPPED 2026-03-08</summary>

See `.planning/milestones/v1.0-ROADMAP.md`

</details>

<details>
<summary>✅ v1.1 Frontend Complete (Phases 9-13) — SHIPPED 2026-03-11</summary>

See `.planning/milestones/v1.1-ROADMAP.md`

</details>

<details>
<summary>✅ v1.2 Token Routing & Analytics (Phases 14-17) — SHIPPED 2026-03-12</summary>

See `.planning/milestones/v1.2-ROADMAP.md`

</details>

<details>
<summary>✅ v1.3 Playground Enhancement (Phases 18-20) — SHIPPED 2026-03-15</summary>

See `.planning/milestones/v1.3-ROADMAP.md`

</details>

### 🚧 v1.4 Model Management & Three-Pool (In Progress)

**Milestone Goal:** 模型定义数据化，消除硬编码，支持三池路由

- [x] **Phase 21: Model Schema & Store** — model_family + model_mode 两表 CRUD + 冲突校验 (completed 2026-04-14)
- [x] **Phase 22: Seed Data & Registry** — 种子数据导入 + ModelRegistry 内存快照 (completed 2026-04-15)
- [x] **Phase 23: Three-Pool Routing** — heavy 池 + pool_floor 硬门槛 + 废弃旧配置 (completed 2026-04-15)
- [ ] **Phase 24: Integration & Admin UI** — 主链路接入 Registry + 模型管理页面

## Phase Details

### Phase 21: Model Schema & Store
**Goal**: 模型定义持久化到 DB，支持 family/mode 两级结构和请求名冲突校验
**Depends on**: Nothing (v1.4 first phase)
**Requirements**: MODEL-01, MODEL-02, MODEL-03
**Success Criteria** (what must be TRUE):
  1. model_family 表存在且可通过 store 层完成 CRUD 操作
  2. model_mode 表存在且通过外键关联 family，CRUD 正常工作
  3. 创建或更新 mode 时，若派生请求名与已有记录冲突，写入被拒绝并返回明确错误
  4. DB migration 在应用启动时自动执行，SQLite 和 PostgreSQL 均兼容
**Plans:** 1/1 plans complete
Plans:
- [x] 21-01-PLAN.md — ModelFamily/ModelMode 定义 + ModelStore CRUD + 冲突校验 + 测试

### Phase 22: Seed Data & Registry
**Goal**: 首次启动自动导入种子模型数据，ModelRegistry 提供 O(1) 请求名解析
**Depends on**: Phase 21
**Requirements**: SEED-01, REG-01
**Success Criteria** (what must be TRUE):
  1. 首次启动时 models.seed.toml 中的模型数据自动写入 DB，已有数据时跳过
  2. embed.FS 内嵌默认种子文件，无外部文件时仍可正常导入
  3. ModelRegistry 启动后加载全部 enabled 模型，byRequestName 和 enabledByType 索引可用
  4. DB 中模型数据变更后，调用刷新方法可更新 Registry 内存快照
**Plans:** 2/2 plans complete
Plans:
- [x] 22-01-PLAN.md — 种子数据文件 + embed.FS + SeedModels 函数 + main.go 接入
- [x] 22-02-PLAN.md — ModelRegistry 内存快照 + Refresh/Resolve/EnabledByType + main.go 接入

### Phase 23: Three-Pool Routing
**Goal**: Token 池从双池扩展为三池，pool_floor 硬门槛驱动模型路由
**Depends on**: Phase 21
**Requirements**: POOL-01, POOL-02, POOL-03
**Success Criteria** (what must be TRUE):
  1. Token 表支持 ssoHeavy 池类型，admin 可创建/编辑 heavy 池 token
  2. 模型请求根据 pool_floor 值匹配 >= 对应等级的池，不做级联降级
  3. basic_models / super_models / preferred_pool 配置项不再影响路由，路由完全由 ModelRegistry 驱动
  4. 旧配置项存在时不报错（向后兼容），但被忽略
**Plans:** 3/3 plans complete
Plans:
- [x] 23-01-PLAN.md — PoolHeavy 常量 + PoolLevel 等级体系 + heavy 冷却配置
- [x] 23-02-PLAN.md — GetPoolForModel/PickForModel/CostForModel 路由核心重写
- [x] 23-03-PLAN.md — 删除旧配置项 + 适配所有调用点 + 前端清理

### Phase 24: Integration & Admin UI
**Goal**: 主链路和 API 端点全面接入 ModelRegistry，前端提供模型管理界面
**Depends on**: Phase 22, Phase 23
**Requirements**: INTG-01, INTG-02, INTG-03
**Success Criteria** (what must be TRUE):
  1. xai/chat.go 的 modelMappings 和 chat 路由类型判断改为读取 ModelRegistry，硬编码映射移除
  2. GET /v1/models 返回的模型列表由 ModelRegistry 动态构建，新增/禁用模型后列表实时更新
  3. Admin API 提供模型 family/mode 的 CRUD 端点，操作后触发 Registry 刷新
  4. 前端模型管理页面可查看、新增、编辑、删除模型定义
  5. Token 管理页面的池选项包含 heavy 池
**Plans**: TBD
**UI hint**: yes

## Progress

**Execution Order:**
Phases execute in numeric order: 21 → 22 → 23 → 24
(Phase 22 and 23 can parallel — both depend only on 21)

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 0-8 | v1.0 | 23/23 | Complete | 2026-03-08 |
| 9-13 | v1.1 | 13/13 | Complete | 2026-03-11 |
| 14-17 | v1.2 | 9/9 | Complete | 2026-03-12 |
| 18-20 | v1.3 | 7/7 | Complete | 2026-03-15 |
| 21. Model Schema & Store | v1.4 | 1/1 | Complete    | 2026-04-14 |
| 22. Seed Data & Registry | v1.4 | 2/2 | Complete    | 2026-04-15 |
| 23. Three-Pool Routing | v1.4 | 3/3 | Complete   | 2026-04-15 |
| 24. Integration & Admin UI | v1.4 | 0/? | Not started | - |
